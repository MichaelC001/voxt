# VoxtMini

[English](README.md) · **简体中文**

**以本地模型为主**的 macOS 语音输入：在任意 App 里按下快捷键说话，文字出现在光标处。只有你为某个功能选了远程服务时，才会用远程服务。

**VoxtMini 是 Voxt 的继任者，就在这个仓库里。** 这个仓库到 2026 年为止放的是 Swift 写的 Voxt，它现在以 [SGLang Omni Voxt](https://github.com/sgl-project/sglang-omni/tree/main/Voxt) 的形式继续开发；Voxt 的全部历史都保留在 VoxtMini 那次提交之前（Voxt 的最后一次提交是 `a28d0c8`，标签 `voxt-final`；Voxt 的 v1.x 版本仍在 [Releases](https://github.com/hehehai/VoxtMini/releases) 页面）。VoxtMini 用 Go（基于 [MyGo](https://github.com/egoist/mygo)）加一个小的 Swift 辅助进程重写：保留 Voxt 的行为，设置更少。

## 功能

- **转录**：把说的话输入到光标处，说话时实时显示文字，可以再由语言模型整理（去掉口头语和说了一半的话，补上标点）。可以设一个结束词（“over”“完毕”“以上”）说完就停止录音。
- **翻译**：把选中的文字或说的话译成英文、简体中文或日文。
- **转写**：说一句怎么改（“短一点”“正式一点”）来改写选中的文字，或者按语音要求写新内容；在答案卡片里继续追问（按空格开始）。
- **应用增强**：把 App 和网站分组，每组有自己的场景提示词和插入后的按键（聊天时口语一些，插入后按回车发送）。
- **词典**：人名和专业术语，语音识别模型提前知道，语言模型照你的写法输出；支持分类、使用次数、只在某个应用增强分组里生效，历史里标出词典做的更正。
- **历史**：每条结果连同录音（可选），可以直接播放；可以用录音重试，也可以针对一条记录继续提问。
- **快捷键**：键盘或鼠标按键，有预设（包括 fn）；Esc 取消；菜单栏图标里可以切换麦克风。
- 中文结果一律是简体字；日文不做任何改动。界面有英文和简体中文；听写支持英文、中文和日文。

**默认快捷键**（在各功能页里可以改）：单按**右 Option** 转录，右 Option 加 Shift 翻译，Control 加右 Option 转写；Control-Option-V 粘贴上一条结果。VoxtMini 是 [ActNow](https://actnow.dev) 家族的 app：这套按键把 fn 留给 Auralink，两个 app 可以同时使用。

## 模型

**本地**（默认；“自动”总是指本地模型）：

- **语音识别**：Qwen3-ASR（精准 1.7B，推荐；快速 0.6B），实时显示文字，跑在 sglang-omni 的原生 MLX 服务上（[`sglang_omni_mlx/native`](sglang_omni_mlx/native)，C++，不用 Python）；Whisper（large-v3-turbo、large-v3、small；实验性，没有实时文字）；**Apple 语音识别**，只在本机。
- **语言模型**：Qwen3.5 4B（均衡，推荐）和 2B（快速）、翻译专用的 Hy-MT2，以及标为实验性的更多 MLX 模型（Gemma 4、Ministral 3、LFM2 等），跑在 VoxtMini 自己的 Swift 进程 `voxtmini-llm` 里（MLX，GGUF 用 llama.cpp）；macOS 26 上还有 **Apple 智能**。
- 模型在你需要时从 Hugging Face（或镜像）下载，每个文件都按固定的 SHA-256 校验，放在与其他 ActNow app 共用的模型库里。模型闲置几分钟后停止（在 模型 → 高级 里可以改）。

**远程**（在模型页设置，按功能选择）：语音识别有 OpenAI、豆包、阿里云百炼、Gemini Live、阶跃星辰、智谱、小米 MiMo；语言模型有 Anthropic、OpenAI（也可用 Responses API，转写时可联网搜索）、用 ChatGPT 账号登录的 Codex、Gemini、DeepSeek、OpenRouter 等，或者本机的 Ollama / LM Studio / oMLX。API 密钥可以与其他 ActNow app 共用。

## 系统要求

Apple 芯片、macOS 15 及以上的 Mac（Apple 智能和新版 Apple 语音识别需要 macOS 26）。app 约 280 MB；推荐的两个模型共需下载 5.8 GB，同时加载时约占 7 GB 内存，16 GB 内存的 Mac 比较从容；8 GB 内存的 Mac 请选快速模型或 Apple 的模型。

## 下载

VoxtMini 的构建发布在 [GitHub Releases](https://github.com/hehehai/VoxtMini/releases)，是经过公证的 Apple 芯片版磁盘映像（标签 `v0.x`；那里的 `v1.x` 是 Voxt 的版本）。装好后从 [actnow.dev](https://actnow.dev) 更新，设置 → 诊断与关于 里有 Beta 通道。

> **状态：beta**（最新发布 `0.1.0-beta.2`；之后的改动见 [CHANGELOG.md](CHANGELOG.md) 的 Unreleased）。尚未在真机上验证：Apple 语音识别和 Apple 智能，以及 Gemini Live、Codex 登录和 Responses API 对真实服务的调用；标为实验性的模型还没在 Mac 上用 VoxtMini 跑过。

## 隐私

默认情况下音频和文字不会离开这台 Mac：识别、整理、翻译和改写都在本机完成。你为某个功能选了远程服务时，这个功能的录音或文字只发给那一个服务；密钥保存在 macOS 钥匙串里。VoxtMini 没有自己的服务器，不需要账号，也不做统计。除此之外它只为两件事联网：下载你选择的模型（从 Hugging Face，走系统代理，或用 hf-mirror.com），以及在 actnow.dev 检查更新和公告。历史、可选保存的录音、词典和设置都在 `~/Library/Application Support/VoxtNew/`（VoxtMini 测试版时的名字，保留它是为了让已安装的版本保留数据；bundle id `dev.hehehai.voxtnew` 和 `voxtnew://` 链接同理）。

## 从源码构建

需要 Go 1.27.4（`GOTOOLCHAIN=auto` 会自动下载）；构建 app 需要 Apple 芯片、macOS 15 及以上的 Mac，Xcode（编译辅助进程和 `voxtmini-llm`），以及 [uv](https://docs.astral.sh/uv/) 或 Python 3（Omni 运行时的构建工具：`scripts/build-omni.sh` 用固定版本的 MLX 编译 `sglang_omni_mlx/native`，CMake 会从 GitHub 下载 C++ 依赖，需要时走 shell 的 `https_proxy`）。在仓库根目录执行：

```sh
scripts/build-helper.sh              # Swift 辅助进程，输出到 resources/darwin-arm64/helper/
scripts/build-omni.sh                # Omni 运行时（语音识别模型），输出到 resources/darwin-arm64/omni/
scripts/build-llm.sh                 # voxtmini-llm（语言模型），输出到 resources/darwin-arm64/llm/
go tool mygo dev                     # 热重载运行
go tool mygo build                   # 生成 build/darwin-arm64/VoxtMini.app 和 dmg
```

`go tool mygo build` 和 `mygo dev` 之前必须先编好辅助进程和 `voxtmini-llm`：`mygo.json` 给这两者指定了 entitlement，缺了就会失败。没有 Omni 运行时 app 也能运行，但不能识别语音（也可以把 `VOXTMINI_OMNI_RUNTIME` 指向 `scripts/build-omni.sh --out DIR` 编出的运行时；`VOXTMINI_LLM` 对应 `scripts/build-llm.sh --out DIR`）。

**发布版本**在 Mac 上用一条命令构建：`scripts/release.sh` 编译这三样，打包、签名 app 并检查结果（`--runtimes DIR` 直接用编好的运行时；有 ActNow 家族 app 的 Developer ID 时，`--notarize` 和 `--publish` 公证并发布到 actnow.dev；ssh 里经 `scripts/mac-gui.sh` 运行）。见 [docs/release.md](docs/release.md)。

在 Linux 上，`scripts/mac.sh <命令>` 会通过 ssh 把项目同步到 Mac（`MAC_HOST` 是 ssh 主机名，`MAC_DIR` 默认 `code/VoxtMini`）并在那里执行，例如 `scripts/mac.sh 'scripts/build-helper.sh && scripts/build-llm.sh && go tool mygo build'`。

<details>
<summary>开发时的签名（重新构建后保留权限）</summary>

macOS 按签名记住麦克风和辅助功能的授权。ad hoc 签名的构建（默认）每次重新构建签名都会变，所以重建以后 macOS 会忘掉授权：麦克风变成**拒绝且不再询问**，辅助功能在系统设置里看着是开的，却不起作用。辅助进程的授权就是 app 的（macOS 把 VoxtMini.app 当作它的责任进程），所以它也一样。解决办法：

- **只改辅助进程时**：保留一个构建好的 app，用 bundle 外面的辅助进程运行它。辅助进程放在哪里都继承 app 的授权，重编它不受影响：
  ```sh
  scripts/build-helper.sh --out ~/voxtmini-dev
  open --env VOXTMINI_HELPER=$HOME/voxtmini-dev/voxtmini-helper build/darwin-arm64/VoxtMini.app
  ```
- **重新开始**：`tccutil reset Microphone dev.hehehai.voxtnew`（`Accessibility` 同理），再打开 app 重新授权。如果旧的辅助功能记录开着却不起作用，在系统设置里用 − 删掉再重新授权。
- **用固定的身份签名**：Apple Development 证书能让授权在重建后保留：`go tool mygo build -sign "Apple Development: …"`（或者在本地的 mygo.json 里写 `macos.signingIdentity`；`scripts/build-helper.sh` 和 `scripts/build-llm.sh` 用 `VOXTMINI_SIGN_IDENTITY`）。通过 ssh 时，`codesign` 会报 `errSecInternalComponent`，直到允许不经图形界面使用私钥：在 Mac 的“终端”里执行一次 `security set-key-partition-list -S apple-tool:,apple: -s ~/Library/Keychains/login.keychain-db`。

在自己编的版本和发布版（Developer ID）之间切换，也要这样 reset 一次（[docs/release.md](docs/release.md) §4）。下载好的模型会保留。

</details>

## 开发

VoxtMini 是基于 [MyGo](https://github.com/egoist/mygo) 的 Go 应用（用 Go 写原生 macOS 界面，不用 cgo）。由 Go 做决定；辅助进程和模型运行时照指令办事，通过 stdin/stdout 上的 JSON 行（辅助进程、`voxtmini-llm`）或回环地址上的 HTTP（Omni 服务）通信。除了 Swift 代码，全部是普通的 Go，在 Linux 上用辅助进程和运行时的假实现测试。在仓库根目录执行：

```sh
CGO_ENABLED=0 go test ./...          # 全部测试，Linux 上可跑
go tool mygo vet .                   # go vet 和 MyGo 的 UI 生命周期检查
VOXTMINI_SNAPSHOTS=/tmp/shots go test -run TestSnapshots .   # 每个页面截图，浅色/深色、英文/简体中文
cd helper && swift test              # Swift 辅助进程的测试（在 Mac 上），用同一套协议 fixture
scripts/build-llm.sh --test          # voxtmini-llm 的 Swift 测试（在 Mac 上）
scripts/mac.sh scripts/mac-e2e.sh    # Mac 上的无界面测试：真实的辅助进程，假的模型
scripts/mac.sh scripts/mac-regress.sh   # Mac 上的真实模型：运行时、评测、打包好的 app
```

存储的测试用 MyGo 的 SQLite 插件，它会下载一次自己的库（按 SHA-256 校验），或者用 `MYGO_SQLITE_LIBRARY`。在 Linux 上，浮窗中文文字的像素检查需要 CJK 字体（Ubuntu：`fonts-noto-cjk`）。

- [docs/architecture.md](docs/architecture.md)：包结构、线程、会话、运行时、磁盘上的数据、测试。
- [docs/plan.md](docs/plan.md)：VoxtMini 是什么，以及里程碑。
- [docs/behavior.md](docs/behavior.md)：VoxtMini 和 Voxt 行为上的不同，以及原因。
- [docs/models-page.md](docs/models-page.md)、[docs/remote-models.md](docs/remote-models.md)、[docs/apple-models.md](docs/apple-models.md)：模型页、远程服务、Apple 的本机模型。
- [docs/llm-models.md](docs/llm-models.md)：语言模型、它们的参数和实测。
- [docs/eval.md](docs/eval.md)：模型在评测集上的质量，以及由此做的决定。
- [docs/helper-protocol.md](docs/helper-protocol.md)、[docs/llm-protocol.md](docs/llm-protocol.md)：辅助进程和 `voxtmini-llm` 的协议。
- [docs/manual-test.md](docs/manual-test.md)：在 Mac 上手动验证的清单，以及自动的真机测试。
- [docs/release.md](docs/release.md)：构建、签名、公证和发布，以及更新。
- [AGENTS.md](AGENTS.md)：改动规则（文字、线程、协议）。
- [CHANGELOG.md](CHANGELOG.md)：各版本的变化。[CONTRIBUTING.md](CONTRIBUTING.md)：怎么提交改动。

VoxtMini 由 [Auralink](https://github.com/hehehai/Auralink) 复制而来。Auralink 是同一作者基于远程模型的应用，它用 Go 和 MyGo 以 [AudioSmith](https://github.com/hehehai/AudioSmith) 的界面重写了 Voxt；VoxtMini 的本地模型运行时和请求策略沿用 Voxt 的做法（见本仓库 `voxt-final` 之前的历史）。

## 许可

Apache License 2.0（[LICENSE](LICENSE)），与本仓其余部分一致。版权归 ActNow 和 SGLang-Omni 团队所有。第三方代码、运行时及其许可见 [NOTICE](NOTICE)；模型按各自的许可下载。
