package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func updateResponse(r *http.Request, code int, body []byte) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: r}
}
func updateFixture(t *testing.T) (*App, *BotUpdater) {
	t.Helper()
	dir := t.TempDir()
	store, e := openStore(filepath.Join(dir, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	cfg := defaultConfig()
	cfg.GuildID = "123456789012345678"
	cfg.Token = "local-token"
	a := &App{dir: dir, cfg: cfg, store: store, csrf: "panel-key", host: "127.0.0.1:9999", shutdown: make(chan struct{})}
	u := newBotUpdater(a)
	a.updater = u
	return a, u
}
func sampleUpdatePE() []byte {
	b := make([]byte, 1024)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[0x84:], 0x8664)
	binary.LittleEndian.PutUint16(b[0x86:], 1)
	binary.LittleEndian.PutUint16(b[0x94:], 240)
	binary.LittleEndian.PutUint16(b[0x96:], 0x22)
	o := b[0x98:]
	binary.LittleEndian.PutUint16(o, 0x20b)
	binary.LittleEndian.PutUint32(o[16:], 0x1000)
	binary.LittleEndian.PutUint64(o[24:], 0x140000000)
	binary.LittleEndian.PutUint32(o[32:], 4096)
	binary.LittleEndian.PutUint32(o[36:], 512)
	binary.LittleEndian.PutUint32(o[56:], 8192)
	binary.LittleEndian.PutUint32(o[60:], 512)
	binary.LittleEndian.PutUint16(o[68:], 3)
	binary.LittleEndian.PutUint32(o[108:], 16)
	s := b[0x188:]
	copy(s, ".text")
	binary.LittleEndian.PutUint32(s[8:], 16)
	binary.LittleEndian.PutUint32(s[12:], 4096)
	binary.LittleEndian.PutUint32(s[16:], 512)
	binary.LittleEndian.PutUint32(s[20:], 512)
	return b
}
func updateMetadata(version string, data []byte) UpdateManifest {
	h := sha256.Sum256(data)
	return UpdateManifest{Schema: 1, Version: version, URL: updateReleasePrefix + version + "/MubbleDiscordBot.exe", SHA256: hex.EncodeToString(h[:]), Size: int64(len(data)), Notes: "A new bot version"}
}
func TestUpdateVersionOrderingAndStableOnly(t *testing.T) {
	for _, tc := range []struct {
		next, current string
		want          bool
	}{
		{"1.4.1", "1.4.0", true}, {"1.10.0", "1.9.9", true}, {"2.0.0", "1.99.99", true}, {"1.4.0", "1.4.0", false},
		{"1.3.9", "1.4.0", false}, {"1.4.1-beta", "1.4.0", false}, {"1.04.1", "1.4.0", false}, {"v1.5.0", "1.4.0", false},
	} {
		if got := newerVersion(tc.next, tc.current); got != tc.want {
			t.Fatalf("%s > %s = %v", tc.next, tc.current, got)
		}
	}
}
func TestUpdateManifestIsBoundToWorkshop(t *testing.T) {
	good := updateMetadata("1.4.1", sampleUpdatePE())
	if e := validUpdateManifest(good); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*UpdateManifest){
		func(m *UpdateManifest) {
			m.URL = "http://github.com/MubbleYT/mubbles-workshop/releases/download/bot-v1.4.1/MubbleDiscordBot.exe"
		},
		func(m *UpdateManifest) { m.URL = "https://github.com/attacker/repo/download/MubbleDiscordBot.exe" },
		func(m *UpdateManifest) { m.URL += "?redirect=bad" }, func(m *UpdateManifest) { m.SHA256 = "invalid" },
		func(m *UpdateManifest) { m.Size = maxUpdateBytes + 1 }, func(m *UpdateManifest) { m.Version = "1.4.1-beta" }, func(m *UpdateManifest) { m.Schema = 2 },
	} {
		bad := good
		mutate(&bad)
		if validUpdateManifest(bad) == nil {
			t.Fatal("unsafe manifest accepted", bad)
		}
	}
	for _, raw := range []string{"http://objects.githubusercontent.com/a", "https://evil.example/a", "https://github.com/attacker/repo/a", "https://user@release-assets.githubusercontent.com/a", "https://objects.githubusercontent.com:443/a"} {
		v, _ := url.Parse(raw)
		if trustedUpdateRedirect(v) {
			t.Fatal("unsafe redirect accepted", raw)
		}
	}
	for _, raw := range []string{"https://release-assets.githubusercontent.com/a", "https://objects.githubusercontent.com/a", good.URL} {
		v, _ := url.Parse(raw)
		if !trustedUpdateRedirect(v) {
			t.Fatal("valid download redirect rejected", raw)
		}
	}
}
func TestUpdateDownloadVerifiesHashSizeAndExecutable(t *testing.T) {
	for _, tc := range []string{"valid", "hash mismatch", "truncated", "oversized", "not an exe", "HTTP failure"} {
		t.Run(tc, func(t *testing.T) {
			_, u := updateFixture(t)
			data := sampleUpdatePE()
			m := updateMetadata("1.4.1", data)
			code := 200
			switch tc {
			case "hash mismatch":
				data[1023] = 1
			case "truncated":
				data = data[:600]
			case "oversized":
				data = append(data, 1)
			case "not an exe":
				data = []byte(strings.Repeat("invalid download", 80))
				m = updateMetadata("1.4.1", data)
			case "HTTP failure":
				code = 404
			}
			u.client.Transport = updateTransport(func(r *http.Request) (*http.Response, error) { return updateResponse(r, code, data), nil })
			path := filepath.Join(t.TempDir(), "update.exe")
			e := u.download(context.Background(), m, path)
			if tc == "valid" {
				if e != nil {
					t.Fatal(e)
				}
				if u.snapshot().Downloaded != m.Size {
					t.Fatal("progress not recorded")
				}
			} else {
				if e == nil {
					t.Fatal("bad download accepted")
				}
				if _, e = os.Stat(path); !os.IsNotExist(e) {
					t.Fatal("failed staging file retained")
				}
			}
		})
	}
}
func TestUpdateNetworkFailureKeepsConfigurationAndData(t *testing.T) {
	a, u := updateFixture(t)
	a.store.warn(a.cfg.GuildID, "member", "mod", "reason")
	before, _ := json.Marshal(a.store.snapshot())
	cfg := a.config()
	u.client.Transport = updateTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if e := u.check(context.Background()); e == nil {
		t.Fatal("offline check succeeded")
	}
	after, _ := json.Marshal(a.store.snapshot())
	if string(before) != string(after) || a.config().Token != cfg.Token || u.snapshot().Busy {
		t.Fatal("failed check changed data or stayed busy")
	}
}
func TestUpdateChecksCurrentAndNewerWithoutInstalling(t *testing.T) {
	for _, candidate := range []string{"1.3.0", version, "1.5.0"} {
		a, u := updateFixture(t)
		m := updateMetadata(candidate, sampleUpdatePE())
		raw, _ := json.Marshal(m)
		u.client.Transport = updateTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "raw.githubusercontent.com" {
				t.Fatal("check attempted executable download")
			}
			if r.Header.Get("Authorization") != "" {
				t.Fatal("bot token sent to update service")
			}
			return updateResponse(r, 200, raw), nil
		})
		if e := u.check(context.Background()); e != nil {
			t.Fatal(e)
		}
		state := u.snapshot()
		if state.Available != newerVersion(candidate, version) || state.Latest != candidate || state.Busy {
			t.Fatal(state)
		}
		if _, e := os.Stat(filepath.Join(a.dir, "updates")); !os.IsNotExist(e) {
			t.Fatal("check staged an install")
		}
	}
}
func TestUpdateRoutesRequirePostAndPanelKey(t *testing.T) {
	a, u := updateFixture(t)
	raw, _ := json.Marshal(updateMetadata("1.5.0", sampleUpdatePE()))
	u.client.Transport = updateTransport(func(r *http.Request) (*http.Response, error) { return updateResponse(r, 200, raw), nil })
	for _, tc := range []struct {
		method, key string
		want        int
	}{{"POST", "", 403}, {"GET", a.csrf, 405}, {"POST", a.csrf, 200}} {
		r := httptest.NewRequest(tc.method, "http://"+a.host+"/api/update-check", nil)
		r.Header.Set("X-Panel-Key", tc.key)
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if e := u.begin(); e != nil {
		t.Fatal(e)
	}
	if e := u.begin(); e == nil {
		t.Fatal("concurrent updater allowed")
	}
	u.finish(nil)
}
func planFixture(t *testing.T) (string, UpdatePlan, string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("APPDATA", base)
	appDir := filepath.Join(base, "MubbleDiscordBot")
	dir := filepath.Join(appDir, "updates", "pending-test")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	data := sampleUpdatePE()
	m := updateMetadata(version, data)
	target := filepath.Join(base, "MubbleDiscordBot.exe")
	os.WriteFile(target, data, 0600)
	p := UpdatePlan{Target: target, Staged: filepath.Join(dir, "update.exe"), ParentPID: os.Getpid(), SHA256: m.SHA256, Size: m.Size, Version: m.Version, Nonce: strings.Repeat("a", 64)}
	os.WriteFile(p.Staged, data, 0600)
	path := filepath.Join(dir, "plan.json")
	atomicJSON(path, p)
	return path, p, appDir
}
func TestUpdatePlanAndHealthAreBoundToLocalStaging(t *testing.T) {
	path, p, appDir := planFixture(t)
	if _, _, e := loadUpdatePlan(path); e != nil {
		t.Fatal(e)
	}
	health := filepath.Join(filepath.Dir(path), "health.json")
	if e := writeUpdateHealth(appDir, []string{"--update-health", health}); e != nil {
		t.Fatal(e)
	}
	var got UpdateHealth
	raw, _ := os.ReadFile(health)
	json.Unmarshal(raw, &got)
	if got.Nonce != p.Nonce || got.Version != version {
		t.Fatal("health check binding lost")
	}
	for _, bad := range []UpdatePlan{
		{Target: p.Target, Staged: p.Target, ParentPID: p.ParentPID, SHA256: p.SHA256, Size: p.Size, Version: p.Version, Nonce: p.Nonce},
		{Target: p.Target, Staged: p.Staged, ParentPID: p.ParentPID, SHA256: p.SHA256, Size: p.Size, Version: p.Version, Nonce: "short"},
	} {
		atomicJSON(path, bad)
		if _, _, e := loadUpdatePlan(path); e == nil {
			t.Fatal("unbound plan accepted")
		}
	}
	if _, _, e := loadUpdatePlan(filepath.Join(appDir, "plan.json")); e == nil {
		t.Fatal("plan outside update folder accepted")
	}
}
func TestAutomaticRetryStopsAfterFailedVersion(t *testing.T) {
	a, u := updateFixture(t)
	m := updateMetadata(nextUpdateVersion(), sampleUpdatePE())
	raw, _ := json.Marshal(m)
	atomicJSON(filepath.Join(a.dir, "update-result.json"), UpdateResult{Version: m.Version, Error: "startup failed"})
	u.client.Transport = updateTransport(func(r *http.Request) (*http.Response, error) { return updateResponse(r, 200, raw), nil })
	e := u.perform(context.Background(), true, false)
	if e == nil || !strings.Contains(e.Error(), "automatic retry is paused") {
		t.Fatal("failed version retried", e)
	}
}
func TestLegacySettingsEnableOnlineUpdatesWithoutYouTubeKey(t *testing.T) {
	c := defaultConfig()
	json.Unmarshal([]byte(`{"token":"bot-token","auto_start":false,"youtube_api_key":"old-unused-key"}`), &c)
	raw, _ := json.Marshal(c)
	if !c.AutoUpdate || c.AutoStart || strings.Contains(string(raw), "youtube_api_key") {
		t.Fatal("legacy settings did not migrate to simple updater and key-free video feed")
	}
}

func nextUpdateVersion() string {
	parts, _ := versionParts(version)
	return fmt.Sprintf("%d.%d.%d", parts[0], parts[1], parts[2]+1)
}
