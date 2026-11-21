# How VoxtMini works

**English** · [简体中文](#voxtmini-如何工作)

What runs on your Mac, which models VoxtMini uses, what goes online and where your data is kept. For the code, see [architecture.md](architecture.md).

## Features in detail

- **Transcribe**: what you say, typed where your cursor is, with live text while you speak and an optional clean-up by a language model (fillers and false starts out, punctuation in). An optional end word ("over", 完毕, 以上) stops the recording.
- **Translate**: the selected text, or what you say, into English, Simplified Chinese or Japanese.
- **Rewrite**: change the selected text by saying how ("shorter", "more formal"), or have new text written from a spoken request; ask follow-ups in the answer card (Space starts one).
- **App enhancement**: groups of apps and websites with their own scene prompt and a key pressed after inserting (a casual tone in chats, Return to send).
- **Dictionary**: names and jargon the speech model expects and the language model writes as you spell them; categories, usage counts, terms limited to an App enhancement group, and the corrections shown in the history.
- **History**: every result with its recording (optional), played back in place; retry a recording, or ask follow-up questions about an entry.
- **Shortcuts**: keys or mouse buttons, with presets (fn included); Escape cancels; a menu bar icon with a microphone switch.
- Chinese results are always in Simplified characters; Japanese text is never altered. The app speaks English and Simplified Chinese; dictation works in English, Chinese and Japanese.

**Default shortcuts** (each feature's page changes them): tap the **right Option** key to transcribe, right Option with Shift to translate, Control with right Option to rewrite; Control-Option-V pastes the last result. VoxtMini is one of the [ActNow](https://actnow.dev) apps: these keys leave fn to Auralink, so the two run side by side.

## Models

**Local** (the default; "Automatic" always means a local model):

- **Speech recognition**: Qwen3-ASR (accurate 1.7B, recommended; fast 0.6B), with live text, on sglang-omni's native MLX servers ([`sglang_omni_mlx/native`](../sglang_omni_mlx/native), C++, no Python); Whisper (large-v3-turbo, large-v3, small; experimental, no live text); **Apple Speech**, on this Mac only.
- **Language models**: Qwen3.5 4B (balanced, recommended) and 2B (fast), Hy-MT2 for translation, more MLX models marked experimental (Gemma 4, Ministral 3, LFM2 and others), in `voxtmini-llm`, a Swift process of VoxtMini's own (MLX, llama.cpp for GGUF); **Apple Intelligence** on macOS 26.
- Models download from Hugging Face (or a mirror) when you ask, each file checked against a pinned SHA-256, into the model library shared with the other ActNow apps. A model stops after a few minutes unused, sooner under memory pressure (Models → Advanced changes it).

**Remote** (set up on the Models page, chosen per feature): OpenAI, Doubao, Alibaba Cloud, Gemini Live, StepFun, Z.ai, Xiaomi MiMo for speech; Anthropic, OpenAI (also the Responses API, with web search for Rewrite), Gemini, DeepSeek, OpenRouter and others, or Ollama / LM Studio / oMLX on your Mac, for language. API keys can be shared with the other ActNow apps.

More: [models-page.md](models-page.md), [remote-models.md](remote-models.md), [apple-models.md](apple-models.md), [llm-models.md](llm-models.md).

## Requirements

A Mac with Apple silicon and macOS 15 or later (Apple Intelligence and the newer Apple Speech: macOS 26). The app is about 280 MB; the recommended pair of models is a 5.8 GB download and takes about 7 GB of memory while both are loaded, so 16 GB of memory is comfortable; on an 8 GB Mac choose the fast models or Apple's.

## Download and updates

Download VoxtMini from [actnow.dev/voxtmini](https://actnow.dev/voxtmini) ([download](https://actnow.dev/download/voxtmini), with a free ActNow account), or from [GitHub Releases](https://github.com/hehehai/VoxtMini/releases), both notarised Apple silicon disk images (tags `v0.x`; the `v1.x` releases there are Voxt's). Installed copies update from actnow.dev (`/updates/voxtmini`), with a Beta channel in Settings → About.

> **Status: beta** (the releases and what changed are in [CHANGELOG.md](../CHANGELOG.md)). Not yet checked on a real Mac: Apple Speech and Apple Intelligence, and Gemini Live and the Responses API against the real services; models marked experimental have not been run on a Mac with VoxtMini.

## Privacy

By default audio and text never leave the Mac: recognition, clean-up, translation and rewriting all run on it. A feature for which you choose a remote service sends its recordings or text to that service, and only that one; its key stays in the macOS Keychain. VoxtMini has no server of its own, no account and no analytics. Otherwise it goes online only to download the models you choose (from Hugging Face through the system's proxy, or hf-mirror.com) and to check actnow.dev for updates and announcements. The history, its optional recordings, the dictionary and the settings stay in `~/Library/Application Support/VoxtMini/`. Betas before 0.1.0-beta.3 kept them in `…/VoxtNew/` under the bundle id `dev.hehehai.voxtnew`, which another app (Voxt 2) also uses; the first start of a newer VoxtMini moves its own files over and leaves anything else there alone. macOS then asks once more for the microphone and Accessibility.

## From Voxt to VoxtMini

This repository held Voxt, the Swift app, until 2026; Voxt lives on as [SGLang Omni Voxt](https://github.com/sgl-project/sglang-omni/tree/main/Voxt). Its whole history is kept here before the VoxtMini commit (the last Voxt commit is `a28d0c8`, tagged `voxt-final`; Voxt's releases, v1.x, stay on the [Releases](https://github.com/hehehai/VoxtMini/releases) page). VoxtMini is a rewrite in Go on [MyGo](https://github.com/egoist/mygo) with a small Swift helper: Voxt's behaviour, with fewer settings ([behavior.md](behavior.md) lists the differences).

VoxtMini started as a copy of [Auralink](https://github.com/hehehai/Auralink), the same author's app on remote models, which rewrote Voxt on Go and MyGo with [AudioSmith](https://github.com/hehehai/AudioSmith)'s interface; its local model runtimes and their request policies follow Voxt's.

---

## VoxtMini 如何工作

[English](#how-voxtmini-works) · **简体中文**

本机上运行什么、VoxtMini 用哪些模型、什么会联网、数据放在哪里。代码结构见 [architecture.md](architecture.md)。

### 功能详解

- **转录**：把说的话输入到光标处，说话时实时显示文字，可以再由语言模型整理（去掉口头语和说了一半的话，补上标点）。可以设一个结束词（“over”“完毕”“以上”）说完就停止录音。
- **翻译**：把选中的文字或说的话译成英文、简体中文或日文。
- **转写**：说一句怎么改（“短一点”“正式一点”）来改写选中的文字，或者按语音要求写新内容；在答案卡片里继续追问（按空格开始）。
- **应用增强**：把 App 和网站分组，每组有自己的场景提示词和插入后的按键（聊天时口语一些，插入后按回车发送）。
- **词典**：人名和专业术语，语音识别模型提前知道，语言模型照你的写法输出；支持分类、使用次数、只在某个应用增强分组里生效，历史里标出词典做的更正。
- **历史**：每条结果连同录音（可选），可以直接播放；可以用录音重试，也可以针对一条记录继续提问。
- **快捷键**：键盘或鼠标按键，有预设（包括 fn）；Esc 取消；菜单栏图标里可以切换麦克风。
- 中文结果一律是简体字；日文不做任何改动。界面有英文和简体中文；听写支持英文、中文和日文。

**默认快捷键**（在各功能页里可以改）：单按**右 Option** 转录，右 Option 加 Shift 翻译，Control 加右 Option 转写；Control-Option-V 粘贴上一条结果。VoxtMini 是 [ActNow](https://actnow.dev) 家族的 app：这套按键把 fn 留给 Auralink，两个 app 可以同时使用。

### 模型

**本地**（默认；“自动”总是指本地模型）：

- **语音识别**：Qwen3-ASR（精准 1.7B，推荐；快速 0.6B），实时显示文字，跑在 sglang-omni 的原生 MLX 服务上（[`sglang_omni_mlx/native`](../sglang_omni_mlx/native)，C++，不用 Python）；Whisper（large-v3-turbo、large-v3、small；实验性，没有实时文字）；**Apple 语音识别**，只在本机。
- **语言模型**：Qwen3.5 4B（均衡，推荐）和 2B（快速）、翻译专用的 Hy-MT2，以及标为实验性的更多 MLX 模型（Gemma 4、Ministral 3、LFM2 等），跑在 VoxtMini 自己的 Swift 进程 `voxtmini-llm` 里（MLX，GGUF 用 llama.cpp）；macOS 26 上还有 **Apple 智能**。
- 模型在你需要时从 Hugging Face（或镜像）下载，每个文件都按固定的 SHA-256 校验，放在与其他 ActNow app 共用的模型库里。模型闲置几分钟后停止，内存紧张时更早（在 模型 → 高级 里可以改）。

**远程**（在模型页设置，按功能选择）：语音识别有 OpenAI、豆包、阿里云百炼、Gemini Live、阶跃星辰、智谱、小米 MiMo；语言模型有 Anthropic、OpenAI（也可用 Responses API，转写时可联网搜索）、Gemini、DeepSeek、OpenRouter 等，或者本机的 Ollama / LM Studio / oMLX。API 密钥可以与其他 ActNow app 共用。

### 系统要求

Apple 芯片、macOS 15 及以上的 Mac（Apple 智能和新版 Apple 语音识别需要 macOS 26）。app 约 280 MB；推荐的两个模型共需下载 5.8 GB，同时加载时约占 7 GB 内存，16 GB 内存的 Mac 比较从容；8 GB 内存的 Mac 请选快速模型或 Apple 的模型。

### 下载和更新

从 [actnow.dev/voxtmini](https://actnow.dev/voxtmini) 下载（[下载页](https://actnow.dev/download/voxtmini)，需要免费的 ActNow 账户），或从 [GitHub Releases](https://github.com/hehehai/VoxtMini/releases) 下载，都是经过公证的 Apple 芯片版磁盘映像（标签 `v0.x`；那里的 `v1.x` 是 Voxt 的版本）。装好后从 actnow.dev（`/updates/voxtmini`）更新，设置 → 诊断与关于 里有 Beta 通道。

> **状态：beta**（各版本和改动见 [CHANGELOG.md](../CHANGELOG.md)）。尚未在真机上验证：Apple 语音识别和 Apple 智能，以及 Gemini Live 和 Responses API 对真实服务的调用；标为实验性的模型还没在 Mac 上用 VoxtMini 跑过。

### 隐私

默认情况下音频和文字不会离开这台 Mac：识别、整理、翻译和改写都在本机完成。你为某个功能选了远程服务时，这个功能的录音或文字只发给那一个服务；密钥保存在 macOS 钥匙串里。VoxtMini 没有自己的服务器，不需要账号，也不做统计。除此之外它只为两件事联网：下载你选择的模型（从 Hugging Face，走系统代理，或用 hf-mirror.com），以及在 actnow.dev 检查更新和公告。历史、可选保存的录音、词典和设置都在 `~/Library/Application Support/VoxtMini/`。0.1.0-beta.3 之前的测试版放在 `…/VoxtNew/`，bundle id 是 `dev.hehehai.voxtnew`，另一个 app（Voxt 2）也用这个 id；新版 VoxtMini 第一次启动时把自己的文件搬过来，那里别的东西不动。之后 macOS 会再请求一次麦克风和辅助功能权限。

### 从 Voxt 到 VoxtMini

这个仓库到 2026 年为止放的是 Swift 写的 Voxt，它现在以 [SGLang Omni Voxt](https://github.com/sgl-project/sglang-omni/tree/main/Voxt) 的形式继续开发；Voxt 的全部历史都保留在 VoxtMini 那次提交之前（Voxt 的最后一次提交是 `a28d0c8`，标签 `voxt-final`；Voxt 的 v1.x 版本仍在 [Releases](https://github.com/hehehai/VoxtMini/releases) 页面）。VoxtMini 用 Go（基于 [MyGo](https://github.com/egoist/mygo)）加一个小的 Swift 辅助进程重写：保留 Voxt 的行为，设置更少（不同之处见 [behavior.md](behavior.md)）。

VoxtMini 由 [Auralink](https://github.com/hehehai/Auralink) 复制而来。Auralink 是同一作者基于远程模型的应用，它用 Go 和 MyGo 以 [AudioSmith](https://github.com/hehehai/AudioSmith) 的界面重写了 Voxt；VoxtMini 的本地模型运行时和请求策略沿用 Voxt 的做法。
