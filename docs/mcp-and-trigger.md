# VoxtMini 新功能方案：MCP 支持与调用式触发器

> 状态：**已实现（P0–P3 已完成并完成 Linux 联调；P4 Mac 真机联调待排期）** · 日期：2026-11-25  
> 适用范围：VoxtMini（macOS，Go + MyGo + Swift Helper）  
> 实际打包路径：`VoxtMini.app/Contents/Resources/ctl/voxtmini-ctl`（单一轻量二进制，同时提供 `mcp` stdio 桥与 `trigger`/`devices`/`status` 控制子命令）
> 架构参考：Inboxly 的 MCP 实现（`/home/guanwei/helper/Inboxly/internal/mcp`、`internal/mcpbridge`、`cmd/inboxly-mcp`、`docs/design/mcp-devmode.zh-CN.md`）

---

## 一、背景与目标

本次规划包含两项互相关联的开放能力，**均在设置中提供开关，默认关闭**：

1. **MCP 支持**：参考 Inboxly 的本机 MCP 架构，让用户自己的 AI Agent（Codex / ChatGPT 桌面版、Claude Code / Desktop、Cursor、WorkBuddy 等）通过标准 MCP 协议读写 VoxtMini 的业务数据（词典、历史记录、应用增强规则），并在授权范围内调用 VoxtMini 的本地/已配置模型（语音识别、翻译、转写、文本推理）。
2. **调用式触发器（External Trigger）**：为外部硬件伴侣 App（如自带按键的麦克风设备 App、Stream Deck、快捷指令、脚本等）提供标准化的唤醒与控制方式，使其在外部触发转录、翻译、转写等功能，并能在**唤醒的那一刻指定或切换本次会话使用的麦克风设备**，同时不抢占当前输入焦点。

两项能力在底层共用一套本机控制通道与权限管理，避免重复造轮子，同时遵守 VoxtMini 的核心原则：**本地优先、隐私透明、默认关闭、不引入常驻公网/TCP 暴露面**。

---

## 二、功能一：VoxtMini MCP 支持

### 2.1 整体架构（参考 Inboxly）

直接复用 Inboxly 已在真机验证过的「**轻量 stdio 桥 + 本机 Unix 套接字 + App 进程内服务**」模式：

```
AI Agent 客户端（Codex / ChatGPT / Claude / Cursor / WorkBuddy …）
   │ stdio（由客户端按需拉起）
   ▼
VoxtMini.app/Contents/Helpers/voxtmini-mcp   ← 独立轻量二进制（约 3–5 MB），只做字节转发，不开 UI、不开库
   │ Unix 套接字：~/Library/Application Support/VoxtMini/run/mcp.sock（权限 0600，目录 0700）
   │ 首行握手头：标识发起进程与客户端信息，后续原样转发 JSON-RPC
   ▼
运行中的 VoxtMini 主进程：internal/mcp.Server（官方 Go SDK github.com/modelcontextprotocol/go-sdk）
   ├── 历史 & 词典 & 应用增强（internal/store）
   ├── 本地/远程模型调度（internal/local、internal/remote）
   ├── 语音会话与麦克风（internal/session、internal/helper）
   └── 审计日志与权限闸门（mcp_calls 表 + gate）
```

**为什么采用独立小二进制 `voxtmini-mcp` 而非直接跑主程序？**
- Inboxly 在实测中发现，ChatGPT/Codex 每个对话会同时拉起约 10 个 MCP 桥进程；如果桥是主程序本身，每个进程会占用完整的 Go + UI 运行时内存，而独立的 `voxtmini-mcp` 只导入极少依赖，体积仅几 MB，内存开销极低。
- 放在 `Contents/Helpers/voxtmini-mcp`，随主 App 一同签名公证，不需要额外沙盒权限。
- **不开 TCP 端口、不需要 Bearer 令牌**：通过 Unix 套接字文件权限（0600 / 目录 0700）和内核对端进程校验（`LOCAL_PEERPID` / 签名 Team ID）识别连接来源。
- **自动拉起 App**：客户端连进来时，桥先读数据目录的 `run/mcp.json`（无需开 SQLite）。若开关已开启但 VoxtMini 未运行，则 `open -g -b dev.hehehai.voxtmini` 后台拉起并等待套接字就绪（最多 45 秒）；若开关关闭，桥直接返回清晰的 JSON-RPC 错误，提示用户去设置页开启。
- **协议兼容**：使用 `github.com/modelcontextprotocol/go-sdk`，同时兼容 MCP 2026-07-28（无状态 `server/discover`）与 2025-11-25（`initialize` 握手）。

