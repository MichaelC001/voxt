# Releases and updates

VoxtMini is distributed through [GitHub Releases](https://github.com/hehehai/VoxtMini/releases) and the ActNow family site. GitHub releases carry a notarized macOS Apple silicon DMG. Its page is [actnow.dev/voxtmini](https://actnow.dev/voxtmini), with the download at [actnow.dev/download/voxtmini](https://actnow.dev/download/voxtmini). Installed copies receive signed updates from `https://actnow.dev/updates/voxtmini`; beta copies use the corresponding beta feed (`/updates/voxtmini/beta`). Copies built before that point at `https://actnow.dev/updates/voxtnew`, which the site redirects to `/updates/voxtmini` (MyGo's updater follows redirects). Since 0.1.0-beta.3 the app has an identity of its own: bundle id `dev.hehehai.voxtmini` (helper `dev.hehehai.voxtmini.helper`, language models `dev.hehehai.voxtmini.llm`), URL scheme `voxtmini`, folders `~/Library/{Application Support,Caches,Logs}/VoxtMini`, ActNow app id `voxtmini`. Earlier betas were `dev.hehehai.voxtnew`, the id Voxt 2 ships with. They update through the same feed and the same update key (MyGo's updater checks the Ed25519 signature, not the bundle id, and swaps the bundle at the same path); the first start of the new id moves VoxtMini's data, caches and logs, its model claims and its place among the users of the family's shared keys over, and turns opening at login on again ([behavior.md](behavior.md) 9.27). macOS keeps permissions per bundle id: those users grant the microphone and Accessibility once more. Say so in the release notes.

The native speech runtime is included at `sglang_omni_mlx/native` and pinned in [`UPSTREAM_PIN`](../sglang_omni_mlx/native/UPSTREAM_PIN). `scripts/build-omni.sh` builds only this checked-in version, so a release does not depend on a neighboring SGLang-Omni checkout.

### The oldest macOS

A release runs on macOS 15.0 and later (`mygo.json`'s `macos.minimumSystemVersion`), though the runner builds on macOS 26 with Xcode 26. MLX publishes a wheel per macOS (14, 15, 26), and the macOS 26 one's `mlx.metallib` uses Metal 4, which macOS 15 cannot load (issue #162: every local speech model failed on macOS 15 in 0.1.0-beta.6 and earlier). So `scripts/build-omni.sh` sets `MACOSX_DEPLOYMENT_TARGET` from `mygo.json`, and the runtime's `build_runtime.sh` installs the wheel for it (`uv pip install --python-platform aarch64-apple-darwin`) and builds the servers for it (`CMAKE_OSX_DEPLOYMENT_TARGET`). voxtmini-llm's `mlx.metallib` follows mlx-swift's own platform (macOS 14, Metal 3.1). The macOS 26 build's extra kernels (Metal 4, for the M5's neural accelerators) are not shipped. `scripts/release-verify.sh` runs `internal/tools/minos` over the app: it fails when any Mach-O file's `minos`, a Metal library's target macOS or its Metal language version is newer than `mygo.json`'s minimum. To check a published build on any machine, unpack its update archive and run `go run ./internal/tools/minos -mygo mygo.json VoxtMini.app`.

## GitHub Actions setup

The release workflow builds on an Apple silicon macOS runner. Add these repository secrets in **Settings → Secrets and variables → Actions**. Organization secrets with access to this repository also work.

| Secret | Purpose |
|---|---|
| `MACOS_CERTIFICATE` | Base64-encoded Developer ID Application `.p12` certificate |
| `MACOS_CERTIFICATE_PASSWORD` | Password for the `.p12` |
| `APPLE_TEAM_ID` | Developer team ID, currently `779M533DL5` |
| `APPLE_ID` | Apple developer account used by `notarytool` |
| `APPLE_APP_PASSWORD` | App-specific password for that account |
| `MYGO_UPDATER_PRIVATE_KEY` | Existing update private key (the VoxtNew-era one, kept across the bundle id change) matching `release/mygo.release.json` |
| `ACTNOW_INGEST_TOKEN` | ActNow release-ingest token for the `voxtmini` app (the site's `RELEASE_INGEST_TOKEN_VOXTMINI`) |

`scripts/export-devid-secret.sh`, run in Terminal on the Mac that holds the Developer ID certificate, exports it and sets `MACOS_CERTIFICATE` and `MACOS_CERTIFICATE_PASSWORD`. The update signing key is the existing VoxtNew app identity's (public key `MbROqANE…`, kept on the release Mac in `~/.config/voxtnew-release/update-keys/`; Auralink's is a different key); the ActNow token is the `voxtmini` app's (the site rejects `--app voxtnew`). Do not generate a replacement update key: installed copies trust the public key already embedded in them. The workflow stores the signing certificate and update key in temporary runner files and removes the signing keychain at the end.

For local releases, `scripts/release.sh --notarize <profile> --publish` uses the same app identity, update key, ActNow feed and signing team. `VOXTMINI_SIGN_IDENTITY` selects the Developer ID identity, `VOXTMINI_UPDATE_KEY` may point at the existing private key file, and `ACTNOW_INGEST_TOKEN` supplies the site token. A build without the signing identity is a local development build and cannot update installed copies.

## Publishing a version

1. Set the same version in `mygo.json`, `helper/Support/Info.plist` and `llm/Support/Info.plist`. The first standalone release continues the existing sequence at `0.1.0-beta.2`.
2. Run `scripts/vulncheck.sh` (govulncheck on the darwin/arm64 build, needs the network): it must report no vulnerabilities. A Go standard library one is fixed by raising `go.mod`'s Go version (and the version in `AGENTS.md`, `docs/development.md`, `CONTRIBUTING.md`); a module's by upgrading the module.
3. Move the release notes from `Unreleased` into a versioned section in `CHANGELOG.md`. Keep the English notes first and the Chinese notes under `### 中文`.
4. Commit and push the version, then create and push the matching tag, for example `v0.1.0-beta.2`.
5. The tag workflow builds the app, signs it with Developer ID, notarizes and staples the app and DMG, verifies the bundle, publishes the update to ActNow, then creates a **draft GitHub release** with the DMG attached (`scripts/release-github.sh`, below).
6. Review and publish the GitHub draft. Beta versions should remain marked as pre-releases. ActNow updates become available after the workflow publishes them.

`scripts/release-github.sh [vVERSION]` is that last step on its own, for a release built on a Mac: it checks that the tag is `v` + `mygo.json`'s version, takes the notes of that version from `CHANGELOG.md` (it stops when there is none), and creates the tag's draft release with every `build/darwin-arm64/*.dmg` attached and a download line on top of the notes, as a full release marked **latest** even for a beta (the older v1.x releases are Voxt's; GitHub only picks its latest release among non-prereleases, so a pre-release would leave Voxt 1.15.0 as the latest); when the release exists already it turns it back into a draft with the same title and notes and replaces the DMGs. It needs `gh` and `GH_TOKEN` (or `GITHUB_TOKEN`) and never publishes: review the draft and publish it on GitHub.

A manual run from **Actions → Release → Run workflow** makes an artifact for checking the build. With `sign` off, it does not require release secrets and does not publish. With `sign` on, it signs and notarizes but still does not publish.

## Distribution paths

- **GitHub install:** users download the DMG from GitHub Releases and install VoxtMini.
- **ActNow install:** users download from [actnow.dev/voxtmini](https://actnow.dev/voxtmini).
- **In-app update:** every installed copy keeps reading the existing ActNow feed. GitHub release publication does not change the update source.
