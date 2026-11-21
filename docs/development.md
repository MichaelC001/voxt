# Building and testing

How to build VoxtMini from source, keep macOS permissions across rebuilds, and run the tests. The rules for changes are in [AGENTS.md](../AGENTS.md) and [CONTRIBUTING.md](../CONTRIBUTING.md); the code's structure in [architecture.md](architecture.md).

## Build from source

Needs Go 1.27.4 (`GOTOOLCHAIN=auto` fetches it) and, to build the app, an Apple silicon Mac with macOS 15 or later, Xcode (the helper and `voxtmini-llm`), and [uv](https://docs.astral.sh/uv/) or Python 3 for the Omni runtime's build tools (`scripts/build-omni.sh` builds `sglang_omni_mlx/native` with its pinned MLX; CMake fetches its C++ dependencies from GitHub, through the shell's `https_proxy` if needed). Run from the repository root:

```sh
scripts/build-helper.sh              # the Swift helper, into resources/darwin-arm64/helper/
scripts/build-omni.sh                # the Omni runtime (speech models), into resources/darwin-arm64/omni/
scripts/build-llm.sh                 # voxtmini-llm (language models), into resources/darwin-arm64/llm/
go tool mygo dev                     # run with hot reload
go tool mygo build                   # build/darwin-arm64/VoxtMini.app and its disk image
```

The helper and `voxtmini-llm` must be built before `go tool mygo build` or `mygo dev`: `mygo.json` gives both their entitlements and fails without them. Without the Omni runtime the app runs but cannot recognise speech (or point `VOXTMINI_OMNI_RUNTIME` at one built with `scripts/build-omni.sh --out DIR`; likewise `VOXTMINI_LLM` for `scripts/build-llm.sh --out DIR`).

**The release build** is one command, on the Mac: `scripts/release.sh` builds the three, packages and signs the app, and checks the result (`--runtimes DIR` takes prebuilt runtimes; with the ActNow apps' Developer ID, `--notarize` and `--publish` notarise it and publish it on actnow.dev; over ssh it runs through `scripts/mac-gui.sh`). See [release.md](release.md).

From Linux, `scripts/mac.sh <command>` mirrors the project to a Mac over ssh (`MAC_HOST`, an ssh host name; `MAC_DIR`, default `code/VoxtMini`) and runs the command there: `scripts/mac.sh 'scripts/build-helper.sh && scripts/build-llm.sh && go tool mygo build'`.

## Signing during development

macOS keeps the Microphone and Accessibility grants for a signature's *designated requirement*. An ad hoc signature (`codesign -s -`, what `go tool mygo build`, `scripts/build-helper.sh` and `scripts/build-llm.sh` do without an identity) has the cdhash as its requirement, and every build has a new one: after a rebuild macOS forgets the grants. The microphone turns **denied without asking again**, and Accessibility still shows on in System Settings but does nothing. The helper's grants are the app's (macOS checks the helper against VoxtMini.app, its responsible process: [helper-protocol.md](helper-protocol.md)), so the app's signature is what counts. A release (Developer ID) keeps its grants; over ssh `codesign` cannot use that key (it is in the login keychain: `scripts/mac-gui.sh`), so development builds were ad hoc.

**The dev identity.** On the Mac, a self-signed code signing certificate, "VoxtMini Dev" (ten years), in a keychain of its own (`~/Library/Keychains/voxtmini-dev.keychain-db`, its random password in `~/.config/voxtmini-dev/sign.env`, 0600) signs development builds over ssh without the GUI. A build signed with it has the certificate as its requirement, the same on every build, so the grants stay. The login keychain is not touched.

```sh
scripts/mac.sh scripts/mac-dev-identity.sh                             # once: creates it (does nothing if it is there)
MAC_DIR=code/VoxtMini-test scripts/mac.sh scripts/mac-dev-install.sh    # build, sign, install into /Applications
```

`scripts/mac-dev-install.sh` runs the helper's Swift tests, builds the helper, copies the runtimes from `~/voxtmini-runtime` (`--runtimes DIR`), runs `go tool mygo build` (ad hoc), signs the app again with the identity (`scripts/mac-dev-identity.sh --sign`), checks that the designated requirements of the app, the helper and `voxtmini-llm` name the certificate (`--check`), then installs: a copy to `/Applications/.VoxtMini.new.app`, a running VoxtMini asked to quit (SIGTERM), the copy swapped in. It never opens the app, prints `rev <commit>` first (from `.rev` in a git archive) and `INSTALL_DONE` or `BUILD_FAILED` last, and deletes the build trees. `--dry-run` prints the steps; `VOXTMINI_MIN_FREE` (default 30) is the least free memory in percent to start with.

The dev signature has **no hardened runtime**, as an ad hoc build: a self-signed certificate has no Team ID, and the hardened runtime's library validation would refuse the app's libraries (SQLite, `llama.framework`, the Omni runtime's). That is why the script signs the finished app itself instead of `go tool mygo build -sign`, which turns the hardened runtime on for every real identity. Releases (`scripts/release.sh`, Developer ID) are signed as before.

Switching from ad hoc builds to the dev identity, or between it and a release build, costs one more grant: the old entries name the other requirement. Reset them first, so a stale Accessibility entry does not stay on without working: `tccutil reset Microphone dev.hehehai.voxtmini` and `tccutil reset Accessibility dev.hehehai.voxtmini`, then open the app and grant again. Entries of the old bundle id (`dev.hehehai.voxtnew`, before 0.1.0-beta.3) may belong to Voxt 2 as well: leave them unless Voxt 2 is not installed. Deleting the dev keychain, or creating the identity again, does the same. Downloaded models stay.

Other ways, without the identity:

- **Change only the helper:** keep one built app and run it with a helper outside the bundle. The helper inherits the app's grants wherever it lives, so rebuilding it costs nothing:
  ```sh
  scripts/build-helper.sh --out ~/voxtmini-dev
  open --env VOXTMINI_HELPER=$HOME/voxtmini-dev/voxtmini-helper build/darwin-arm64/VoxtMini.app
  ```
- **An Apple Development certificate** (in Terminal on the Mac, or over ssh once `security set-key-partition-list -S apple-tool:,apple: -s ~/Library/Keychains/login.keychain-db` has run there): `go tool mygo build -sign "Apple Development: …"` (`VOXTMINI_SIGN_IDENTITY` for `scripts/build-helper.sh` and `scripts/build-llm.sh`). It has a Team ID, so the hardened runtime that mygo turns on works.

## Tests

VoxtMini is a Go app on [MyGo](https://github.com/egoist/mygo) (native macOS UI from Go, no cgo). Go decides; the helper and the model runtimes do what they are told, over JSON lines on stdin and stdout (the helper, `voxtmini-llm`) or loopback HTTP (the Omni servers). Everything but the Swift code is plain Go and is tested on Linux, with fakes of the helper and the runtimes. Run from the repository root:

```sh
CGO_ENABLED=0 go test ./...          # everything; runs on Linux
go tool mygo vet .                   # go vet and MyGo's UI-lifetime checks
VOXTMINI_SNAPSHOTS=/tmp/shots go test -run TestSnapshots ./app   # every page as PNG, light/dark, en/zh-Hans
cd helper && swift test              # the Swift helper (on a Mac), against the same protocol fixtures
scripts/build-llm.sh --test          # voxtmini-llm's Swift tests (on a Mac)
scripts/mac.sh scripts/mac-e2e.sh    # headless Mac tests: the real helper, fake models
scripts/mac.sh scripts/mac-regress.sh   # the real models on a Mac: runtimes, eval, the packaged app
```

The store's tests use MyGo's SQLite plugin, which downloads its library once (checked against its SHA-256) or uses the file `MYGO_NATIVE_SQLITECORE` names. On Linux the overlay's pixel checks of Chinese text need a CJK font (Ubuntu: `fonts-noto-cjk`). The full list of commands is in [AGENTS.md](../AGENTS.md); the checks by hand on a Mac in [manual-test.md](manual-test.md).
