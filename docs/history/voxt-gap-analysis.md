# Voxt → VoxtMini 功能差距分析

*2026-11-11，项目线程 #3 写（只写文档，不改代码）。对照的版本：Voxt `a28d0c8`（1.15.0，`../voxt`），VoxtMini `7b85172`，Auralink `36c8577`（0.1.0-beta.5）。*

**2026-11-13：用户已经回答了第 5 节的问题，范围收窄（只做转录、翻译、转写；只支持英语、简体中文、日语），见[第 6 节](#6-用户的决定2026-11-13)。下面的清单和评估是之前写的，与第 6 节冲突时以第 6 节为准。**

本文列出 Voxt 有而 VoxtMini 还没有（或只有一部分）的功能，逐项评估价值和成本，最后给出迁移路线。已经做了的、而且和 Voxt 行为不同的，仍以 [behavior.md](../behavior.md) 为准，这里只写一句并指向它。

**另有线程正在做的**（本文只标为 ⏳ 进行中，不评估）：
- **远程模型（ASR + LLM）**：选服务商、URL、Key、模型、高级参数、按功能选择，线程 #1。这一项推翻了 AGENTS.md 里“只用本地模型”的规则，由 #1 改 AGENTS.md 和文档。
- **设置 → “在程序坞中显示图标”**，默认关，线程 #2。

状态记号：✅ 已有　◐ 部分　○ 缺失　✕ 有意不做（[voxt-migration.md](voxt-migration.md) 的决定，或 Voxt 里是死代码 / 开发工具）　⏳ 进行中（#1 / #2）

---

## 1. 方法与来源

**读过的文档**
- Voxt：`AGENTS.md`、`README.md`（项目已迁到 sglang-omni/Voxt）、`docs/Architecture.md`、`docs/Meeting.zh-CN.md`、`docs/index.md` 的目录，以及 `docs/` 下各专题文件的文件名。
- VoxtMini：`AGENTS.md`、`docs/architecture.md`、`docs/plan.md`、`docs/behavior.md`（全文）、`docs/history/voxt-migration.md`。
- Auralink：`AGENTS.md`；和 VoxtMini 逐文件比较了文件列表（Auralink 有、VoxtMini 没有的 83 个文件，见 §2.T）。

**核对过的 Voxt 源码**（`Voxt/` 下，共 509 个 Swift 文件；下面每条都在源码里确认过，没有只看文档）
- 设置的总表：`Settings/Shell/AppPreferenceKey.swift`（全部 UserDefaults 键）、`Settings/Features/FeatureSettings.swift`（各功能的设置结构、功能开关 `FeatureAvailabilitySettings`）、`Settings/Shell/SettingsNavigationTypes.swift`（设置页和各节：General / Model / Feature / Dictionary / App Branch / History / Permissions / Hotkey / About；功能页 Transcription / Translation / Rewrite / App Enhancement / Notes / Meeting / Files）。
- 各设置页：`Settings/GeneralSettingsSections.swift`、`GeneralSettingsView.swift`、`HotkeySettingsView.swift`、`VoiceEndCommandSettingsSection.swift`、`PermissionsSettingsView.swift`、`ReportSettingsView.swift`（首页统计）、`AboutSettingsView.swift`、`Settings/Dictionary/*`、`Settings/Features/*`、`Settings/History/*`、`Settings/Onboarding/*`、`Settings/Shell/SettingsFeedbackDialog.swift`、`SettingsSearchDialog.swift`、`SettingsNotificationDialog.swift`。
- 运行逻辑：`Hotkey/*`（鼠标按键、预设）、`App/Recording/VoiceEndCommandFlow.swift`、`App/PauseEnhancementFlow.swift`、`App/Recording/RecordingTextRouting.swift`、`App/SessionTextIO.swift`、`App/MenuWindowCoordinator.swift`、`Core/ChineseScriptNormalizer.swift`、`Core/FeaturePromptPreset.swift`、`Core/Dictionary/*`、`Core/History/*`、`Core/LLM/TextEnhancer.swift`（Apple Intelligence）、`Transcription/SpeechTranscriber.swift`（Apple 听写）、`Core/Models/RemoteModelConfiguration.swift`（远程服务商列表）、`Core/Utilities/VoxtNotificationStore.swift`、`SystemNotificationSupport.swift`、`Windows/TranscriptionDetailWindow.swift`、`Meeting/*`、`Core/Notes/*`。

**核对 VoxtMini 的方式**：在 Go 源码（不含测试、`third_party`）和 Swift helper 里按关键字 grep 每个功能，读 `internal/settings/settings.go` 的全部字段、各设置页的 `i18n.T` 文案、`history_detail.go`、`home_page.go`、`internal/dict/dict.go`、`internal/lang`、`sglang_omni_mlx/native`（运行时里有哪些服务）和 `scripts/build-omni.sh`（实际打包了哪个服务）。

**两处在 Voxt 里是死代码，不算差距**：
- 停顿时增强（`App/PauseEnhancementFlow.swift`）：唯一的入口 `startPauseLLMIfNeeded()`（`RecordingTextRouting.swift:155`）没有任何调用者。
- 项目词典扫描（`Core/Dictionary/ProjectDictionaryScanner.swift`，扫 `~/.codex`、`~/.claude`）：`scanDefaultContextDirectories` 没有调用者。

---

## 2. 功能清单

“Voxt”一列写行为和源码位置（路径相对 `voxt/Voxt/`）；“VoxtMini”一列写状态和实现位置（路径相对仓库根）。

### A. 录音与快捷键

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| A1 | 多条快捷键、单按 / 双击 / 长按、区分左右修饰键、冲突检查 | `Hotkey/HotkeyManager.swift`、`HotkeyActionResolver.swift`、`Settings/HotkeySettingsValidation.swift` | ✅ `internal/hotkey`、`helper/.../HotkeyRecognizer.swift`、`feature_keys.go`、`settings_shortcuts.go`（behavior 4.2、5.10、5.11） | 无；另有 ActNow 家族的冲突提示（9.13） |
| A2 | **鼠标按键当快捷键**（中键、侧键；预设“鼠标中键 + fn⇧”） | `Hotkey/HotkeySupport.swift` 的 `Input.mouseButton`，`HotkeyManager.swift:184,383,493` 监听 `otherMouseDown/Up` | ○ helper 的 tap 收鼠标事件只用来作废和弦（`helper/Sources/VoxtMiniHelper/Hotkey.swift:68-72,185`，`HotkeyRecognizer.swift:227`），绑定格式只有键盘 | 缺：识别器、`settings.Hotkey` 格式、录制界面、协议 |
| A3 | 快捷键预设（fn 组合 / ⌘ 组合 / 鼠标中键 / 自定义） | `HotkeySupport.swift:143` `Preset`，设置页和引导里选 | ✕ 默认是家族分配的右 ⌥ 一套（`settings.DefaultTranscribe` 等，behavior 4.2a），没有预设下拉 | 有意：fn 留给 Auralink |
| A4 | 转写“双击转录键唤起” | `Settings/HotkeyRewriteActivationState.swift` | ◐ 转写可以单独绑一个“双击”快捷键，效果相同，没有专门的开关 | 可忽略 |
| A5 | “用 Esc 取消”开关（默认开） | `HotkeySettingsView.swift:532`、`App/HotkeyLifecycle.swift:235` | ◐ Esc 总是取消（`internal/hotkey.Route`），不能关 | 小 |
| A6 | **语音结束指令**：说出“over”（或自定义词）后静默一会儿自动停止，并从结果里删掉这个词 | `App/Recording/VoiceEndCommandFlow.swift`（看实时文字的末尾 + 静音时长），`Settings/VoiceEndCommandSettingsSection.swift` | ○ | 缺 |
| A7 | 粘贴最近结果 | `customPasteHotkey*` | ✅ `Manager.Last`（behavior 5.9） | 无 |
| A8 | 麦克风优先列表、自动切换 | `Core/MicrophonePreferenceManager.swift`、`Settings/MicrophonePriority*.swift` | ✅ `settings_audio.go`（behavior 9.1）；◐ 不能拖动排序（上移 / 下移代替） | 菜单栏里的麦克风子菜单见 O4 |
| A9 | 提示音 | `Core/InteractionSoundPlayer.swift` | ✅ 家族提示音库，开始音 / 结束音分开选（behavior 9.2） | 无 |
| A10 | 录音时静音输出设备 | `Core/SystemAudioMuteController.swift`（默认关） | ✅ `internal/outmute`、`mute_link.go`（默认开、崩溃安全，behavior 9.3） | 无 |
| A11 | 听写 VAD 模式 | `localVADMode`、`Core/Transcription/ASRVoiceActivity*.swift` | ✅ Silero + 能量 VAD 兜底，“跳过静音”开关（behavior 3.8、5.22）；另有“声音优化”（5.29） | 无 |

### B. 语音识别（ASR）

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| B1 | 本地 MLX 语音模型目录（Whisper、Parakeet、SenseVoice、Qwen3-ASR、FireRed、Moonshine、Voxtral、Granite、Canary 等，可按标签筛选） | `Transcription/MLXModelManager.swift`、`Settings/Models/LocalASRModelCatalogBuilder.swift` | ◐ 只有 Qwen3-ASR 1.7B / 0.6B（`internal/models/catalog.go`），有意精简（behavior 3.1）。`sglang_omni_mlx/native/src` 里已经有 whisper、parakeet、sensevoice、cohere、moss、nemotron 的服务，但 `scripts/build-omni.sh` 只打包 `qwen3_asr_server`。**2026-11-12（#7）**：Voxt 的 11 个语音模型都进了目录（“模型”页的“更多模型”），Qwen3-ASR 1.7B 6bit 可以下载使用，其余 8 个标“即将支持”（[models-page.md](../models-page.md)） | Qwen3-ASR 不支持的语言没有可选的模型 |
| B2 | **Apple 听写**（“直接听写”：系统的 Speech 框架，不用下载模型，“马上就能用”） | `Transcription/SpeechTranscriber.swift`，`TranscriptionEngine.dictation`（`Settings/TranscriptionTypes.swift`），新设置的默认值 | ○ helper 里没有 `Speech` 框架 | 缺：第一次打开时只能先下 1–2.5 GB 的模型 |
| B3 | 远程 ASR（OpenAI Whisper、豆包、GLM、阿里百炼、阶跃、小米 MiMo、Gemini） | `Transcription/RemoteASR/*`、`Core/RemoteProviders/*`、`RemoteModelConfiguration.swift:235` | ⏳ #1。Auralink 有 Go 实现：`internal/asr/{aliyun,doubao,openai}`、`internal/asr/ws.go`、`endpoint.go`、`internal/providers`、`internal/secrets` | — |
| B4 | ASR 提示：词典作 context bias、语言提示；“ASR 提示”对话框可以改各服务商的提示模板 | `Core/Transcription/ASRHintResolver.swift`、`Settings/ASRHintSettingsSheet.swift`、`Resources/Prompts/*/…-asr-hint.txt` | ✅ 本地的部分照 Voxt 调好（`internal/omni/policy.go`，behavior 3.6）；◐ 只有“识别提示”开关（`settings.Dictionary.ASRHints`），不能改模板 | 本地模型不需要改；远程服务商的模板归 #1 |
| B5 | 录音时显示实时文字，可以关（“显示实时文字”，默认开） | `realtimeTextDisplayEnabled`，`Windows/WaveformView.swift:114` | ◐ 实时文字总是显示（behavior 5.18），没有开关 | 小 |
| B6 | 长录音 | 停止后一次 Final（1200 s 切块） | ✅ 更好：录音期间分段识别（`internal/omni/pieces.go`，behavior 3.7a） | 无 |
| B7 | **繁体中文**：主语言可以选“中文（繁体）”，识别和整理的结果按主语言统一转成繁体（或简体） | `Core/ChineseScriptNormalizer.swift`（`CFStringTransform`），调用在 `App/SessionOutputPreparation.swift:157`、`Recording/RecordingSessionSupport.swift:105-122`；`ASRHintResolver.swift:338` 给 ASR 传 `zh-Hant` | ○ 主语言只有“zh”（`internal/lang`），不区分简繁，没有转换；翻译的目标语言里有繁体（`internal/compose/translate.go:13`） | 缺：台湾、香港的用户拿到的是简体 |
| B8 | 每个功能各选一个语音模型 | `FeatureSettings.*.asrSelectionID` | ✅ `settings.Features.*.ASR` | 无 |

### C. 语言模型与整理

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| C1 | 本地 MLX / GGUF 语言模型 | `Core/Models/CustomLLM*.swift`、`GGUFTranslation*.swift`（在 app 进程里） | ✅ 单独的 `voxtmini-llm` 进程（`llm/`、`internal/llmrt`），3 个模型 + 家族里借用的 2 个（behavior 3.10–3.18、3.25）；2026-11-12（#7）补上 Voxt 目录里除 Qwen3 VL 之外的 MLX 模型（实验性，behavior 3.35） | 无 |
| C2 | **Apple Intelligence**（macOS 26 的系统模型，不用下载） | `Core/LLM/TextEnhancer.swift`（`SystemLanguageModel.default.availability`），`FeatureModelSelectionID.appleIntelligence`；用于转录整理、转写、笔记标题 | ○ | 缺：和 B2 一样，免下载的本地选项 |
| C3 | 远程 LLM（OpenAI、Anthropic、Google、Codex（ChatGPT 账号 OAuth）、Ollama、LM Studio、oMLX、DeepSeek、OpenRouter、Grok、智谱、火山、Kimi、MiniMax、百炼、阶跃、MiMo） | `Core/LLM/*`、`RemoteModelConfiguration.swift:344`、`Core/LLM/CodexOAuthCredentialProvider.swift` | ⏳ #1。Auralink 有：`internal/llm/{openai,anthropic,router,probe,presets,sse,http}.go`、`internal/netx`、`family_keys*.go`（家族共享 Key）。**Auralink 没有 Codex OAuth**，要从 Voxt 移植 | — |
| C4 | 本地模型的生成参数（每个模型一份：温度、top-p、最大 token…） | `Settings/Models/CustomLLMGenerationSettingsSheet.swift`、`customLLMGenerationSettingsByRepo` | ✕ 参数按模型调好放在 `internal/llmrt/models.go`（behavior 3.12）；远程模型的高级参数 ⏳ #1 | 有意 |
| C5 | 提示词预设：转录“精准清理 / 清晰结构”，翻译“精准 / **自然**”，转写“严格”；可以整段改写系统提示词 | `Core/FeaturePromptPreset.swift`，`Resources/Prompts/{en,zh-Hans,ja}/` | ◐ 分层组合（`internal/compose`）：“结构整理”开关（`Transcribe.Structure`）、每个功能的“你的规则”、完整提示词预览（behavior 5.15）；没有“自然翻译”风格，不能整段替换（有意） | “自然翻译”缺 |
| C6 | 停顿时提前增强 | `App/PauseEnhancementFlow.swift` | ✕ Voxt 里没有调用者（§1） | 不算 |
| C7 | LLM 预热 | `App/LLMWarmup.swift` | ✅ 更好：提示词缓存 + 录音期间预热（behavior 3.18） | 无 |
| C8 | 模型调试窗口、LLM 调试提示词、冒烟测试 | `Windows/ModelDebug*.swift`、`llmDebugCustomPrompt`、`App/LLMSmokeTest.swift` | ✕ 开发工具；VoxtMini 用评测（`internal/eval`，docs/eval.md） | 不算 |

### D. 翻译

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| D1 | 语音翻译、选中文本翻译、22 种目标语言、卡片里换语言、“看起来没翻译”重试 | `App/TranslationFlow.swift`、`TranslationSupport.swift` | ✅ 一次请求（behavior 6.1–6.6） | 无 |
| D2 | 翻译专用模型不可用时的后备服务商 | `translationFallbackModelProvider`（`TranslationSupport.swift:469`） | ◐ 本地只有一种选择；有远程模型以后才有意义（⏳ #1） | 交给 #1 |
| D3 | “自然翻译”风格 | 见 C5 | ○ | 见 C5 |

### E. 转写（Rewrite）

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| E1 | 按口述指令改写选区 / 生成、结构化答案、答案卡片、继续追问 | `App/SessionTextIO.swift`、`Core/LLM/RewriteAnswer*.swift`、`Windows/WaveformAnswerCard*.swift` | ✅（behavior 6.7–6.11、5.24） | 无 |
| E2 | 单独的“继续”快捷键 | `RewriteFeatureSettings.continueShortcut` | ◐ 用转写快捷键继续（非激活窗口拿不到空格，behavior 6.11） | 可忽略 |

### F. 应用增强

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| F1 | 按 App / 网址分组、分组提示词、插入后按键、自定义浏览器 | `Settings/AppEnhancement/*`、`Core/AppBranchURLPatternService.swift` | ✅ 更多：最具体的规则优先、按功能生效、叠加而不是替换、匹配预览、App 图标（behavior 7.1–7.8a） | 无 |
| F2 | 浏览器网址用 AppleScript 读（要自动化授权）、CDP 读输入框 | `App/BrowserContextResolver.swift`、`TextInputCDP.swift`、权限页的 “App Branch URL Authorization” | ✕ 只用辅助功能读（behavior 4.6、4.7、1.6） | 有意 |

### G. 词典

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| G1 | 手动词条、替换词（别名）、启用开关、搜索、导入导出 | `Core/Dictionary/DictionaryStore.swift`、`Settings/Dictionary/*`；导入导出是 JSON（`DictionaryTransferManager.swift`：`{version, categories, entries:[{term, replacementTerms, categoryID, groupID…}]}`） | ✅ `internal/dict`、`dictionary_page.go`：CSV 和文本（behavior 7.10）；○ **不认识 Voxt 导出的 JSON** | 缺一条现成的迁移路径（见 S1） |
| G2 | 分类（可折叠，删除时可以把词条移到默认分类） | `DictionaryModels.swift:53` `DictionaryCategory`、`DictionarySettingsView.swift:152` | ○ 只有一个平面列表（`store.Term`） | 小 |
| G3 | 词条限定在某个应用增强分组里生效 | `DictionaryEntry.groupID`，`DictionaryStoreQueries.swift:141-186` 按 `activeGroupID` 选词 | ○ | 中：词典大了才有用 |
| G4 | 命中统计（次数、最近命中），ASR 提示按命中排序 | `DictionaryEntry.matchCount/lastMatchedAt` | ○ 按最近修改排序（behavior 3.6） | 中 |
| G5 | **高置信度自动纠正**：模糊匹配到词条时直接改成词条（默认开） | `DictionaryMatchingSupport.swift:314` `applyCorrections`（`bestFuzzyCandidate`），`dictionaryHighConfidenceCorrectionEnabled` | ○ 只有别名的精确替换，在整理之前（`dict.Replace`，behavior 7.9a） | 中，有误改的风险 |
| G6 | **自动学习**：插入后观察输入框里用户的修改，交给语言模型判断哪些修改该进词典（默认开） | `Core/Dictionary/DictionaryLearningMonitor.swift`、`App/DictionaryLearningObservationFlow.swift`、`DictionaryLearningAnalysisFlow.swift`、`Resources/Prompts/*/…-dictionary-auto-learning.txt` | ○ | 缺；方案 D4 定的“以后” |
| G7 | **一键入库**：用语言模型扫描历史，把术语加进词典 | `Settings/Dictionary/DictionaryOneClickIngestDialog.swift`、`App/DictionaryHistoryScanSupport.swift`、`…-dictionary-ingest.txt` | ○ | 缺 |
| G8 | 扫描 `~/.codex`、`~/.claude` 找术语 | `ProjectDictionaryScanner.swift` | ✕ Voxt 里没有调用者（§1） | 不算 |
| G9 | 历史详情里标出词典改了哪些字 | `Settings/History/HistoryCorrectionPresentation.swift` | ○ | 小 |

### H. 历史

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| H1 | 按功能分 tab、搜索、详情、删除、清理、录音存储与导出 | `Core/History/*`、`Settings/History/*` | ✅ 更多：全文搜索、失败记录、重试 / 重新识别、失败录音保留 7 天、搬家时移动录音（behavior 5.20–5.23a、9.15、9.16） | 无 |
| H2 | **在 app 里播放录音** | `Windows/HistoryAudioPlaybackSupport.swift`、`TranscriptionDetailWindow.swift` | ○ 只有“在访达中显示录音”（`history_detail.go:52`、`history_menu.go:58`） | 缺 |
| H3 | 对一条历史继续提问（详情窗口里的对话，结果存回记录） | `TranscriptionDetailWindow.swift`（`followUpAnswerer`、`followUpPersistence`），`Resources/Prompts/shared/saved-transcription-follow-up.txt` | ○ | 小 |
| H4 | 笔记、会议记录这两类历史 | `HistoryFilterTab.note/.transcript` | ✕ 随 I、J | — |

### I. 会议（Meeting）· J. 笔记（Notes）· K. 文件转写（Files）

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| I1 | 会议记录：麦克风 + 系统声音（Core Audio process tap）、实时转写、说话人区分（Sortformer）、实时翻译、摘要和追问、会议浮窗（可对屏幕共享隐藏）、详情窗口（波形、虚拟列表、导出） | `Meeting/*`（59 个文件）、`Windows/MeetingDetail/*`（18 个）、`App/Meeting*Flow.swift`、`docs/Meeting.zh-CN.md`（Beta） | ✕ voxt-migration.md 决定不做 | 运行时里已经有 `sortformer`（`native/ci/models.json` 钉了 `diar_streaming_sortformer_4spk-v2.1`） |
| J1 | 笔记：转录时按笔记快捷键记下要点，短 AI 标题，屏幕角落唤出的悬浮面板，同步到 Obsidian（按会话 / 按天 / 单文件）和“提醒事项” | `App/NoteFlow.swift`、`NoteTitleSupport.swift`、`Windows/VoxtNoteWindow*.swift`、`Core/Notes/*`、`Core/Security/RemindersPermissionManager.swift` | ✕ | — |
| K1 | 文件转写：拖入音频 / 视频，排队转写、说话人分析、进度和剩余时间，系统通知完成 | `Meeting/Processing/MeetingImported*.swift`、`MeetingFileTask*.swift`、`Settings/Features/FeatureFileTaskSections.swift` | ✕；但“重新识别”已经能对一段保存的录音跑整段 Final（`history_retry.go`、`session.Manager.Retry`），离“转写一个文件”只差导入和解码 | 见 §3 K1 |

### L. 同步与账号

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| L1 | 账号 / 云同步 | Voxt 没有自己的账号和云同步；只有 Codex OAuth（远程 LLM 的登录，C3）和笔记同步到 Obsidian / 提醒事项（J1） | ✕ 无；VoxtMini 有 ActNow 家族的本机共享（模型、语言、代理、快捷键、内存、粘贴，behavior 3.19–3.29） | 不算 |

### M. 设置 / 通用

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| M1 | 界面语言：英文、简体中文、**日文** | `en.lproj`、`zh-Hans.lproj`、`ja.lproj`（2371 行）；提示词也有 `Resources/Prompts/ja` | ◐ 英文、简体中文（`internal/i18n/locales`，1079 条）；整理的提示词模板也只有 `en`、`zh-Hans`（`internal/compose/templates`） | 缺日文 |
| M2 | 开机启动 | `launchAtLogin` | ✅（behavior 9.7） | 无 |
| M3 | 在程序坞中显示 | `showInDock`、`Core/AppBehaviorController.swift` | ⏳ #2 | — |
| M4 | 代理：跟随系统 / 关 / 自定义（HTTP、HTTPS、SOCKS5） | `GeneralSettingsSections.swift:561-594`、`Core/VoxtNetworkSession.swift` | ◐ 只管模型下载：本地模型页的“网络…”，跟随家族（behavior 3.26）；远程请求的代理 ⏳ #1（Auralink `settings_network.go`、`internal/netx`） | 交给 #1 |
| M5 | **“结果同时复制到剪贴板”**（每次完成都把结果留在剪贴板） | `autoCopyWhenNoFocusedInput`，`App/TextOutputDelivery.swift:69`、`SessionTextIO.swift:371-415` | ○ 粘贴后总是恢复原来的剪贴板（只有粘贴没生效时才留下，behavior 5.8a） | 缺 |
| M6 | 调试日志开关（快捷键、模型） | `hotkeyDebugLoggingEnabled`、`llmDebugLoggingEnabled` | ◐ “记录说了什么”（`Privacy.LogText`）、日志页和诊断包 | 可忽略 |
| M7 | 功能开关（关掉翻译、转写、笔记、应用增强、会议、文件，并隐藏它们的页面） | `Settings/Features/FeatureAvailabilitySections.swift`，`FeatureAvailabilitySettings` | ○ 三个功能总是有；不想用可以删掉快捷键 | 小 |
| M8 | 模型存放位置可选 | `modelStorageRootPath` + 安全书签 | ◐ 跟随家族的 `models.root`（`internal/actnow/actnow.go:138`），VoxtMini 里只显示（behavior 3.27） | 有意：家族共享 |
| M9 | 设置搜索 | `Settings/Shell/SettingsSearchDialog.swift` | ✅ ⌘K | 无 |
| M10 | 反馈 | `SettingsFeedbackDialog.swift`（打开 GitHub Issues） | ✅ “报告问题”+ 诊断包（`settings_about.go`） | 无 |
| M11 | 关于：作者、微信二维码、赞助、致谢 | `AboutSettingsView.swift` | ✕ 只有版本、源码、许可（`settings_about.go`） | 产品决定 |

### N. 权限

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| N1 | 麦克风、辅助功能 | `PermissionsSettingsView.swift`、`Core/Security/AccessibilityPermissionManager.swift` | ✅ 设置 → 权限（behavior 2.5） | 无 |
| N2 | 浏览器自动化、提醒事项、会议的系统声音 | 同上 | ✕ 随 F2、J1、I1 | — |

### O. 浮窗与界面

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| O1 | 录音浮窗、答案卡片、样式设置（位置、不透明度、圆角、边距） | `Windows/RecordingOverlay*.swift`、`WaveformView.swift`、`WaveformAnswerCard*.swift` | ✅ 按 Voxt 重做（behavior 5.18–5.28、9.4） | 无 |
| O2 | 首页统计：总时长、总字数、平均速度、近 7 天每天字数、翻译字数、自动学到的词 | `Settings/ReportSettingsView.swift` | ◐ 今天的次数、字数、按功能的次数、最近 5 条（`home_page.go`，behavior 8.1） | 缺累计、趋势、速度 |
| O3 | 浮窗对屏幕共享隐藏 | `hideMeetingOverlayFromScreenSharing`（只用于会议浮窗） | ✕ 随 I1 | — |
| O4 | 菜单栏：Dashboard、各设置页、按功能的历史子菜单、最近记录、**麦克风子菜单**、检查更新、反馈 | `App/MenuWindowCoordinator.swift:80-236` | ◐ 开始 / 停止、粘贴最近、最近 3 条、打开、设置、检查更新、退出（`tray.go`，behavior 9.8）；没有麦克风子菜单（behavior 9.1 有意） | 麦克风子菜单小 |

### P. 通知与更新

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| P1 | 自动更新、Beta 通道 | Sparkle（`Core/Utilities/AppUpdateManager.swift`） | ✅ MyGo updater + actnow.dev（behavior 9.10、9.11） | 无 |
| P2 | **服务端公告**（设置 → 通知，未读红点） | `Core/Utilities/VoxtNotificationStore.swift`（`https://voxt.actnow.dev/api/app/notifications`）、`SettingsNotificationDialog.swift` | ✅ `internal/announce`：`https://actnow.dev/api/voxtmini/notifications`（actnow-www），新公告成为通知卡片，每条一次（behavior 9.18） | 无（没有封面图和单独的通知页） |
| P3 | 系统通知中心（模型下载完成 / 失败、文件任务完成） | `Core/Utilities/SystemNotificationSupport.swift`，`MLXModelManager.swift:521,550` | ○ | 小 |

### Q. 引导 · R. 多语言 · S. 迁移

| # | 功能 | Voxt | VoxtMini | 差距 |
|---|---|---|---|---|
| Q1 | 设置引导 | `Settings/Onboarding/*`（6 步） | ✅ 7 步，复用家族里已装的模型（behavior 8.2–8.11） | 无 |
| R1 | 日文界面和提示词 | 见 M1 | ○ | 见 M1 |
| S1 | **从 Voxt 导入数据**（词典、应用增强分组、历史） | Voxt 是沙盒 app，数据在 `~/Library/Containers/com.voxt.Voxt`（GRDB `VoxtDatabase`，分组在 UserDefaults）；词典能导出 JSON | ○ voxt-migration.md“Open”一节，未决定 | 缺 |

### T. Auralink 有、VoxtMini 没有的（不来自 Voxt，顺带列出）

逐文件比较的结果（除远程相关的部分外）：

| # | 功能 | Auralink | 说明 |
|---|---|---|---|
| T1 | 诊断日志行：启动时发现了什么（版本、数据文件夹、数据库、设置、菜单栏图标）、会话为什么没开始、快捷键按了却没触发 | `diagnostics.go`、`session_diag.go`、`internal/diag`、`internal/datadir/status.go`、`docs/diagnostics.md` | 排查用户问题很有用，几乎可以照搬 |
| T2 | 重启后日志页能看到上一次运行的日志 | `logs_history.go` | 小，照搬 |
| T3 | 卡片“插入”、粘贴最近结果也写一行交付日志 | `session_deliver_log.go`、`internal/deliver/describe.go` | 小，照搬 |
| T4 | app 内的更新面板和更新日志历史 | `updates_view.go` | VoxtMini 用插件的原生窗口（behavior 9.10），不需要 |
| T5 | 演示构建（README 截图用的种子数据） | `demo*.go`、`internal/demo` | 发布官网素材时有用，低 |
| T6 | 家族共享 API Key、网络设置页 | `family_keys*.go`、`settings_network.go`、`internal/netx` | ⏳ 归 #1 |

---

## 3. 缺失 / 部分项的评估

价值：高 / 中 / 低。工作量：S（≤1 天，只动 Go 或只动一处）、M（2–5 天，跨 Go + helper / llm 进程，或要改协议、迁移）、L（1 周以上，或新子系统）。来源：“Voxt”指移植 Swift 代码（AGENTS.md 允许，Apache-2.0），“Auralink”指移植 Go 代码。

### 3.1 值得做

| # | 项 | 价值 | 工作量 | 依赖 / 风险 | 来源 | 建议 |
|---|---|---|---|---|---|---|
| S1 | 从 Voxt 导入（第一步：认 Voxt 导出的词典 JSON） | 高：老用户迁过来，最痛的是重新录一遍词典 | **S**：`dict.Import` 加一种格式，`replacementTerms` → 别名，分类 / 分组名写进备注；不用碰 Voxt 的沙盒 | 无权限问题（用户自己在 Voxt 里导出）。第二步（直接读容器里的分组和历史）是 M，要用户同意访问另一个 app 的数据，而且 GRDB 的表结构会变 | Voxt `DictionaryTransferManager.swift` | **现在做**第一步；第二步等用户决定（§5 Q1） |
| B7 | 繁体中文主语言 + 输出简繁统一 | 高（对台湾、香港用户），其他人为零 | **M**：主语言要能区分 `zh-Hans` / `zh-Hant`（`internal/lang` 现在只有 `zh`，设置、组合器、ASR 语言提示都要改）；转换不能用 cgo，要么在 helper 里加一个 `text.transform`（`CFStringTransform`，改协议），要么用纯 Go 的 OpenCC 词表（Apache-2.0，进 NOTICE） | 语言模型在繁体输入下的整理质量要用评测测一次；Qwen3-ASR 繁体输出偏少，主要靠转换 | Voxt `ChineseScriptNormalizer.swift`（逻辑很简单） | **现在做**，如果目标用户里有繁体用户（§5 Q2） |
| P2 | 服务端公告 | 中：要通知 Voxt 用户迁移的那套机制，VoxtMini 自己也会需要（停用旧版本、重大问题） | **S–M**：后台拉一个 JSON，按版本过滤，作为 `internal/notify` 的卡片显示；缓存和“已读”放 documents | 要在 actnow.dev 上有接口（`actnow-www` 仓库）；只拉，不上报任何东西 | Voxt `VoxtNotificationStore.swift`（字段格式）；卡片用现有的 `shell_notices.go` | **以后**，等 actnow.dev 有接口时 |
| T1–T3 | 诊断日志行、上次运行的日志、交付日志 | 中：排查用户反馈靠它们 | **S**：Auralink 的代码几乎原样能用（同一个外壳） | `internal/diag` 不依赖别的包；注意 archtest 的 `imports.txt` | Auralink | **现在做** |
| M5 | 结果同时复制到剪贴板 | 中：不少用户习惯插入后再 ⌘V 一次，或在别处再粘一次 | **S**：Go 给 `textio.paste` 一个“不恢复剪贴板”的参数（helper 已经有“粘贴没生效时不恢复”的路径），设置加一个开关 | 和家族的 `pasting` 规则不冲突；要改 helper 协议的参数（AGENTS.md：所有地方一起改） | Voxt `TextOutputDelivery.swift:69` | **现在做** |
| A6 | 语音结束指令 | 中：适合长时间口述、不方便按键的场合 | **S–M**：纯 Go 逻辑（实时文字的末尾 + Silero / 电平的静音时长 → `session` 自己停止，Final 后删掉指令），设置里一个开关 + 预设 / 自定义词 | 只有流式会话有实时文字；Final 和预览的文字可能不同，删除要对 Final 做；误停的风险要测 | Voxt `VoiceEndCommandFlow.swift`（匹配和删除规则） | **以后**（第二批） |
| H2 | 历史里播放录音 | 中：确认“到底说了什么”时最直接 | **S–M**：一个播放 / 停止按钮和进度；播放可以用 actnowkit 的 `sounds.Player`（提示音已经在用，`settings_sounds.go`，AVAudioEngine）或 helper 的 AVAudioPlayer；不需要波形 | build pass 不能做 I/O：播放放在 goroutine 里 | Voxt `HistoryAudioPlaybackSupport.swift`（行为参考） | **以后**（第二批） |
| B2 | Apple 听写（免下载的语音识别） | 中–高：第一次打开就能用，模型下好之前不用等；内存也最小 | **M**：helper 里加 `Speech`（`SFSpeechRecognizer` 的 on-device 模式，或 macOS 26 的 `SpeechAnalyzer`），作为 `local.Services` 的一个 ASR；要新权限“语音识别”（Info.plist、权限页、引导） | 质量明显低于 Qwen3-ASR，词典只能作 contextualStrings；违背“只放最好的几个模型”的思路，所以只适合做“下载期间的临时方案” | Voxt `SpeechTranscriber.swift` | **以后**，看用户是否在意首次体验（§5 Q3） |
| C2 | Apple Intelligence（免下载的语言模型） | 中：在支持的 Mac 上不用下 2–3 GB 的模型 | **M**：`voxtmini-llm` 本来就是 Swift 进程，加一个 `FoundationModels` 后端（macOS 26，Apple 芯片、开启了 Apple Intelligence）；llm 协议里加模型 id，评测一遍 | 系统模型的中文整理质量、指令遵从要实测；不能控制采样参数，提示词缓存用不上 | Voxt `TextEnhancer.swift` | **以后**，先做一次评测再决定 |
| G5 | 词典的高置信度模糊纠正 | 中 | **M**：纯 Go（拼音 / 编辑距离的候选、置信度门槛），放在 `dict.Replace` 旁边 | 误改比不改更糟；要先在评测集里量误改率 | Voxt `DictionaryMatchingSupport.swift`（候选和门槛） | **以后** |
| G7 | 用语言模型从历史里提取术语（一键入库） | 中：新用户冷启动词典 | **M**：后台任务：分批把历史文本交给本地模型，得到候选，用户勾选后入库 | 本地小模型抽术语的质量要测；长历史要分批、能取消 | Voxt `DictionaryOneClickIngestDialog.swift`、`DictionaryHistoryScanSupport.swift`、`…-dictionary-ingest.txt` | **以后** |
| G4 | 词条命中统计 | 低–中：词典大了以后，ASR 提示（`asr.fast` 只取 30 条）应该优先放常用的 | **S–M**：store 迁移加 `match_count`、`last_matched_at`；会话结束时更新；`dict.Hints` 换排序 | 迁移；排序变化会影响提示词缓存命中（要量） | Voxt `DictionaryEntry` | **以后**，和 G5 一起 |
| O2 | 首页累计统计和 7 天趋势 | 低–中 | **S**：在 goroutine 里从历史汇总（`readHome` 已有）；趋势用简单的条形 | 只有保存历史时才有数据 | Voxt `ReportSettingsView.swift`（指标定义） | **以后** |
| K1 | 转写一个音频 / 视频文件（不含说话人区分） | 中：会议录音、语音备忘录 | **M**：导入（拖放 / 选择）、解码成 16 kHz PCM（视频要 helper 用 AVFoundation 解码，改协议）、复用重试路径的整段 Final 和分段识别、结果进历史 | 长文件的内存（已有 60 s 切块，behavior 3.6）；和会话共用模型时的排队 | 复用 VoxtMini 的 `history_retry.go`；Voxt 的文件任务队列太重，不搬 | **以后**（§5 Q4） |
| M1 | 日文界面和提示词 | 取决于用户群 | **M**：1079 条文案翻译 + 审校；`TestKeyParity` 要扩展到第二种语言；`compose/templates/ja` 和对应的 golden；评测里加日文样本 | 提示词质量要评测；之后每加一条文案要三种语言 | Voxt `ja.lproj`、`Resources/Prompts/ja`（可直接参考措辞） | **以后**，有日文用户需求再做（§5 Q5） |
| C5 / D3 | “自然翻译”风格 | 低–中 | **S**：翻译页一个“风格”选项，compose 加一层 | 要评测两种风格的差别；也可以靠“你的规则”写 | Voxt `en-translation-natural.txt` 等 | **以后** |
| A2 | 鼠标按键当快捷键 | 低–中：用多键鼠标的人喜欢 | **M**：helper 识别器把 `otherMouseDown/Up` 当作一个“键”（单按 / 长按），`settings.Hotkey` 加鼠标格式，录制界面，家族快捷键清单的表示 | 吞掉鼠标键会影响别的软件；家族规范要能描述鼠标键 | Voxt `HotkeyManager.swift`（事件处理） | **以后** |
| G3 / G2 | 词条限定在分组 / 分类 | 低 | M / S | store 迁移，词典页变复杂 | Voxt `DictionaryModels.swift` | **暂不做**，等用户反馈词典太大 |
| G6 | 自动学习（观察插入后的修改） | 中，但风险高 | **L**：helper 要在插入后持续读焦点元素的文字（新协议），Go 做差异和判定，本地模型判断，再入库；还要处理隐私（读用户正在写的整段内容）、Electron / 网页的读取不稳定 | 隐私和误学是主要风险；Voxt 的实现量大（约 6 个文件 + 提示词） | Voxt `DictionaryLearning*.swift` | **暂不做**；先做 G7（同样的价值、风险小得多） |
| H3 | 对历史记录继续提问 | 低 | M | 需要一个对话界面 | Voxt `TranscriptionDetailWindow.swift` | **暂不做** |
| A5 / B5 / M7 / O4 / P3 / G9 | Esc 开关、实时文字开关、功能开关、菜单栏麦克风、系统通知、历史里标出词典纠正 | 低 | 各 S | — | Voxt | **按需**，有用户提再做 |
| B1 | 更多语音模型（如给 Qwen3-ASR 不支持的语言加 Whisper） | 低–中 | **M**：运行时里已有 `whisper` 服务，要打包（`build-omni.sh`）、钉版本、写请求策略、评测 | 和“只放最好的几个”冲突；磁盘 | sglang-omni native + Voxt 的模型策略 | **暂不做**，除非有语言覆盖的需求 |

### 3.2 不建议做

| # | 项 | 理由 |
|---|---|---|
| I1 | 会议记录 | voxt-migration.md 已决定不做。它在 Voxt 里是 Beta，约 80 个文件，要系统声音的权限和 Core Audio process tap，和“轻量语音输入”的定位不符。如果以后要做，应该是一个单独的线程甚至单独的 app；运行时里的 Sortformer 让说话人区分不再是障碍 |
| J1 | 笔记（含 Obsidian / 提醒事项同步） | 同上；而且要提醒事项的权限、Obsidian 的文件夹授权 |
| F2 | AppleScript 读网址、CDP | 已用辅助功能取代（用户 10-15 决定） |
| C4 | 本地模型的生成参数 | 参数是按模型调好、评测过的；暴露出来只会让用户调坏。远程模型的参数归 #1 |
| M11 | 作者二维码、赞助 | 产品决定，不是功能差距 |
| C6、C8、G8 | 停顿增强、调试窗口、项目词典扫描 | Voxt 里是死代码或开发工具 |
| T4、T5 | Auralink 的更新面板、演示构建 | 前者 VoxtMini 用了插件原生窗口；后者只在做官网素材时才需要 |

---

## 4. 迁移路线（建议）

前提：#1（远程模型）和 #2（程序坞）各自进行，下面的批次不依赖它们；同时动 `textio.paste` 参数或 helper 协议的工作要和 #1 协调（#1 大概率不碰 helper）。

**第一批：小而确定，1 周左右**
1. S1 第一步：导入 Voxt 导出的词典 JSON（S）。
2. T1–T3：从 Auralink 搬诊断日志行、上次运行的日志、交付日志（S）。
3. M5：“结果同时复制到剪贴板”（S，改 helper 协议的一个参数）。
4. B7：繁体中文主语言和简繁转换（M）——如果 §5 Q2 的答案是“要”。

**第二批：日常体验，1–2 周**
5. H2：历史里播放录音（S–M）。
6. A6：语音结束指令（S–M）。
7. O2：首页累计统计和 7 天趋势（S）。
8. P2：服务端公告（S–M，等 actnow.dev 的接口）。
9. C5 / D3：“自然翻译”风格（S，评测决定要不要）。

**第三批：词典变聪明，2 周左右**
10. G4：命中统计（S–M，含迁移）。
11. G7：用本地模型从历史里提取术语（M）。
12. G5：高置信度模糊纠正（M，先量误改率）。

**第四批：先评测、再决定**
13. C2：Apple Intelligence 后端——先在 mac-air 上对评测集跑一次，比 `llm.fast` 好或接近才做。
14. B2：Apple 听写——只作为“模型下载完成前”的临时识别。
15. K1：转写一个音频 / 视频文件。
16. M1：日文界面和提示词。
17. A2：鼠标按键快捷键。

**不排期**：G6 自动学习、G2 / G3 词典分类与分组、H3 历史追问、B1 更多语音模型、I1 会议、J1 笔记，以及 §3.2 的其他各项。

---

## 5. 需要用户决定的问题

1. **Voxt 数据导入做到哪一步？** (a) 只认 Voxt 导出的词典 JSON（建议，S，不碰权限）；(b) 再直接读 Voxt 容器里的应用增强分组和历史（M，macOS 会弹“访问其他 app 的数据”，Voxt 升级后表结构可能变）。
2. **要不要支持繁体中文用户？**（B7）要的话放进第一批。
3. **首次体验要不要“免下载”？** Apple 听写（B2）和 Apple Intelligence（C2）都在本机、不用下载，但质量不如我们挑的模型。可以 (a) 不做；(b) 只在模型下载完成之前临时用；(c) 作为正式选项。建议先评测，再在 (a)、(b) 之间选。
4. **文件转写（K1）算不算“语音输入”的范围？** 做的话只做“转写一个文件进历史”，不做说话人区分和会议。
5. **日文（M1）有没有目标用户？** 没有就不做；有的话，之后每条新文案都要三种语言。
6. **服务端公告（P2）**：actnow.dev 是否会提供各 app 的公告接口（`actnow-www`）？VoxtMini 是否也用它？
7. **会议和笔记**是否仍维持 voxt-migration.md 的“不做”？（本文按“不做”写。）

## 6. 用户的决定（2026-11-13）

第 5 节的问题，用户 2026-11-13 的回答（项目规则 RULES.md 里也记了）：

| # | 问题 | 决定 | 落实 |
|---|---|---|---|
| 1 | Voxt 数据导入 | **不做**导入，也不做历史分析 / 术语提取 | — |
| 2 | 繁体中文（B7） | **不支持繁体中文** | 线程 #11：语言只剩三种，设置里的 `zh-Hant` 启动时去掉（behavior.md 9.13） |
| — | 支持哪些语言 | **只支持英语、简体中文、日语**；不支持这三种的模型不要 | 线程 #11：`internal/lang` 只剩三种；主要语言、其他语言、翻译目标、远程语音的语言提示都只在三种里选；目录去掉 GLM-4 9B 0414（模型卡没有日语）和 5 个“即将支持”的语音模型（models-page.md “Languages”） |
| 3 | Apple 听写（B2）和 Apple Intelligence（C2） | **要做**，作为本地模型 | 另开线程 |
| 4 | 文件转写（K1） | **不做** | — |
| 5 | 日文界面（M1） | 还在和用户确认（应用增强是否保留也在确认） | — |
| 6 | 服务端公告（P2） | **做**：actnow.dev 下的 `/voxtmini` 接口 + VoxtMini 客户端 | 另开线程 |
| 7 | 会议和笔记 | **不做** | — |
| — | 功能范围 | 只有**转录、翻译、转写**三个功能 | — |
