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
| 4 | Remote ASR / 会议 provider 会话契约与去重 | 已实施，新增 36 个定向测试 | 需本批提交的 CI；真实 provider 人工验收待执行 |
| 5 | AppDelegate、录音、会议、热键、模型生命周期的状态所有权 | 待实施 | 以阶段 0–3 Mac 通过为前置条件 |
| 6 | Dictionary/History/MeetingDetail 等剩余大文件与最终验收 | 待实施 | 分域推进；完整构建、测试和人工回归 |

阶段 0–3 的 `fa087ed` 已通过 [macOS CI Tests 工作流](https://github.com/hehehai/voxt/actions/runs/35433366619)。这是前一批的证据，不能替代阶段 4 新增行为的编译和测试，也不代表 Release 构建、模型回放及真实设备验收已完成。

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

`refactor` 回归组已包含这些测试和原有 ASR/会议协议覆盖。应用现有 440 个 Swift 文件、152,041 行，24 个文件仍 >1,000 行；测试 190 个 Swift 文件，静态 XCTest 方法增加至 1,621。阶段 5–6 尚未实施。

## 验证与下一门禁

Linux 已执行：

- Python 工具测试：10 项通过。
- Shell 语法检查、模型源码/锁文件审计、`git diff --check`：通过。
- 保留函数/测试正文、目录移动内容、删除引用与 Markdown 链接的静态核对。

阶段 4 新提交仍需 macOS CI / Mac 验证；通过后再进入阶段 5。真实执行并记录结果：

```bash
xcodebuild build -project Voxt.xcodeproj -scheme Voxt -configuration Debug -destination 'platform=macOS' CODE_SIGNING_ALLOWED=NO
xcodebuild build -project Voxt.xcodeproj -scheme Voxt -configuration Release -destination 'platform=macOS' CODE_SIGNING_ALLOWED=NO
bash tools/run_local_regression_matrix.sh refactor
xcodebuild test -project Voxt.xcodeproj -scheme Voxt -destination 'platform=macOS' CODE_SIGNING_ALLOWED=NO
```

人工检查：六步引导的前进/后退/关闭、三种练习、权限和麦克风切换；设置导航、通知、反馈；会议远程启动配置；本地 ASR live/final/取消。核对新 suite 的测试发现数量，不能只看 xcodebuild 退出码。

阶段 4 已补充上述 ASR/会议契约。阶段 5 再确定应用级 task、session ID、音频缓冲和模型 lease 的唯一所有者；远程 LLM 的流式重试故障注入仍需单独补齐。当前没有实测延迟、峰值内存和编译时间数据，不宣称性能已提升。
