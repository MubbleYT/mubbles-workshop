# Mubble's Workshop

Live release catalog and publisher for **MubbleYT/mubbles-workshop**. The static website reads an online catalog; downloads come from GitHub Releases. This package contains no Workshop desktop executable or real mod binaries.

## Try the localhost preview

1. Extract the entire ZIP.
2. Double-click **START-WORKSHOP.cmd**. Your browser opens **http://localhost:3000**.
3. Keep the terminal open. Close it to stop.

Windows PowerShell runs the preview without installing anything or requiring administrator rights. The catalog initially has no published mods. Click **Show / hide samples** for six labelled interface examples. They have no download files. Local drafts stay in this browser.

Alternatively, with Node.js 22 or later, run `npm start`. No npm install is needed. The server listens only on 127.0.0.1; port 3000 must be free. Use localhost rather than opening index.html directly because this version uses JavaScript modules.

## Upload the source to GitHub

An agent with repository write access can upload the source directly. The package also provides a first-time uploader for your own Windows PC:

1. Install [Node.js LTS](https://nodejs.org/) (22 or later) and [GitHub CLI](https://cli.github.com/). Reopen terminals after installing.
2. Run **LOGIN-GITHUB.cmd** and complete the browser login as **MubbleYT**. Choose HTTPS if asked about the Git protocol.
3. Run **UPLOAD-WORKSHOP.cmd**.

This uploader targets MubbleYT/mubbles-workshop. It initializes an empty repository and commits the source. It stops if files other than README.md are already present, preventing an existing catalog from being overwritten. It uploads neither browser drafts nor mod archives. Git does not need to be installed separately.

The ChatGPT GitHub plugin and GitHub CLI on your PC are separate connections. The publisher reads the CLI credential internally and never prints it.

## Host the website on Cloudflare Pages

Create a free Cloudflare account. **R2 is not needed for this version.** In Workers & Pages, create a Pages project, connect GitHub, and select MubbleYT/mubbles-workshop. Use:

| Setting | Value |
| --- | --- |
| Production branch | main, or the repository's actual default branch |
| Framework preset | None |
| Build command | Leave blank |
| Build output directory | dist |
| Root directory | Leave blank |

Save and deploy. Cloudflare supplies the actual *.pages.dev URL; a domain purchase is optional. Pages redeploys after the publisher commits the catalog. Wait for deployment to finish, then press Refresh in Workshop.

This source package does not create a Cloudflare project or contain a deployed URL yet.

## Publish a finished release

With Node.js and GitHub CLI installed and LOGIN-GITHUB.cmd completed, double-click **PUBLISH-RELEASE.cmd**. Enter the original archive path and project details. You can instead supply a release-details JSON exported from a local Workshop draft. The original archive must be on disk; the publisher cannot read files stored inside your browser.

For an automatic build hook, run this after a successful build and your chosen release checks:

```sh
node tools/publish.mjs --file path/to/mod.jar --metadata path/to/release.json
```

Run from the source directory, or call the publisher by absolute path. Each mod project must include this step in its build process. This package does not watch every ChatGPT chat or automatically change other projects.

Edit examples/release.json, replace every placeholder, and set rightsConfirmed to true only when you have permission to distribute the file. Keep a stable project ID such as water-erosion. Increment the version whenever the binary changes. Categories and extensions are Mods (.jar), Shaders (.zip), Resource packs (.zip), and Modpacks (.mrpack).

An offline check that neither authenticates nor uploads:

```sh
node tools/publish.mjs --file path/to/mod.jar --metadata path/to/release.json --dry-run
```

For CI, store a repository-scoped credential in the build system's secrets and expose it as GH_TOKEN. Fine-grained tokens need Contents read/write access to this Workshop repository. GitHub Actions running in this repository may use GITHUB_TOKEN with contents: write; another repository's token does not automatically grant access here. Credentials belong outside the website, catalog, repository files and desktop app.

## Release reliability

The publisher validates metadata, extension, archive signature and distribution confirmation. It computes SHA-256, creates a draft release, uploads the archive and workshop-release.json, verifies the uploaded file checksum, and publishes the draft. It then commits dist/catalog.json using GitHub's SHA guard. Concurrent changes are re-read and merged.

Previous versions remain listed. The publisher rejects a different binary using an existing version, and resumes matching drafts after interrupted uploads. If the release upload succeeds but the catalog commit fails, rerun the same command to finish. Completed matching files are reused.

Archive signature validation is not malware scanning or complete archive verification. The script does not overwrite published binaries, but the repository owner can still delete releases. GitHub's optional immutable release policy is separate and is not enabled by this package.

## What works and what remains

The website fetches catalog.json on launch and Refresh. It displays projects, release history, checksums, licenses, source links and AI disclosure. Filters match an older release's Minecraft version and loader together. Downloads open GitHub Releases directly. Checksums are displayed; browser downloads are not automatically verified by the website.

A successfully loaded catalog is cached for offline viewing. Downloads need internet. Browser drafts and files are saved in IndexedDB on this device. Clearing browser data removes them; keep the originals.

The first Workshop desktop executable still needs to be built and configured with the hosted catalog URL. It can consume this versioned catalog and verify downloaded checksums. New catalog entries would not require rebuilding the app. Changing app behavior or supported catalog formats may require an update.

Only the repository owner/publishing credential can publish. No visitor accounts, public creator uploads, moderation queue, malware scanner, download analytics, or verified compatibility tests are included. Requiring source links for AI-generated executable mods is a Workshop metadata rule, not a third-party platform eligibility guarantee.

Cloudflare Pages hosts the small website and catalog; archives belong in GitHub Releases, not Git history or Pages assets. GitHub's release file limit is under 2 GiB. Costs depend on the current provider free limits. No rented server, database, R2 or purchased domain is required for this initial setup.

## Checks

Run `npm run check` and `npm test`. Tests simulate GitHub and cover upload ordering, resumable drafts, repeat publication, changed binary rejection, checksum mismatches, version history, concurrent catalog changes and first-time source upload guards. Live authenticated publishing, Cloudflare deployment and Windows execution need separate verification after account setup. There are no package dependencies to install.

## Official documentation

- [GitHub Releases and limits](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases)
- [GitHub release API](https://docs.github.com/en/rest/releases/releases)
- [GitHub release asset API](https://docs.github.com/en/rest/releases/assets)
- [GitHub CLI login](https://cli.github.com/manual/gh_auth_login)
- [Cloudflare Pages Git integration](https://developers.cloudflare.com/pages/get-started/git-integration/)
- [Cloudflare Pages limits](https://developers.cloudflare.com/pages/platform/limits/)

This independent project is not affiliated with Mojang, Microsoft, Modrinth, Cloudflare or GitHub.
