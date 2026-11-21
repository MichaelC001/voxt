<div align="center"><a name="readme-top"></a>

<img src="assets/logo.svg" width="118" alt="VoxtMini Logo">

# VoxtMini

Voice input for macOS on local models: press a shortcut in any app, speak, and the text appears at your cursor.

**English** · [简体中文](./README.zh-CN.md) · [Report Issues][github-issues-link]

[![][website-shield]][website-link]
[![][github-release-shield]][github-release-link]
[![][macos-version-shield]][macos-version-link]
[![][license-shield]][license-link]
[![][release-date-shield]][release-date-link]

<img width="1600" height="1037" alt="VoxtMini on a Mac: History, a result with its recording and a follow-up question" src="docs/images/history.webp" />

</div>

> [!NOTE]
> VoxtMini is the successor of Voxt, in the same repository: a rewrite in Go with Voxt's behaviour and fewer settings. Voxt continues as [SGLang Omni Voxt](https://github.com/sgl-project/sglang-omni/tree/main/Voxt); its history here ends at the tag `voxt-final`.

## ✨ Speak, and it is written

**Transcribe — speech to text**

- Live text while you speak, an optional clean-up by a language model (fillers out, punctuation in)
- An optional end word ("over", 完毕, 以上) stops the recording

<p align="center">
  <img width="720" alt="Transcribe: the recording overlay with the live text under a notes window" src="docs/images/transcribe.webp" />
</p>

**Translate — into English, Chinese or Japanese**

- Translate what you say, or select text and press the shortcut

<p align="center">
  <img width="720" alt="Translate: a selection translated into Chinese, in the answer card" src="docs/images/translate.webp" />
</p>

**Rewrite — say what you want**

- Select text and say how to change it ("shorter", "more formal"), or ask for new text
- Ask follow-ups in the answer card (Space starts one)

<p align="center">
  <img width="720" alt="Rewrite: a message written from a spoken request, then made shorter by a follow-up" src="docs/images/rewrite.webp" />
</p>

**App enhancement** — groups of apps and websites with their own prompt and a key pressed after inserting (a casual tone in chats, Return to send).

**Dictionary** — names and jargon, heard right and written your way; categories, usage counts, per-group terms.

**History** — every result with its recording (optional): play it, retry it, ask follow-up questions.

<p align="center">
  <img width="49%" alt="Models: the speech models, installed or to download, local or remote" src="docs/images/models.webp" />
  <img width="49%" alt="Dictionary: terms by category, with what speech recognition hears and how often each is used" src="docs/images/dictionary.webp" />
</p>

Chinese results are always in Simplified characters; Japanese text is never altered.

[![][back-to-top]](#readme-top)

## 🧠 Models

- **Local** (the default): Qwen3-ASR and Whisper for speech, Qwen3.5, Hy-MT2 and more for language, on MLX. Downloaded when you ask, shared with the other [ActNow](https://actnow.dev) apps.
- **Apple**: Apple Speech and Apple Intelligence (macOS 26), on this Mac only.
- **Remote** (only if you choose one, per feature): OpenAI, Gemini, Anthropic, DeepSeek, Doubao, Alibaba Cloud and others, or Ollama / LM Studio on your Mac.

Details: [How VoxtMini works](docs/how-it-works.md).

[![][back-to-top]](#readme-top)

## 📦 Download / Install

- [Official website](https://actnow.dev/voxtmini)
- [Download from the official website](https://actnow.dev/download/voxtmini)
- [GitHub Releases](https://github.com/hehehai/VoxtMini/releases) (tags `v0.x`; the `v1.x` ones are Voxt's)

Needs a Mac with Apple silicon and macOS 15 or later; 16 GB of memory is comfortable for the recommended models. VoxtMini is in beta: see [CHANGELOG.md](CHANGELOG.md).

**Default shortcuts**: tap right Option to transcribe, right Option + Shift to translate, Control + right Option to rewrite.

[![][back-to-top]](#readme-top)

## 🔒 Privacy

Audio and text stay on your Mac unless you choose a remote service for a feature; then only that feature's data goes to that service. Keys stay in the Keychain. No account, no server of VoxtMini's, no analytics. Feedback goes to actnow.dev only when you send it from the feedback window (**Help → Send Feedback**, [privacy policy](https://actnow.dev/legal/privacy)); you can see its diagnostics and an error report before sending, and they never hold what you said or wrote. Bugs can also be reported as public issues on GitHub.

[![][back-to-top]](#readme-top)

## 🛠 Development

[Contributing](CONTRIBUTING.md) · [Documentation](docs/README.md) · [Architecture](docs/architecture.md) · [Building](docs/development.md)

## License

Apache 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

<!-- LINK GROUP: the releases page, not /releases/latest, which is Voxt's v1.15.0 until a VoxtMini release is marked latest. -->

[back-to-top]: https://img.shields.io/badge/-BACK_TO_TOP-151515?style=flat-square
[website-link]: https://actnow.dev/voxtmini
[website-shield]: https://img.shields.io/badge/website-actnow.dev%2Fvoxtmini-0d9384?style=flat-square&labelColor=000000
[github-issues-link]: https://github.com/hehehai/VoxtMini/issues/new/choose
[github-release-link]: https://github.com/hehehai/VoxtMini/releases
[macos-version-link]: https://github.com/hehehai/VoxtMini/releases
[license-link]: ./LICENSE
[release-date-link]: https://github.com/hehehai/VoxtMini/releases
[github-release-shield]: https://img.shields.io/github/v/release/hehehai/VoxtMini?include_prereleases&filter=v0.*&label=release&labelColor=000000&color=3fb950&style=flat-square&logo=github&logoColor=white
[macos-version-shield]: https://img.shields.io/badge/macOS-15.0%2B-58a6ff?style=flat-square&labelColor=000000&logo=apple&logoColor=white
[license-shield]: https://img.shields.io/badge/License-Apache%202.0-58a6ff.svg?style=flat-square&labelColor=000000&logo=apache&logoColor=white
[release-date-shield]: https://img.shields.io/github/release-date-pre/hehehai/VoxtMini?style=flat-square&labelColor=000000&color=58a6ff&logo=github&logoColor=white