---

### 2.2 权限分级与隐私边界

| 开关 / 权限档 | 默认值 | 说明 |
|---|---|---|
| **总开关：启用 MCP 服务** | **关** | 首次开启时弹出简短确认框，明确告知：Agent 读到的历史、词典等内容会发往该 Agent 自己的模型服务商；MCP 仅在本机监听，不对外网开放 |
| **只读访问** | 随总开关开启 | 查看状态、历史记录、词典、应用增强规则、可用模型列表、麦克风列表（**永不暴露 API Key**） |
| **管理词典、历史与规则** | **关** | 允许 Agent 增删改词条/分类、增删改应用增强分组、删除单条历史或发起历史追问；批量删除/清空需在 App 内弹窗确认 |
| **调用本地模型** | **关** | 允许 Agent 调用已安装的本地语音/语言模型（含 Apple Speech / Apple Intelligence）执行转录、翻译、转写或文本推理 |
| **允许调用远程模型** | **关**（需先开“调用本地模型”） | 明确单独授权，防止 Agent 意外消耗用户远程 API 额度或把文本发往远程服务商 |
| **触发录音会话** | **关** | 允许 Agent 唤醒/停止 VoxtMini 的录音功能（与功能二共用触发管线） |

**补充保护机制（沿用 Inboxly 经验）：**
- **Agent 首次连接确认**：新客户端（如 Codex、Claude）首次接入或首次执行写入/调用模型时，在 VoxtMini 内弹窗询问（拒绝 / 允许一次 / 1 分钟内允许 / 总是允许此 Agent）。
- **频率与大小限制**：按 Agent 限流（例如每分钟 120 次调用），列表分页上限 50 条，音频输入限制大小与时长；防止失控循环。
- **审计日志**：在 `voxtmini.db` 新增 `mcp_calls` 表，记录时间、Agent、工具名、结果、耗时（**不记录敏感正文与密钥**），保留 30 天或 5000 条，在设置页可查看和清空。

---

### 2.3 MCP 工具清单（按业务模块）

工具总数控制在 **16–18 个以内**（保持 `tools/list` 精简，节省 Agent 上下文）：

#### A. 概览与只读（总开关开即可用）
| 工具名 | 参数 | 返回与用途 |
|---|---|---|
| `voxtmini_overview` | — | **推荐第一步调用**：返回 App 状态、当前录音会话状态、已开启的权限档、词典/历史/应用增强数量、已就绪的本地与远程模型概况、当前可用麦克风列表 |
| `list_history` | `kind`（transcribe/translate/rewrite）、`query`、`limit`（≤50）、`cursor` | 检索历史记录摘要（id、时间、功能、来源 App、结果预览、是否有录音） |
| `get_history` | `id` | 获取单条历史详情：识别原文、最终结果、命中的词典纠错、追问记录、音频元数据（时长、路径，不直接返回大音频） |
| `list_terms` | `search`、`category` | 列出词典词条（拼写、听写提示 hints、分类、应用分组作用域、启用状态、使用次数） |
| `list_categories` | — | 列出词典分类 |
| `list_app_groups` | — | 列出应用增强分组（名称、包含的 App/网站、自定义 prompt、插入后按键） |
| `list_models` | `kind`（speech/language/all） | 列出可用模型（ID、名称、本地/Apple/远程、是否已安装/就绪、适用功能），**绝不包含 Key** |
| `list_microphones` | — | 列出当前 Mac 可用的输入设备（Core Audio UID、名称、传输类型 usb/bluetooth/builtin、是否系统默认、在 VoxtMini 中的优先级） |

