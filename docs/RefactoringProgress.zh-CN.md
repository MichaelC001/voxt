# 分阶段重构实施记录

关联：[全项目评估](RefactoringAssessment.zh-CN.md)、[源码地图](Architecture.md)、[回归矩阵](LocalRegressionMatrix.md)。

状态定义：**已实施**表示代码已修改并完成可用的静态检查；**已验收**必须包含 macOS 编译、XCTest 和相关人工检查。实施环境为 Linux，没有 Swift/Xcode；下面各批均不能仅凭静态检查标记为已验收。提交及 PR 状态以 Git 历史和 GitHub 为准；合入前仍需完成下述验收。

## 阶段总览

| 阶段 | 范围 | 实施状态 | 验收状态 |
| --- | --- | --- | --- |
| 0 | 远程 LLM 拆分、旧收尾流水线删除、首轮文档修正 | 已实施，见评估报告 | CI 测试工作流通过；Release / 人工待验收 |
| 1 | MLX 纯逻辑/独立推理边界、Onboarding、Settings 组件 | 已实施 | CI 通过；有状态驱动仍待阶段 5 |
| 2 | 核实的孤儿 UI、下载动作、会议空包装 | 已实施 | CI 通过；人工待验收 |
| 3 | 大测试套件、目录归位、回归入口维护 | 已实施 | CI 通过；人工待验收 |
| 4 | Remote ASR / 会议 provider 会话契约与去重 | 已实施，新增 36 个定向测试 | `fd38d43` macOS CI 通过；真实 provider 人工待验收 |
| 5A | 请求/启动任务、会议 session-token 与清理屏障、MLX 校正任务所有权 | 已实施，新增 20 个测试 | `b771be1` macOS CI 通过；设备/模型验收待执行 |
| 5B | 热键监听安装对象与业务状态、MLX native-live 任务/use 释放 | 已实施，新增 15 个测试 | `5936e62` macOS CI 通过；设备/模型验收待执行 |
| 5C | 录音身份/提交、模型加载退出、会议导入资源与最终化快照 | 已实施，新增 22 个测试 | 等待本批 macOS CI；整体集成验收仍待执行 |
| 6 | Dictionary/History/MeetingDetail 等剩余大文件与最终验收 | 待实施 | 分域推进；完整构建、测试和人工回归 |

