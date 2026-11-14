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

macOS keeps the Microphone and Accessibility grants for a signature. An ad hoc signed build (the default) has a new one each time the app is rebuilt, so after a rebuild macOS forgets them: the microphone turns **denied without asking again**, and Accessibility still shows on in System Settings but does not work. The helper's grants are the app's (macOS checks the helper against VoxtMini.app, its responsible process), so the same happens to it. Ways around it:

- **Change only the helper:** keep one built app and run it with a helper outside the bundle. The helper inherits the app's grants wherever it lives, so rebuilding it costs nothing:
  ```sh
  scripts/build-helper.sh --out ~/voxtmini-dev
  open --env VOXTMINI_HELPER=$HOME/voxtmini-dev/voxtmini-helper build/darwin-arm64/VoxtMini.app
  ```
- **Start over:** `tccutil reset Microphone dev.hehehai.voxtnew` (likewise `Accessibility`), then open the app and grant again. If a stale Accessibility entry stays on but does nothing, remove it with − in System Settings and grant again.
- **Sign with a fixed identity:** an Apple Development certificate keeps the grants across rebuilds: `go tool mygo build -sign "Apple Development: …"` (or `macos.signingIdentity` in a local mygo.json; `VOXTMINI_SIGN_IDENTITY` for `scripts/build-helper.sh` and `scripts/build-llm.sh`). Over ssh, `codesign` fails with `errSecInternalComponent` until the key may be used without the GUI: once, in Terminal on the Mac, `security set-key-partition-list -S apple-tool:,apple: -s ~/Library/Keychains/login.keychain-db`.

Switching between your own builds and a release build (Developer ID) needs the same reset once. Downloaded models stay.

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

The store's tests use MyGo's SQLite plugin, which downloads its library once (checked against its SHA-256) or uses `MYGO_SQLITE_LIBRARY`. On Linux the overlay's pixel checks of Chinese text need a CJK font (Ubuntu: `fonts-noto-cjk`). The full list of commands is in [AGENTS.md](../AGENTS.md); the checks by hand on a Mac in [manual-test.md](manual-test.md).