#### B. 数据管理（需开启「管理词典、历史与规则」）
| 工具名 | 参数 | 返回与用途 |
|---|---|---|
| `upsert_terms` | `terms[]`（单条或批量：spelling、hints、category、group、enabled） | 新增或更新词典词条（支持 Agent 从项目文档/代码库提取术语后批量写入） |
| `delete_term` | `id` | 删除指定词条 |
| `upsert_category` | `id`（可选）、`name` | 新建或重命名词典分类 |
| `upsert_app_group` | `id`（可选）、`name`、`apps[]`、`domains[]`、`prompt`、`after_key` | 新建或更新应用增强分组 |
| `delete_app_group` | `id` | 删除应用增强分组 |
| `delete_history` | `id` | 删除单条历史（含关联录音）；不提供一键清空全部的 MCP 工具 |

#### C. 模型调用（需开启「调用本地模型」；使用远程模型需另开子开关）
| 工具名 | 参数 | 返回与用途 |
|---|---|---|
| `transcribe_audio` | `audio_path`（本机 `.wav`/`.m4a`/`.mp3` 路径）、`model_id`（可选，默认自动选本地）、`cleanup`（bool，是否用 LLM 润色）、`language`（可选） | 调用 VoxtMini 语音识别管线（自动应用词典 hints 与简体中文规范化、日文保护），返回识别文本与耗时 |
| `translate_text` | `text`、`target_language`（en/zh-Hans/ja）、`model_id`（可选） | 调用 VoxtMini 翻译管线（含词典术语表、简中规范化、日文保护） |
| `rewrite_text` | `instruction`、`text`（可选）、`model_id`（可选） | 调用 VoxtMini 转写管线，按指令改写或生成文本 |
| `chat_model` | `prompt`、`system`（可选）、`model_id`（可选，默认本地推荐语言模型） | 直接调用 VoxtMini 管理的模型进行单次推理，复用已常驻的本地 MLX 运行时，避免 Agent 重复加载大模型 |

#### D. 会话控制（需开启「触发录音会话」）
| 工具名 | 参数 | 返回与用途 |
|---|---|---|
| `trigger_session` | `feature`（transcribe/translate/rewrite）、`action`（start/stop/toggle/cancel）、`device`（可选 UID/名称）、`target_language`（可选）、`deliver`（cursor/clipboard/card） | 与功能二完全共用同一套触发器服务 |
| `session_status` | — | 返回当前会话状态（idle/recording/processing/done/failed）、正在使用的麦克风、最近一次结果 |

### 2.4 一键接入与 Skill

参考 Inboxly 的做法，降低用户配置成本：
- **内置 Skill（`internal/mcp/skill/voxtmini/SKILL.md`）**：通过 `go:embed` 打包进二进制，提供一键安装到 `~/.agents/skills/voxtmini/`（Codex / ChatGPT / Cursor / VS Code / Gemini）、`~/.claude/skills/voxtmini/`、`~/.workbuddy/skills/voxtmini/`。
- **一键写入客户端配置**：在设置页提供针对 Codex / ChatGPT、Claude Code / Desktop、Cursor、WorkBuddy 的「添加」与「拷贝」按钮（写入前展示 diff 并备份，只改 `voxtmini` 节点）。
- **连接自检**：设置页提供「测试连接」按钮，后台运行 `voxtmini-mcp --self-test` 走完握手 → `tools/list` → `voxtmini_overview`，显示耗时与工具数。

---

## 三、功能二：调用式触发器与硬件麦克风联动

### 3.1 业务场景拆解

