# Contributing to VoxtMini

**English** · [简体中文](#参与贡献)

VoxtMini lives in the [hehehai/VoxtMini](https://github.com/hehehai/VoxtMini) repository and takes its issues and pull requests there, under the repository's licence, the Apache License 2.0. No contributor licence agreement is needed.

- **Bugs**: Settings → About → Report a problem opens a GitHub issue with the diagnostics filled in. Or open an [issue](https://github.com/hehehai/VoxtMini/issues) by hand: what happened, what you expected, the steps.
- **Changes**: VoxtMini does Transcribe, Translate and Rewrite, with App enhancement and the dictionary, on local models first (remote services only where the user chooses one); meetings, notes and file transcription are out of its scope. For anything larger than a fix, open an issue first.
- **Voxt**: this repository was Voxt's (its history ends at the tag `voxt-final`). Voxt itself continues as [SGLang Omni Voxt](https://github.com/sgl-project/sglang-omni/tree/main/Voxt): report Voxt's bugs there.
- Read [AGENTS.md](AGENTS.md) (the rules for changes) and [docs/architecture.md](docs/architecture.md) before changing code.

## Pull requests

1. Keep a pull request to one change, with tests. Commit messages and code comments are in English.
2. Every user-visible string goes through `i18n.T` with an English message, and its Simplified Chinese translation goes into `internal/i18n/locales/zh-Hans.json`.
3. A change users notice gets a line in the `Unreleased` section of [CHANGELOG.md](CHANGELOG.md), in English and Chinese.
4. Run the checks below, and say in the pull request what changed and how you checked it. For a change in the UI, add before and after screenshots.

## Development

VoxtMini is a Go app on [MyGo](https://github.com/egoist/mygo) (native UI, no cgo) with a small Swift helper. Everything but the helper is plain Go and is tested on Linux; building the app needs a Mac with macOS 15 or later on Apple silicon and Xcode. Go 1.27.1 (`GOTOOLCHAIN=auto` fetches it). Run these from the repository root:

```sh
CGO_ENABLED=0 go test ./...                                  # all tests (Linux and Mac)
go tool mygo vet .                                           # go vet and MyGo's UI checks
git ls-files '*.go' ':!:third_party/**' | xargs gofmt -l    # must print nothing
go test -run TestKeyParity ./internal/i18n                   # strings and zh-Hans
VOXTMINI_SNAPSHOTS=/tmp/shots go test -run TestSnapshots .   # every page as PNG
cd helper && swift test                                      # the Swift helper (on a Mac)
scripts/build-helper.sh && scripts/build-llm.sh && go tool mygo build   # build the app (on a Mac)
scripts/release.sh                                           # the release build, runtimes inside (docs/release.md)
```

The store's tests use MyGo's SQLite plugin, which downloads its library once (checked against its SHA-256) or uses `MYGO_SQLITE_LIBRARY`. On Linux the overlay's pixel checks of Chinese text need a CJK font (Ubuntu: `fonts-noto-cjk`). What needs permissions or a screen is checked by hand: [docs/manual-test.md](docs/manual-test.md).

---

## 参与贡献

VoxtMini 在 [hehehai/VoxtMini](https://github.com/hehehai/VoxtMini) 仓库里，问题和 PR 都提到那里，按仓库的 Apache License 2.0 授权，不需要签贡献者协议。

- **报告问题**：设置 → 诊断与关于 → 反馈问题，会打开一个已填好诊断信息的 GitHub issue；也可以手动开 [issue](https://github.com/hehehai/VoxtMini/issues)，写清发生了什么、你期望什么、复现步骤。
- **改动**：VoxtMini 只做转录、翻译、转写，加上应用增强和词典，以本地模型为主（用户选了才用远程服务）；会议、笔记、文件转录不在范围内。比修 bug 更大的改动，先开 issue 讨论。
- **Voxt**：这个仓库原来是 Voxt 的（它的历史到标签 `voxt-final` 为止）。Voxt 本身在 [SGLang Omni Voxt](https://github.com/sgl-project/sglang-omni/tree/main/Voxt) 继续开发，Voxt 的问题请提到那里。
- 改代码前先读 [AGENTS.md](AGENTS.md)（改动规则）和 [docs/architecture.md](docs/architecture.md)。
- PR：一个 PR 一件事，带测试；提交信息和代码注释用英文；界面文字都经过 `i18n.T`，并在 `internal/i18n/locales/zh-Hans.json` 补中文；用户能察觉的改动在 [CHANGELOG.md](CHANGELOG.md) 的 `Unreleased` 里中英文各写一行；跑一遍上面的检查。
