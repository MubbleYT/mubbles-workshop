#!/usr/bin/env python3
"""Build and publish immutable bot releases, then promote the update channel."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parent
REPO = "MubbleYT/mubbles-workshop"
API = "https://api.github.com/repos/" + REPO
OUT = ROOT / ".release-build"
VERSION = (ROOT / "VERSION").read_text().strip()
if not re.fullmatch(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", VERSION):
    raise SystemExit("bot/VERSION must be a stable X.Y.Z version")
TAG = "bot-v" + VERSION
TOKEN = os.environ.get("GH_TOKEN", "")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def request(endpoint, method="GET", body=None, content_type="application/json"):
    if not TOKEN:
        raise RuntimeError("Publishing requires the workflow's GH_TOKEN")
    url = endpoint if endpoint.startswith("https://") else API + endpoint
    if not (url.startswith(API + "/") or url.startswith("https://uploads.github.com/repos/" + REPO + "/")):
        raise RuntimeError("Refusing to publish outside the Workshop repository")
    data = body if isinstance(body, bytes) else None if body is None else json.dumps(body).encode()
    headers = {"Authorization": "Bearer " + TOKEN, "Accept": "application/vnd.github+json",
               "User-Agent": "MubbleDiscordBot-release", "X-GitHub-Api-Version": "2022-11-28",
               "Content-Type": content_type}
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=120) as response:
            raw = response.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        if exc.code == 404 and method == "GET":
            return None
        # Never expose request headers or credentials in logs.
        raise RuntimeError(f"GitHub {method} request failed with HTTP {exc.code}") from None


def build():
    OUT.mkdir(exist_ok=True)
    exe = OUT / "MubbleDiscordBot.exe"
    env = dict(os.environ, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0")
    subprocess.run(["go", "build", "-buildvcs=false", "-trimpath",
                    "-ldflags=-s -w -X main.version=" + VERSION, "-o", str(exe), "."],
                   cwd=ROOT / "source", env=env, check=True)
    checksums = OUT / "CHECKSUMS.txt"
    checksums.write_text(digest(exe) + "  MubbleDiscordBot.exe\n")
    notes = (ROOT / "CHANGELOG.txt").read_text().strip()
    manifest = {"schema": 1, "version": VERSION,
                "url": f"https://github.com/{REPO}/releases/download/{TAG}/MubbleDiscordBot.exe",
                "sha256": digest(exe), "size": exe.stat().st_size, "notes": notes}
    (OUT / "update.json").write_text(json.dumps(manifest, indent=2) + "\n")
    archive = OUT / "MubbleDiscordBot-Windows.zip"
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for p in [exe, checksums, ROOT / "README.txt", ROOT / "THIRD-PARTY-LICENSES.txt"]:
            z.write(p, p.name)
        for p in sorted((ROOT / "source").iterdir()):
            if p.is_file() and p.suffix in {".go", ".mod", ".sum", ".html", ".js", ".cmd"}:
                z.write(p, "source/" + p.name)
    with zipfile.ZipFile(archive) as z:
        assert z.testzip() is None
        assert hashlib.sha256(z.read(exe.name)).hexdigest() == manifest["sha256"]
    print(f"Built and verified bot {VERSION}: {exe.stat().st_size} bytes")


def public_asset_matches(url, expected_hash):
    for attempt in range(12):
        fresh_url = url + "?verify=" + str(time.time_ns())
        req = urllib.request.Request(fresh_url, headers={"User-Agent": "MubbleDiscordBot-release", "Cache-Control": "no-cache"})
        try:
            with urllib.request.urlopen(req, timeout=120) as response:
                h = hashlib.sha256()
                for chunk in iter(lambda: response.read(128 * 1024), b""):
                    h.update(chunk)
                if h.hexdigest() != expected_hash:
                    raise RuntimeError("Published download checksum did not match")
                return
        except urllib.error.HTTPError as exc:
            if exc.code not in (404, 502, 503) or attempt == 11:
                raise RuntimeError(f"Published download returned HTTP {exc.code}") from None
            time.sleep(5)
    raise RuntimeError("Published download could not be verified")


def publish():
    manifest_path = OUT / "update.json"
    manifest = json.loads(manifest_path.read_text())
    if manifest["version"] != VERSION:
        raise RuntimeError("Build version did not match release request")
    existing = request("/releases/tags/" + TAG)
    if existing:
        if existing.get("draft"):
            raise RuntimeError("An incomplete draft already exists; review it before retrying")
        assets = {a["name"]: a for a in existing["assets"]}
        required = {"MubbleDiscordBot.exe", "MubbleDiscordBot-Windows.zip", "CHECKSUMS.txt", "update.json"}
        if not required.issubset(assets):
            raise RuntimeError("Published release is missing required assets; use a new version")
        if assets["MubbleDiscordBot.exe"].get("digest") != "sha256:" + manifest["sha256"]:
            raise RuntimeError("This version already has different executable bytes; bump bot/VERSION")
        if assets["update.json"].get("digest") != "sha256:" + digest(manifest_path):
            raise RuntimeError("Published metadata changed; bump bot/VERSION")
        print("Existing immutable release matches; retrying public verification and channel promotion.")
    else:
        release = request("/releases", "POST", {
            "tag_name": TAG, "target_commitish": os.environ.get("GITHUB_SHA", "main"),
            "name": "Mubble Discord Bot " + VERSION, "body": manifest["notes"],
            "draft": True, "prerelease": False, "make_latest": "false"})
        assets = {}
        for name in ["MubbleDiscordBot.exe", "MubbleDiscordBot-Windows.zip", "CHECKSUMS.txt", "update.json"]:
            path = OUT / name
            url = release["upload_url"].split("{", 1)[0] + "?name=" + urllib.parse.quote(name)
            assets[name] = request(url, "POST", path.read_bytes(), "application/octet-stream")
        request("/releases/" + str(release["id"]), "PATCH", {"draft": False, "make_latest": "false"})
    for name in ["MubbleDiscordBot.exe", "MubbleDiscordBot-Windows.zip"]:
        asset_hash = assets[name].get("digest", "")
        if not re.fullmatch(r"sha256:[a-f0-9]{64}", asset_hash):
            raise RuntimeError("Published asset did not provide a SHA-256 digest")
        public_asset_matches(assets[name]["browser_download_url"], asset_hash.split(":", 1)[1])
    # A newer source release may have been queued while these tests/build ran.
    current_version = request("/contents/bot/VERSION?ref=main")
    if base64.b64decode(current_version["content"]).decode().strip() != VERSION:
        print("Release is online; channel promotion skipped because newer source is queued.")
        return
    current = request("/contents/bot/update.json?ref=main")
    if current:
        current_manifest = json.loads(base64.b64decode(current["content"]))
        key = lambda value: tuple(map(int, value.split(".")))
        if key(current_manifest["version"]) > key(VERSION):
            raise RuntimeError("Refusing to downgrade the update channel")
    body = {"message": "Publish Discord bot online update " + VERSION,
            "content": base64.b64encode(manifest_path.read_bytes()).decode(), "branch": "main"}
    if current:
        body["sha"] = current["sha"]
    request("/contents/bot/update.json", "PUT", body)
    print("Bot release is online and its verified update channel is active.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--build", action="store_true")
    parser.add_argument("--publish", action="store_true")
    args = parser.parse_args()
    if args.build:
        build()
    if args.publish:
        publish()
    if not (args.build or args.publish):
        parser.error("Use --build and/or --publish")