典型用户场景：用户有一个**绑定了自有硬件麦克风的配套 App**（例如蓝牙领夹麦/会议麦/AI 录音笔/自定义快捷键硬件的 macOS 伴侣程序），希望在按下硬件按键或点击该 App 按钮时：
1. **唤醒 VoxtMini 的指定功能**：转录（Transcribe）、翻译（Translate）、转写（Rewrite），以及对应的停止/取消；
2. **在唤醒那一刻指定或处理麦克风设备**：例如强制本次录音走该硬件对应的 Core Audio 麦克风（哪怕 macOS 系统默认麦克风仍是 MacBook 内置麦或 AirPods），而不破坏用户在 VoxtMini 设置里配好的默认麦克风排序；
3. **不打断用户当前工作流**：唤醒时不能把 VoxtMini 主窗口切到前台抢焦点，确保文字能准确插入用户原本光标所在的文档/聊天框；或者按调用方要求，把结果交给调用方自己处理（比如显示在硬件伴侣界面上）。

---

### 3.2 业务形态设计：双通道、统一触发引擎

在 VoxtMini 内部抽象一个 **`internal/trigger`（TriggerService）**，统一接收外部触发请求，校验开关与参数后，调用现有的 `session.Manager`。对外提供**两种互补的调用形态**（再加上 MCP 工具，共三个入口，逻辑完全一致）：

```
  [外部硬件 App / Shortcuts / Raycast]        [硬件伴侣守护进程 / 脚本 / 进阶集成]         [AI Agent]
                 │                                            │                                │
     方式 A：URL Scheme                           方式 B：CLI / Unix Socket                方式 C：MCP
   voxtmini://trigger/...                     VoxtMini.app/.../voxtmini-ctl ...        trigger_session
   (单向、零依赖、可自动拉起)                    (双向、毫秒级、拿返回值/结果/设备列表)       (复用 mcp.sock)
                 │                                            │                                │
                 └────────────────────────────────────────────┴────────────────────────────────┘
                                                              ▼
                                           internal/trigger.Service（统一校验、鉴权、设备解析）
                                                              ▼
                                           internal/session.Manager（单会话状态机 + 本次设备覆盖）
                                                              ▼
                                           Swift Helper（capture.start 带本次优先设备列表）
```

#### 方式 A：URL Scheme（`voxtmini://trigger/...`）——最简单、通用

适合所有 macOS App（Swift `NSWorkspace.shared.open`、Electron `shell.openExternal`、Apple Shortcuts、Stream Deck、Raycast 等），**一行代码即可接入**。

- **命令格式**：
  - `voxtmini://trigger/transcribe?action=toggle&device=<uid-or-name>`
  - `voxtmini://trigger/transcribe?action=start&device=<uid>&fallback=default`
  - `voxtmini://trigger/translate?action=toggle&to=en&device=<uid>`
  - `voxtmini://trigger/rewrite?action=toggle&device=<uid>`
  - `voxtmini://trigger/stop`（停止当前录音并进入识别/输出）
  - `voxtmini://trigger/cancel`（取消当前会话，不输出）
- **核心参数**：

| 参数 | 可选值 | 默认值 | 说明 |
|---|---|---|---|
| `action` | `toggle` / `start` / `stop` / `cancel` | `toggle` | 硬件单键按一下开始、再按一下结束用 `toggle`；按住说话（Push-to-Talk）按下发 `start`、松开发 `stop` |
| `device` | Core Audio UID、设备名称、或传输类型（如 `usb` / `bluetooth`） | 空（沿用设置） | **仅对本次会话生效**，不改写用户设置中的麦克风列表 |
| `fallback` | `default` / `fail` | `default` | 指定的硬件麦克风未就绪/未连接时：回退到 VoxtMini 默认麦克风，还是直接报错提示 |
| `wait_device_ms` | `0` – `3000` | `800` | 蓝牙/USB 硬件唤醒时设备注册常有短暂延迟，允许在启动录音前最多等待指定毫秒让设备上线 |
| `to` | `en` / `zh-Hans` / `ja` | 沿用设置 | 仅 Translate 有效，临时覆盖目标语言 |
| `deliver` | `cursor` / `clipboard` / `card` | 沿用设置 | 指定结果交付方式：插入光标（默认）、仅写剪贴板、或仅在浮窗卡片显示 |
| `source` | 任意短字符串（如 `my-mic-app`） | 空 | 调用方标识，记入日志与审计，方便排查 |

