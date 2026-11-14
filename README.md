# VoxtMini

**English** · [简体中文](README.zh-CN.md)

Voice input for macOS, on **local models** first: press a shortcut in any app, speak, and the text appears where your cursor is. Remote services are used only for a feature where you choose one.

**VoxtMini is the successor of Voxt, in the same repository.** Voxt, the Swift app this repository held until 2026, lives on as [SGLang Omni Voxt](https://github.com/sgl-project/sglang-omni/tree/main/Voxt); its whole history is kept here before the VoxtMini commit (the last Voxt commit is `a28d0c8`, tagged `voxt-final`; Voxt's releases, v1.x, stay on the [Releases](https://github.com/hehehai/VoxtMini/releases) page). VoxtMini is a rewrite in Go on [MyGo](https://github.com/egoist/mygo) with a small Swift helper: Voxt's behaviour, with fewer settings.

## Features

- **转录 Transcribe**: what you say, typed where your cursor is, with live text while you speak and an optional clean-up by a language model (fillers and false starts out, punctuation in). An optional end word ("over", 完毕, 以上) stops the recording.
- **翻译 Translate**: the selected text, or what you say, into English, Simplified Chinese or Japanese.
- **转写 Rewrite**: change the selected text by saying how ("shorter", "more formal"), or have new text written from a spoken request; ask follow-ups in the answer card (Space starts one).
- **应用增强 App enhancement**: groups of apps and websites with their own scene prompt and a key pressed after inserting (a casual tone in chats, Return to send).
- **词典 Dictionary**: names and jargon the speech model expects and the language model writes as you spell them; categories, usage counts, terms limited to an App enhancement group, and the corrections shown in the history.
- **History**: every result with its recording (optional), played back in place; retry a recording, or ask follow-up questions about an entry.
- **Shortcuts**: keys or mouse buttons, with presets (fn included); Escape cancels; a menu bar icon with a microphone switch.
- Chinese results are always in Simplified characters; Japanese text is never altered. The app speaks English and Simplified Chinese; dictation works in English, Chinese and Japanese.

**Default shortcuts** (each feature's page changes them): tap the **right Option** key to transcribe, right Option with Shift to translate, Control with right Option to rewrite; Control-Option-V pastes the last result. VoxtMini is one of the [ActNow](https://actnow.dev) apps: these keys leave fn to Auralink, so the two run side by side.

## Models

**Local** (the default; "Automatic" always means a local model):

- **Speech recognition**: Qwen3-ASR (accurate 1.7B, recommended; fast 0.6B), with live text, on sglang-omni's native MLX servers ([`sglang_omni_mlx/native`](sglang_omni_mlx/native), C++, no Python); Whisper (large-v3-turbo, large-v3, small; experimental, no live text); **Apple Speech**, on this Mac only.
- **Language models**: Qwen3.5 4B (balanced, recommended) and 2B (fast), Hy-MT2 for translation, more MLX models marked experimental (Gemma 4, Ministral 3, LFM2…), in `voxtmini-llm`, a Swift process of VoxtMini's own (MLX, llama.cpp for GGUF); **Apple Intelligence** on macOS 26.
- Models download from Hugging Face (or a mirror) when you ask, each file checked against a pinned SHA-256, into the model library shared with the other ActNow apps. A model stops after a few minutes unused (Models → Advanced changes it).

**Remote** (set up on the Models page, chosen per feature): OpenAI, Doubao, Alibaba Cloud, Gemini Live, StepFun, Z.ai, Xiaomi MiMo for speech; Anthropic, OpenAI (also the Responses API, with web search for Rewrite), Codex with your ChatGPT sign-in, Gemini, DeepSeek, OpenRouter and others, or Ollama / LM Studio / oMLX on your Mac, for language. API keys can be shared with the other ActNow apps.

## Requirements

A Mac with Apple silicon and macOS 15 or later (Apple Intelligence and the newer Apple Speech: macOS 26). The app is about 280 MB; the recommended pair of models is a 5.8 GB download and takes about 7 GB of memory while both are loaded, so 16 GB of memory is comfortable; on an 8 GB Mac choose the fast models or Apple's.

## Download

VoxtMini's builds are published on [GitHub Releases](https://github.com/hehehai/VoxtMini/releases) as notarised Apple silicon disk images (tags `v0.x`; the `v1.x` releases there are Voxt's). Installed copies update from [actnow.dev](https://actnow.dev), with a Beta channel in Settings → About.

> **Status: beta** (latest release `0.1.0-beta.2`; what came since is in [CHANGELOG.md](CHANGELOG.md) under Unreleased). Not yet checked on a real Mac: Apple Speech and Apple Intelligence, and Gemini Live, Codex sign-in and the Responses API against the real services; models marked experimental have not been run on a Mac with VoxtMini.

## Privacy

By default audio and text never leave the Mac: recognition, clean-up, translation and rewriting all run on it. A feature for which you choose a remote service sends its recordings or text to that service, and only that one; its key stays in the macOS Keychain. VoxtMini has no server of its own, no account and no analytics. Otherwise it goes online only to download the models you choose (from Hugging Face through the system's proxy, or hf-mirror.com) and to check actnow.dev for updates and announcements. The history, its optional recordings, the dictionary and the settings stay in `~/Library/Application Support/VoxtNew/` (the name of VoxtMini's beta, kept so that existing installs keep their data; likewise the bundle id `dev.hehehai.voxtnew` and the `voxtnew://` links).

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

**The release build** is one command, on the Mac: `scripts/release.sh` builds the three, packages and signs the app, and checks the result (`--runtimes DIR` takes prebuilt runtimes; with the ActNow apps' Developer ID, `--notarize` and `--publish` notarise it and publish it on actnow.dev; over ssh it runs through `scripts/mac-gui.sh`). See [docs/release.md](docs/release.md).

From Linux, `scripts/mac.sh <command>` mirrors the project to a Mac over ssh (`MAC_HOST`, an ssh host name; `MAC_DIR`, default `code/VoxtMini`) and runs the command there: `scripts/mac.sh 'scripts/build-helper.sh && scripts/build-llm.sh && go tool mygo build'`.

<details>
<summary>Signing during development (keeping the permissions across rebuilds)</summary>

macOS keeps the Microphone and Accessibility grants for a signature. An ad hoc signed build (the default) has a new one each time the app is rebuilt, so after a rebuild macOS forgets them: the microphone turns **denied without asking again**, and Accessibility still shows on in System Settings but does not work. The helper's grants are the app's (macOS checks the helper against VoxtMini.app, its responsible process), so the same happens to it. Ways around it:

- **Change only the helper:** keep one built app and run it with a helper outside the bundle. The helper inherits the app's grants wherever it lives, so rebuilding it costs nothing:
  ```sh
  scripts/build-helper.sh --out ~/voxtmini-dev
  open --env VOXTMINI_HELPER=$HOME/voxtmini-dev/voxtmini-helper build/darwin-arm64/VoxtMini.app
  ```
- **Start over:** `tccutil reset Microphone dev.hehehai.voxtnew` (likewise `Accessibility`), then open the app and grant again. If a stale Accessibility entry stays on but does nothing, remove it with − in System Settings and grant again.
- **Sign with a fixed identity:** an Apple Development certificate keeps the grants across rebuilds: `go tool mygo build -sign "Apple Development: …"` (or `macos.signingIdentity` in a local mygo.json; `VOXTMINI_SIGN_IDENTITY` for `scripts/build-helper.sh` and `scripts/build-llm.sh`). Over ssh, `codesign` fails with `errSecInternalComponent` until the key may be used without the GUI: once, in Terminal on the Mac, `security set-key-partition-list -S apple-tool:,apple: -s ~/Library/Keychains/login.keychain-db`.

Switching between your own builds and a release build (Developer ID) needs the same reset once ([docs/release.md](docs/release.md) §4). Downloaded models stay.

</details>

## Development

VoxtMini is a Go app on [MyGo](https://github.com/egoist/mygo) (native macOS UI from Go, no cgo). Go decides; the helper and the model runtimes do what they are told, over JSON lines on stdin and stdout (the helper, `voxtmini-llm`) or loopback HTTP (the Omni servers). Everything but the Swift code is plain Go and is tested on Linux, with fakes of the helper and the runtimes. Run from the repository root:

```sh
CGO_ENABLED=0 go test ./...          # everything; runs on Linux
go tool mygo vet .                   # go vet and MyGo's UI-lifetime checks
VOXTMINI_SNAPSHOTS=/tmp/shots go test -run TestSnapshots .   # every page as PNG, light/dark, en/zh-Hans
cd helper && swift test              # the Swift helper (on a Mac), against the same protocol fixtures
scripts/build-llm.sh --test          # voxtmini-llm's Swift tests (on a Mac)
scripts/mac.sh scripts/mac-e2e.sh    # headless Mac tests: the real helper, fake models
scripts/mac.sh scripts/mac-regress.sh   # the real models on a Mac: runtimes, eval, the packaged app
```

The store's tests use MyGo's SQLite plugin, which downloads its library once (checked against its SHA-256) or uses `MYGO_SQLITE_LIBRARY`. On Linux the overlay's pixel checks of Chinese text need a CJK font (Ubuntu: `fonts-noto-cjk`).

- [docs/architecture.md](docs/architecture.md): packages, threading, the session, the runtimes, data on disk, tests.
- [docs/plan.md](docs/plan.md): what VoxtMini is and its milestones.
- [docs/behavior.md](docs/behavior.md): what VoxtMini does differently from Voxt, and why.
- [docs/models-page.md](docs/models-page.md), [docs/remote-models.md](docs/remote-models.md), [docs/apple-models.md](docs/apple-models.md): the Models page, the remote services, Apple's on-device models.
- [docs/llm-models.md](docs/llm-models.md): the language models, their settings and measurements.
- [docs/eval.md](docs/eval.md): the models' quality on an eval set, and the decisions it led to.
- [docs/helper-protocol.md](docs/helper-protocol.md), [docs/llm-protocol.md](docs/llm-protocol.md): the helper's and `voxtmini-llm`'s protocols.
- [docs/manual-test.md](docs/manual-test.md): the checks by hand on a Mac (in Chinese), and the automated Mac tests.
- [docs/release.md](docs/release.md): building, signing, notarising and publishing a release; updates (in Chinese).
- [AGENTS.md](AGENTS.md): the rules for changes (strings, threading, the protocols).
- [CHANGELOG.md](CHANGELOG.md): changes by version. [CONTRIBUTING.md](CONTRIBUTING.md): how to send a change.

VoxtMini started as a copy of [Auralink](https://github.com/hehehai/Auralink), the same author's app on remote models, which rewrote Voxt on Go and MyGo with [AudioSmith](https://github.com/hehehai/AudioSmith)'s interface; its local model runtimes and their request policies follow Voxt's (in this repository's history up to `voxt-final`).

## Licence

Apache License 2.0 ([LICENSE](LICENSE)), as the rest of this repository. Copyright ActNow and the SGLang-Omni Team. Third-party code, runtimes and their licences are listed in [NOTICE](NOTICE); the models are downloaded under their own licences.