阶段 0–3 的 `fa087ed` 已通过 [macOS CI Tests 工作流](https://github.com/hehehai/voxt/actions/runs/35433366619)。这是前一批的证据，不能替代阶段 4 新增行为的编译和测试，也不代表 Release 构建、模型回放及真实设备验收已完成。

阶段 4 的 `fd38d43` 也已通过 [macOS CI Tests 工作流](https://github.com/hehehai/voxt/actions/runs/35436703019)，日志包含 `TEST SUCCEEDED`。阶段 5A 的 `b771be1` 已通过 [macOS CI](https://github.com/hehehai/voxt/actions/runs/35438916477)，阶段 5B 的 `5936e62` 已通过 [macOS CI](https://github.com/hehehai/voxt/actions/runs/35440624827)。阶段 5C 仍需单独验证。

阶段 3 中不涉及行为的文件归位提前实施；这不表示存储及同步生命周期重构已完成。不得因为暂时没有 Mac 就把阶段 4–6 的风险或验收项删掉。

## 阶段 1：从大文件中分离职责

### MLX

`Voxt/Transcription/MLXTranscriber.swift` 从 3,855 行降至 2,274 行，新增职责文件：

| 文件 | 职责 | 行数 |
| --- | --- | ---: |
| `MLXTranscriptionModels.swift` | 规划、结果、回放使用的值类型 | 112 |
| `MLXTranscriptionPlanning.swift` | 采样选择、VAD 分段、校正节奏、Final 预算 | 318 |
| `MLXTranscriptMerging.swift` | 顺序合并、稳定前缀和隐藏预览合并 | 218 |
| `MLXLiveTextPreview.swift` | 原生语言选择、Qwen 协议头和可见文本 | 231 |
| `MLXCaptureBuffers.swift` | 原有加锁采样缓冲、VAD pre-roll、合并电平投递 | 250 |
| `MLXDetachedInference.swift` | 已解析配置及独立推理；保留 nonisolated 边界 | 377 |
| `MLXStructuredTranscript.swift` | live-ended / batch 共用的片段可靠性规则 | 77 |

- 两份完全相同的 `mergeStablePrefix` / `longestCommonPrefix` 算法合并复用；迁移前先比对原函数体。
- 任务创建、取消传播、会话 revision、模型 pin/unpin 和采样实例所有权仍由原 transcriber 持有；没有为了行数把运行时状态全面开放给扩展。
- **剩余 2,274 行仍是热点**。分离 native-live、capture 和 finalization 的状态所有权属于阶段 5，不能把此次提取描述成已完成生命周期重构。

### Onboarding

`OnboardingGuideView.swift` 从 2,868 行降至 440 行。

- 主视图保留 SwiftUI 状态及生命周期装配。
- `OnboardingGuidePermissions`、`Practice`、`ModelSelection`、`Configuration`、`Modals`、`Shortcuts` 按步骤/职责整理。
- `OnboardingGuideModelRows`、`Components`、`Styles` 与 `SelectableGuideTextView` 分离展示组件和 AppKit 桥接。
- 跨文件实际使用的成员改为 internal，文件内 helper 继续 private；没有更改练习的 session ID、焦点、确认保存、权限轮询或麦克风 watchdog 行为。
- 步骤扩展仍共享主视图状态，这是结构整理，**不是宣称已完成状态解耦**。后续提取子视图时要保持 SwiftUI state identity。

### Settings 外壳

`SettingsView.swift` 从 2,047 行降至 844 行。

- 分离 `SettingsSidebar`、`SettingsSidebarHeader`、`SettingsSidebarFooter`。
- 分离反馈弹窗和通知弹窗；反馈地址通过参数传递，不扩大外壳私有常量的访问范围。
- 导航及观察状态保留 private；通知列表自己的状态仍归通知视图所有。
- 外壳仍略高于 800 行复审线，后续继续按导航/页面装配边界整理，而不是再机械切开所有状态。

## 阶段 2：删除依据

| 删除 / 简化项 | 核实依据 | 保留覆盖 |
| --- | --- | --- |
| `GuideBullet`、旧 onboarding `hotkeyBinding(for:)` | 无调用；当前快捷键配置使用保留 trigger behavior 的 `shortcutBinding` | `OnboardingGuideTests` 及人工引导验收 |
| `GeneralModelStorageCard` | 全仓仅声明，当前设置/引导有实际使用的路径选择组件 | 设置与模型存储回归 |
| 4 个 `ModelDownloadPresentationSupport` 动作工厂及其 localized wrapper | 无调用；保留实际使用的 statusText 和 DownloadState | `ModelDownloadStatusSnapshotTests` |
| `RemoteASRMeetingConfiguration` | `hasValidMeetingModel` 与已有 `isConfigured` guard 完全相同；resolved 配置原样返回；其余配置/status helper 没有调用 | `MeetingStartPlannerTests`、`MeetingASRSupportTests` |
| 不可达 `.remoteASRMeetingUnavailable` 分支和仅供该分支使用的 provider 参数 | 前一个 guard 已排除相同条件；provider 身份仍在 RemoteProviderConfiguration 中 | 原 8 个 planner 测试保留，仅移除 9 处无用实参 |
| `transcribeMeetingChunk`、2 个未使用 typealias | 无调用；实际结构化 chunk 接口保留 | 会议转录测试 |

仍在用的 meeting aliases 保留；adapter 移至 `MeetingTranscriberAdapters.swift`。不改持久化枚举值、数据库迁移、本地化 key 或协议/selector 回调。

## 阶段 3：测试和目录

### 测试

四个大套件共 **230 个测试方法**按行为分组；迁移前后逐个核对方法体和断言，未删除测试：

| 原套件 | 原行数 | 归类 |
| --- | ---: | --- |
| HotkeyManagerTests | 2,588 | 通用派发、Note、修饰键、恢复、双击、粘贴、长按、鼠标、会话停止 |
| RemoteModelConfigurationTests | 1,752 | 通用配置、ASR、凭据读取/写入/迁移、Codex、端点迁移 |
| MLXModelManagerTests | 1,214 | 目录策略、生命周期、安装/存储；Custom LLM 配置单独成组 |
| MeetingDetailViewModelTests | 1,108 | 摘要、live 更新、文本/说话人编辑、翻译 |

共享 fixture 放在 `VoxtTests/TestSupport/*TestCase.swift`，不在基类声明测试方法；保留 MainActor、默认配置恢复、临时目录清理、受控 continuation 和原有 manager 保留策略。迁移不是修改测试的等待/调度策略。

阶段 3 结束时应用 425 个 Swift 文件、152,512 行，26 个文件仍 >1,000 行；测试 186 个 Swift 文件、39,560 行，最大文件 874 行，静态 `func test…` 数仍为 1,585。行数包含注释和空行；测试文本保留不等于 XCTest 已成功发现/运行。

### 目录

原样移动，不修改路径解析、数据格式或存储位置：

- `HistoryRepository`、`HistoryValueResolver` → `Core/History/`。
- `DictionaryRepository` → `Core/Dictionary/`。
- Note store、Obsidian/Reminders export store 和 sync coordinator → `Core/Notes/`。

Xcode 使用同步目录组，无需手工添加 Swift build phase 条目；仍需 Mac 验证 target membership。`Info.plist` 和资源目录未移动，音频夹具未修改。

### 回归脚本修复

`tools/run_local_regression_matrix.sh`：

- 删除开发者机器的绝对路径，改为从脚本位置解析仓库根目录。
- 增加 `refactor` 分组，包含 core 和拆分后的测试家族；旧 suite 名不再代表原整套覆盖。
- 删除指向不存在测试类的 `whisper` / `diagnostic` 分组；未知/已删除组返回非零，不产生“零测试通过”的假象。**不是删除 MLX Whisper 支持或迁移测试。**
- `all` / `full` 不再重复运行已包含在 core 的三个 VAD suite。
- 普通和 build-for-testing 路径均显式关闭签名并严格使用锁文件。
- 新增 5 项 Python CLI 测试，使用假的 xcodebuild 检查 suite 存在、覆盖、路径、参数、去重及失败传播；不冒充 Swift 测试。

## 阶段 4：远程 ASR 协议与完成契约

### 职责拆分与共享

- `RemoteASRTranscriber.swift`：3,547 → 1,259 行。文件请求、Aliyun 流、Doubao 流和响应投影分离；录音/generation 状态仍归 transcriber，剩余有状态驱动属于阶段 5。
- `MeetingRemoteProviderLiveSession.swift`：1,869 → 51 行，仅保留工厂；已有 Base / Doubao / Aliyun Fun / Qwen 类型整体分离。基类 561 行，各 provider 文件均不超过 211 行。
- 原 `RemoteASRSupport.swift` 的 11 个支持声明按文本解析、端点、Aliyun、StepFun、Gemini 原样归位，新文件最大 359 行。
- 会议与短句复用同一份 `DoubaoPacketCodec`，移除重复常量、帧构造、gzip、整数序号/文本提取处理和未调用的旧文件上传实现。保留 dictation 全文与 meeting utterance 时间片段的不同投影。
- Aliyun 端点解析复用已有 `RemoteASREndpointSupport`；PCM 转换复用原 `RemoteASRTranscriber` 的静态实现，不改变采样算法。会议的模型路由和认证头保持原策略。

### 有意改变的行为（不是仅移动代码）

1. **有界解压、拒绝损坏报文**：豆包 WebSocket 短句路径现在与会议一样限制压缩输入 2 MiB、解压输出 8 MiB、扩张比 64（允许 1 MiB 基础窗口）。损坏 gzip 不再当成普通文本回退；未知压缩类型明确失败。整个帧在复制/解码前也受尺寸限制。自建兼容服务的非规范响应需要人工复核。
2. **终包和序号**：区分 flag 2 无序号终包与 flag 3 负序号终包；不再把时间戳等任意 JSON 数字识别成 sequence，不让越界整数转换崩溃。非 JSON 文本回退及 JSON metadata 不会抹掉线上的终包标记。
3. **会议完成仅一次**：提前完成先记录终态；重复 finish 共用完成结果和截止时间，重复回调不会重复 `.finished`。超时从停止请求开始计时，保留原 1.8 秒预算，并覆盖握手/发送期间的等待。
4. **有序 drain 与取消**：握手完成时先发送已缓冲音频，再发送 finish；等待期间的 append 不会越过队列。取消清理队列，不把取消的 partial 提升为 final。失败时在 `.failed` 移除会话 token 前保存可用 partial，然后关闭 socket / receiver / keepalive。
5. **响应 actor 冻结终态**：5 类 provider 的最终等待传播取消，终态、超时返回或取消后不再接受迟到文本/错误。保留各 provider 的拼接规则及 StepFun / Gemini grace window。
6. **握手与错误隔离**：文件转录的握手 gate 记录成功/失败，增加 20 秒上限并响应取消；所有退出路径清理接收任务。错误回调绑定创建时的 generation，不能污染新录音；取消不触发 partial fallback 或交付。

### 新增回归与边界

| 套件 | 方法数 | 重点 |
| --- | ---: | --- |
| `DoubaoPacketCodecTests` | 13 | 帧布局、负序号、无序号终包、截断、gzip 限制、非零 Data 索引、两种文本投影 |
| `RemoteASRResponseStateTests` | 10 | provider 终态、partial drain、取消、迟到事件、握手成功/失败/超时 |
| `RemoteASRCompletionTests` | 5 | 有/无 partial 的失败、取消不交付、旧 generation 的结果/错误隔离 |
| `MeetingRemoteSessionLifecycleTests` | 8 | 提前确认、并发 finish、可控超时、握手/发送失败、取消 drain、重复完成 |

Fake 会话复用真实基类，通过既有 override 边界注入故障；截止时间可控，不依赖真实服务或麦克风。测试不等于真实 URLSession WebSocket、provider 账户/服务行为、设备或模型质量验收。远程 LLM 的 transport 故障注入不包含在本批。

`refactor` 回归组已包含这些测试和原有 ASR/会议协议覆盖。阶段 4 结束时应用 440 个 Swift 文件、152,041 行，24 个文件仍 >1,000 行；测试 190 个 Swift 文件，静态 XCTest 方法增加至 1,621。

## 阶段 5A：在途任务与会话所有权

本批优先减少平行状态和竞态，不为达成文件行数目标继续扩大 private 成员访问范围。

### 实施内容

- `TrackedTaskStore`：统一 MainActor 任务登记、取消和退出等待；每次 invocation 使用独立 ID。取消不删除在途任务，任务退出才注销，避免同一业务请求 ID 的旧任务清掉新任务。
- `LLMRequestLifecycle`：收敛 current request ID 与任务集合；AppDelegate 不再直接持有 `activeLLMRequestID` / `llmTasksByRequestID`。旧请求不能入队或执行，已取消但仍在清理的任务继续阻止深度空闲回收。
- 录音启动通过 `TrackedTaskStore` 管理；新启动等待所有旧启动退出后才操作同一 transcriber/音频引擎。取消只是请求，不能假设 CoreAudio 或权限请求立即终止。
- `MeetingLiveSessionRegistry`：session 与 token 成对存储，区分 active 与 draining。drain 期间的最终文本仍有效；被替换/取消后，旧 token 的 partial、final、failed、finished 都被拒绝。旧 finish 返回不能清掉新 session。
- 会议音频提交取消后仍被跟踪；移除 completed-ID 辅助集合和按 `isCancelled` 提前丢任务的逻辑。
- 会议资源清理现在捕获旧 transcriber/session/model use，按单一 cleanup 屏障等待在途提交与 scheduler drain 后再重置 VAD、archive 并释放 model use。新会议/文件导入等待旧清理完成，避免异步 cleanup 误取消新会话。
- 会话 revision 在清理时立即失效；capture epoch 在清理时递增，旧麦克风回调在修改电平/启动 watchdog 状态前被拒绝。chunk 推理 await 前后校验会话有效性。
- `MLXCorrectionPassCoordinator`：统一校正 pass ID、kind、task 所有权；被取消的 pass 保留串行槽位到真正退出，新 pass 不与旧推理重叠。父任务取消传播到实际推理，过期 revision 不启动/返回输出。
- MLX 最终化在 archive await 后再次核验 revision / cancellation，过期归档清理而不是接管新会话输出。未改变模型参数、校正策略和原生 stream 的模型 pin 规则。

### 测试与范围限制

新增 20 个确定性测试：`TrackedTaskStoreTests`（5）、`LLMRequestLifecycleTests`（4）、`MeetingLiveSessionRegistryTests`（5）、`MLXCorrectionPassCoordinatorTests`（5），以及 `MeetingCaptureTimelineTests` 的 cleanup epoch 用例。共享 `ManualTaskBarrier` 模拟忽略取消、仍需释放的在途原生操作，不靠 sleep 推测时序。均纳入 `refactor` 回归组。

这些测试覆盖提取出的所有者契约；尚不能替代真实 CoreAudio、完整 MeetingSessionCoordinator 启停、模型推理与内存回收的集成验收。本批主动改变了启动串行化、清理等待及旧事件拒绝语义，需要重点验证快速连按、取消后重启、暂停恢复、切换双音源、应用退出。

5A 结束时，热键监听和 native-live 的 task/use 所有权留到 5B；应用完整会话、模型管理器和会议导入/最终化继续作为剩余工作，不因通过单元测试而视为完成。

## 阶段 5B：热键监听与 native-live owner

### 热键

- `HotkeyEventTapInstallation` 拥有一个 tap、source、callback context 和独立 run loop。停止时从 manager 的路由锁内摘出旧 owner，在锁外等待旧线程退出；新安装的线程不会被旧 stop 操作关闭。
- CGEvent 回调不再直接携带未保留的 HotkeyManager 指针，而是使用安装对象持有的 context / weak manager。source 移除块保留 context 到回调线程执行清理，避免释放过程中遗留裸 manager 指针。
- `HotkeyEventTapRunLoop` 将启动中的 thread 也纳入条件变量管理。启动超时后 stop 先记录停止请求，即使线程稍后才开始，也不会再进入长期运行状态；停止后的 owner 不复用。
- deferred event / 恢复请求校验安装代次；主队列业务回调校验状态代次，stop/reset 后不再投递旧 action。实际 App callback 在路由锁外执行，避免重新引入 event tap 超时。
- 权限重试不在 sleep 期间强持有 manager；stop 使重试 ID 失效。取消的长按/双击 fallback task 在拿锁后再次检查取消，不能命中后来复用的 binding ID。
- 六种业务的 36 个平行字段归并为 `HotkeyBusinessState` 记录，统一状态读写和 reset；不改快捷键优先级、鼠标/修饰键/长按/双击算法。对 14 个核心路由方法逆向还原存储替换后，正文与上批一致。
- 删除 4 个未调用的旧 tap-cancel helper 和 `clearNoteTransientState`。`HotkeyManager.swift` 从 2,293 行降至 1,858 行，仍需继续按边界整理，不机械拆文件。

### Native MLX

- `MLXNativeLiveRuntime` 将 installed session、event/feed task 和转交的 model-use release 配对。替换先摘除旧 owner；旧 event 不能更新新状态，旧 retirement 不能清除新 stream。
- retirement task 等待 Voxt 的两个任务退出后恰好释放一次 model use；关机等待全部 retirement。它不是可跳过的普通取消任务，避免取消 cleanup 自身漏掉 use 释放。未显式关闭而被销毁的 owner 通过 isolated deinit 取消 stream，并仅捕获旧 entry 完成退出/use 释放，不在析构后捕获 self。
- native setup 复用 `TrackedTaskStore`，被取消的 setup 保留至退出；Qwen / streaming / Nemotron 三份加载和 pin-transfer 流程合并，具体模型的 StreamingConfig 保持原值。
- 空闲回收同时检查 setup 和 retirement，不能仅因 UI 已停止就销毁仍在退出的 runtime。`MLXTranscriber.swift` 从本批前 2,223 行降至 2,065 行。
- **依赖边界限制**：检查了固定 Audio revision 的源码，库的同步 `cancel()` 仅发出取消/关闭事件流，不提供等待内部 decode / Metal 工作完全退出的 API。因此这里保证的是 Voxt task/use owner 的退出顺序，不能声称已经证明底层推理完全静止；真正的 native quiescence 仍需模型回放及必要的依赖 API 支持。

新增 `HotkeyManagerLifetimeTests`（4）、`HotkeyEventTapRunLoopTests`（4）、`MLXNativeLiveRuntimeTests`（7），共 15 项；纳入 `refactor`，静态 XCTest 方法数为 1,656。run-loop 测试创建专用线程和普通 CF source，不安装系统 event tap、不请求权限；runtime 测试使用 fake stream，不加载模型。真实事件监听、权限恢复、睡眠唤醒和模型内存行为仍须人工/模型验收。

上述内容是 5B 的完成边界。5C 对录音身份、模型加载和会议导入/最终化的后续处理见下一节；阶段 6 的剩余大文件与最终回归尚未实施。

## 阶段 5C：录音身份、模型加载退出和会议导入/最终化

### 录音与结束流程

- `RecordingSessionLifecycle` 统一 session ID、取消、单次输出认领、正在结束及已结束标记；录音/选中翻译/失败复位/应用退出使用明确的 begin/cancel/invalidate 转移，不再分散写五个字段。
- 取消立即使旧输出无效，但保留取消前 ID 的清理资格；新会话开始后旧结束请求不能再清理新会话。旧 complete-end 回调也不能清掉新 ending ID。
- `SessionEndFlow` 删除固定顺序上的 protocol + 5 个 stage 包装，按原顺序直接执行隐藏界面、恢复音量、结束音、复位和残余捕获清理；171 → 98 行。
- 原静态 end-decision helper 在迁移后只剩测试引用，已删除；3 个已有 end-flow 测试改测真正的生命周期转移，保留原断言意图，未为删行而删除测试。
- 输出认领不再让已取消会话进入交付流程。实际文本注入、历史/词典快照和 UI 状态仍在原组件内；这不是宣称全部 AppDelegate 状态或外部编辑器事务已经解耦。

### 模型加载与关机

- `SharedModelLoadCoordinator<Value>` 归入 `Core/Models/`，去掉 `Any` 模型值和 `as! Value`，只对退出等待句柄做类型擦除。
- 分开“仍可共享给 waiter 的当前 load”和“取消后尚未退出的 load”。`cancelAll` 保留后者，后续应用关机仍能等待，不再依赖只覆盖特定调用路径的额外 termination 数组。
- 过期 generation 即使晚到的是错误而非结果，也转为 CancellationError，避免写坏替代 load 的模型状态。
- ASR / Custom LLM 的深度空闲回收检查改用 outstanding load，旧 cancelled native loader 退出前不再当作空闲。原 `hasPendingModelLoad` 保留当前 waiter 语义。
- 两个 manager 的重复 shutdown 调用共用完整退出 task，不只等待 active count 后提前返回。加载/下载/active use 全部退出后才释放缓存。`MLXModelManager.swift` 1,967 → 1,843 行。
- 不强制串行所有模型加载、不修改模型参数/目录，也不把 Swift load task 完成当作底层 Metal quiescence 证明。

### 文件导入与会议最终化

- `MeetingImportedFileAnalyzer` 在等待旧会议 cleanup **之前**就登记任务，关闭无法取消的空窗。cancel 捕获当次 pipeline/task；迟到取消不会命中新导入，调用方取消会传递到实际任务。
- `MeetingImportedFilePipeline` 独立拥有导入 transcriber、标准化音频路径和 model use，不再复用/修改 live coordinator 的 transcriber / active engine 字段。只有成功结果保留音频；失败或清理中取消会删除临时结果，清理可重复执行。
- 导入在清理结束前保持 busy；owner 析构也会取消该次任务及 pipeline。文件队列的延迟取消同样复核 task ID 和 cancelling 状态。
- `MeetingFinalizationContext` 固定停止时的 session ID、capture mode、引擎/模型、时长和 visible snapshot；三次 recovery checkpoint 与最终结果复用同一元数据，移除三份重复构造。
- finalization task 在 checkpoint 收尾完成前持续占用会议生命周期；重复 stop 返回同一 task，不能在旧任务尚未退出时开始新会议再被旧 task 清引用。
- `MeetingSessionCoordinator.swift` 1,996 → 1,821 行；文件导入代码移到独立资源所有者，不是仅把 coordinator 的 private 状态改成 internal 后拆 extension。

### 覆盖和未完成项

新增：`SharedModelLoadCoordinatorTests`（6）、`MeetingImportedFileAnalyzerTests`（7）、`RecordingSessionLifecycleTests`（6）、`MeetingFinalizationContextTests`（3），共 22 项；静态 XCTest 方法数 1,678。聚焦组同时补入已有文件队列和 recovery checkpoint 测试。

测试用受控 task barrier / fake pipeline 覆盖取消窗口、并发拒绝、清理中取消、旧任务隔离、checkpoint 元数据和单次结束。真实文件解码、模型/设备生命周期、并发 shutdown 的完整硬件路径仍依赖 Mac 集成验收。

阶段 5 的这组核心边界已实施，不能推导出所有异步路径已逐行审计或全部大类已拆完。剩余 UI/编辑器事务快照、模型下载状态、大文件、孤儿代码/测试去重和性能基准继续列入阶段 6，库内部 native 退出限制保留为明确待验收项。

## 验证与下一门禁

Linux 已执行：

- Python 工具测试：10 项通过。
- Shell 语法检查、模型源码/锁文件审计、`git diff --check`：通过。
- 保留函数/测试正文、目录移动内容、删除引用与 Markdown 链接的静态核对。

阶段 5C 新提交仍需 macOS CI / Mac 验证；前几批绿色结果不能替代它。真实执行并记录结果：

```bash
xcodebuild build -project Voxt.xcodeproj -scheme Voxt -configuration Debug -destination 'platform=macOS' CODE_SIGNING_ALLOWED=NO
xcodebuild build -project Voxt.xcodeproj -scheme Voxt -configuration Release -destination 'platform=macOS' CODE_SIGNING_ALLOWED=NO
bash tools/run_local_regression_matrix.sh refactor
xcodebuild test -project Voxt.xcodeproj -scheme Voxt -destination 'platform=macOS' CODE_SIGNING_ALLOWED=NO
```

人工检查：六步引导的前进/后退/关闭、三种练习、权限和麦克风切换；设置导航、通知、反馈；会议远程启动配置；本地 ASR live/final/取消。核对新 suite 的测试发现数量，不能只看 xcodebuild 退出码。

阶段 4 已补充 ASR/会议协议契约，阶段 5A–5C 收敛了上述核心所有者。阶段 6 继续整理剩余职责与集成验收；远程 LLM 的流式重试故障注入仍需单独补齐。当前没有实测延迟、峰值内存和编译时间数据，不宣称性能已提升。