- **不抢焦点保证**：硬件 App 调用时建议使用 `NSWorkspace.OpenConfiguration.activates = false`（或命令行 `open -g`）；VoxtMini 处理 `voxtmini://trigger/*` 时**绝不激活主窗口**，只弹出非抢焦点的录音浮窗，并保持当前前台 App 为输入目标（`Target`）。

#### 方式 B：CLI / 本机控制通道（`voxtmini-ctl` 或 `voxtmini-mcp` 同二进制多子命令）——双向交互、硬件深度集成

URL Scheme 的局限是**没有返回值**（不知道是否触发成功、用了哪个麦克风、是否因正在忙而失败，也拿不到设备列表和最终识别文本）。对于硬件伴侣 App，提供轻量命令行/Unix 套接字接口能彻底补齐短板：

- 复用同一个轻量辅助二进制（例如 `Contents/Helpers/voxtmini-ctl`，或直接合并进 `voxtmini-mcp`，带 `mcp` 子命令跑 MCP 桥，带 `trigger` / `devices` / `status` 子命令跑控制命令，只增加 1 个二进制，减少打包体积）。
- 典型调用：
  1. **发现麦克风**：`voxtmini-ctl devices --json` → 返回所有输入设备的精确 Core Audio `id`（UID）、`name`、`transport`、`default`。硬件 App 可据此自动匹配自己的硬件 UID。
  2. **按住说话（Push-to-Talk）**：
     - 按下按键：`voxtmini-ctl trigger transcribe --action start --device "<UID>" --wait-device 1000 --json`  
       立即返回 `{"ok": true, "session_id": "...", "state": "recording", "device": {"id": "...", "name": "MyMic Pro", "fell_back": false}}`
     - 松开按键：`voxtmini-ctl trigger stop --json`（可加 `--wait` 阻塞等到本次识别/翻译/转写完成，直接返回 `{"ok": true, "text": "...", "delivered": true}`，硬件 App 就能在自己的界面或硬件屏幕上同步显示结果）。
  3. **查询状态**：`voxtmini-ctl status --json` → 返回当前是 `idle` / `recording` / `processing` 以及当前使用的麦克风。
  4. **单次静默监听（可选二期）**：`voxtmini-ctl watch --json` 输出 JSONL 事件流（录音开始、电平、结束、结果），让硬件 LED 灯或伴侣 App 实时同步录音状态（例如硬件灯变红表示正在录音、变绿表示识别完成）。

---

### 3.3 唤醒时的麦克风处理机制（核心细节）

结合 VoxtMini 现有架构（`internal/session/session.go` 的 `StartRequest` 和 `internal/helper/proto/capture.go` 的 `CaptureParams.Devices`），麦克风联动不需要大改 Swift Helper，改动非常克制：

1. **会话级设备覆盖（不污染用户设置）**：
   - 在 `session.StartRequest` 新增可选字段 `DeviceOverride *DeviceOverride`（包含 `Query string`、`Fallback string`、`WaitMS int`）。
   - 会话启动时，如果存在 `DeviceOverride`：
     - 从 Helper 当前缓存或实时调用 `capture.devices` 解析目标设备：优先精确匹配 Core Audio `UID`，其次精确匹配 `Name`，再次区分大小写子串匹配或 `Transport`（如 `usb`/`bluetooth`）。
     - 若设备尚未出现且 `WaitMS > 0`（如蓝牙麦刚被按键唤醒、正在切 HFP），以短间隔轮询 `capture.devices` 直至出现或超时（上限 3 秒）。
     - 解析成功后，构造本次 `CaptureParams.Devices`：
       - `fallback=default`：`[目标设备UID, 用户原设置中的设备列表...]`；
       - `fallback=fail`：仅传目标设备且若 Helper 报告 `FellBack=true` 或未找到设备，直接终止并返回明确错误（`device_not_found`），不误用内置麦。
   - **会话结束后自动恢复**：因为没有修改 `settings.Audio.Devices`，下次用户按键盘快捷键时，仍然使用原本配置的麦克风顺序。
2. **可选的“记住硬件绑定”**：
   - 如果外部硬件 App 不想每次传 UID，也可以在 VoxtMini 的「外部触发」设置里（或后续迭代中）为某个 `source` 指定默认绑定的麦克风，不过 v1 优先由调用方按次传入 `device`，最简单、无隐式状态。
3. **冲突与状态处理**：
   - `toggle`：空闲 → 开始录音；正在录音（同功能）→ 停止并处理；正在处理中 → 忽略并返回 `busy`。
   - `start`：空闲 → 开始录音；已在录音/处理 → 返回 `busy`（避免硬件按键抖动触发两次）。
   - `stop`：正在录音 → 停止并处理；空闲 → 幂等返回 `idle`。
   - `cancel`：正在录音/处理 → 取消；空闲 → 幂等返回 `idle`。
   - **防抖**：对同一 `source` 在 200ms 内的重复 `toggle` 自动去重，防止硬件微动开关抖动。

---

### 3.4 安全与防滥用

| 风险 | 对策 |
|---|---|
| **恶意网页通过 `voxtmini://trigger/transcribe` 偷开麦克风** | ① **外部触发默认关闭**，未开启时所有 `voxtmini://trigger/*` 直接忽略并弹轻提示（现有页面跳转 `voxtmini://models` 等不受影响）；② 即使开启，录音时一定会弹出明显的录音浮窗并播放开始提示音，无法“静默偷录”；③ 可选在设置中提供「允许的调用方式」：仅本地 CLI / 同时允许 URL Scheme |
| **外部触发时 VoxtMini 未授予麦克风/辅助功能权限** | 沿用现有权限检查逻辑：缺权限时弹清晰提示引导授权，CLI 返回结构化错误码 `permission_denied` |
| **外部程序在用户正在用快捷键录音时强行打断** | 默认返回 `busy`，不强行打断正在进行的会话（除非显式发 `cancel`） |

---

## 四、设置界面设计（符合 AudioSmith / VoxtMini 简洁原则）

在设置对话框中新增一节：**「开发与集成」**（或 **「开发者」**，`/settings/developer`，放在「隐私与数据」和「关于」之间；也可以叫 **「扩展」/「集成」**，待确认）。

顶部用分段切换（保持弹窗在 720×560 紧凑尺寸内，不加废话副标题，长说明放进 ⓘ）：

1. **「MCP」分段**：
   - 总开关：**启用 MCP 服务**（默认关）+ 状态行（已关闭 / 运行中 · N 个 Agent 已连接）+「测试连接」按钮。
   - 权限勾选：只读（随总开关）、管理词典与历史、调用本地模型、允许调用远程模型、触发录音会话。
   - 连接 Agent：Codex / ChatGPT、Claude、Cursor、WorkBuddy 一键「添加」/「拷贝」+ Skill 一键「安装 / 更新」。
   - 已连 Agent 列表（可「再次询问」或「移除」）与「记录」查看/清空入口。
2. **「外部触发」分段**：
   - 总开关：**允许外部应用触发**（默认关）+ ⓘ 说明（供硬件麦克风配套 App、快捷指令、脚本等唤醒转录/翻译/转写）。
   - 子开关：**允许通过 URL（`voxtmini://trigger/…`）触发**（开/关，让只用 CLI 的高安全用户可关闭 URL 入口）。
   - 快速参考区：显示 `voxtmini-ctl` 路径（可一键复制）、一条示例 URL（可一键复制）、当前已识别的麦克风列表及其 UID（方便开发者/用户直接复制 UID 填进硬件 App）。

---

## 五、分期实施路线（建议）

| 阶段 | 内容 | 交付物 |
|---|---|---|
| **P0：底座与控制通道** | `internal/mcpbridge` + 单一轻量二进制 `cmd/voxtmini-ctl`（同时支持 `mcp` 桥与 `trigger`/`devices`/`status` 子命令）+ 数据目录 `run/mcp.sock` & `run/control.sock` + `mygo.json` 打包与签名检查 | 桥与 CLI 可在 Linux 单元测试通过，Mac 签名检查通过 |
| **P1：外部触发器 + 麦克风覆盖** | `internal/trigger` + `session.StartRequest.DeviceOverride`（含等待设备上线与回退策略）+ `voxtmini://trigger/*` 路由 + `voxtmini-ctl trigger/devices/status` + 设置页「外部触发」分段 | 硬件伴侣 App 可通过 URL 或 CLI 唤醒三大功能并指定麦克风，不抢焦点 |
| **P2：MCP 只读 + 词典/历史/规则管理** | `internal/mcp`（官方 Go SDK、peer 校验、gate 首次确认、审计表 `mcp_calls`）+ 只读与管理类工具 + 设置页「MCP」分段 + 客户端一键配置 + `SKILL.md` | Codex / Claude / Cursor / WorkBuddy 可管理词典、查历史、改应用增强 |
| **P3：MCP 模型调用与会话工具** | `transcribe_audio`、`translate_text`、`rewrite_text`、`chat_model`、`trigger_session`、`session_status` + 本地/远程子开关与限流 | Agent 可直接复用 VoxtMini 本地模型，也可触发录音会话 |
| **P4：真机联调与文档** | Mac 实机验证（含多麦克风切换、不抢焦点、Codex/Claude 接入）+ 更新 `docs/` 面向外部开发者的接入文档 | 完整可用的发布级能力 |

---

## 六、需要你确认的几个决策点

1. **命名与设置入口**：设置里这一节叫 **「开发与集成」**、**「开发模式」**（与 Inboxly 一致），还是拆成独立的 **「MCP」** 和 **「外部触发」** 两节？
   - *建议*：合并为一节（如 **「集成」** 或 **「开发模式」**），顶部用分段切「MCP / 外部触发」，保持设置侧边栏精简。
2. **外部触发的调用方式**：**URL Scheme（`voxtmini://trigger/...`）+ 轻量 CLI（`voxtmini-ctl`）** 这套双通道是否满足那个硬件麦克风 App 用户的需求？是否还需要支持 **本机 HTTP API（`127.0.0.1` + Token）**（比如对方是纯网页或不想调命令行）？
   - *建议*：一期先做 URL Scheme + CLI（无端口冲突、无 Token 维护、可自动拉起、延迟低）；如果该用户明确反馈只能用 HTTP，二期再按 Refto 模式补一个带 Token 的本机 HTTP。
3. **MCP 调用模型的边界**：
   - 是否同意 **默认只允许调用本地模型（含 Apple 模型）**，把「允许 MCP 调用远程模型」做成一个默认关闭的子开关？
   - `transcribe_audio` 允许传入本机音频文件路径，是否需要加一个路径限制（例如仅允许用户主目录/临时目录下的音频文件，且限制大小如 ≤50 MB），避免 Agent 探测系统文件？
   - *建议*：都加上。
4. **独立辅助二进制**：参考 Inboxly，把 MCP stdio 桥和外部触发 CLI 合为**一个**轻量可执行文件 `Contents/Helpers/voxtmini-ctl`（约 3–5 MB，随 App 签名公证），是否同意？
   - *建议*：同意，比直接用主程序省内存，也方便硬件 App 和 Agent 接入。
5. **优先级**：两个功能是先做 **功能二（外部触发 + 麦克风联动）** 还是先做 **功能一（MCP）**，还是按上面的 P0→P3 顺序一起推进？
