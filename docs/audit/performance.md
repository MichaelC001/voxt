# 性能与资源审计（CPU、内存、磁盘、电量、网络、启动）

线程 #37，2026-11-14，基于 main 9943ec6。审计方式：读代码，在 Linux 上做测量（Go benchmark、假服务下的会话、SQLite），没有 Mac 访问。第一部分是给用户的中文报告，第二部分是英文工作计划（English work plan），按可以并行、互不冲突的工作包排列。

---

## 第一部分：报告（中文）

### 结论一句话

VoxtMini 空闲时本身很省：不预加载任何模型，没有高频轮询，会话结束后没有泄漏的 goroutine。**真正让 24 GB、swap 常年九成满的 Mac 吃力的，是用完之后还留在内存里的模型**：语音模型约 3.7 GB 留 10 分钟，语言模型 3.4–4.3 GB 留 5 分钟，而且语言模型的 MLX 缓冲区缓存没有上限。其他问题（空闲唤醒、日志、磁盘）都是小数目，但修起来也便宜。

### 按对用户的影响排序

| # | 问题 | 证据 | 影响 | 修法 | 工作量 / 风险 |
|---|---|---|---|---|---|
| 1 | **用完的模型留在内存里，不看内存压力** | `internal/omni/runtime.go:31`（语音 10 分钟）、`internal/llmrt/llmrt.go:71`（语言 5 分钟）；`models_keep.go` 的“自动”就是这两个固定值。eval.md:218：swap 17.1/18.4 GB 时 voxtmini-llm 3.4 GB 的占用只有 80–97 MB 在物理内存里，其余已被换出 | 每次用完，最多约 7–8 GB（语音 3.7 + 语言 3.4–4.3）再占 5–10 分钟，把用户别的 app 挤进 swap；而 swap 很满时模型自己也被换出，“留着”并不会让下一次更快（eval.md 里首 token 从 0.2 s 变 2 s） | “自动”改为看内存：`macx.SysMemory` 已经能读压力等级和 swap（`internal/macx/sysmem_darwin.go`）。只在有模型加载时每 30 s 读一次；压力为 warn/critical 或 swap 用了 ≥ 80% 时，空闲 60 s 就卸载。用户选了固定时长则不变 | M / 中：下一次口述要重新加载（语音约 1–2 s；语言模型的预热在说话时进行，eval 里已验证能藏住） |
| 2 | **语言模型的 MLX 缓存没有上限** | `llm_link.go:49` 的 `llmrt.Config` 没有设 `CacheLimitMB`（协议早已支持：`internal/llmrt/llmrt.go:90`，Swift 端 `MLXEngine.swift:45`）。llm-models.md：llm.balanced 占用 3.4 GB、峰值 4.3 GB | 和 #20 修过的语音服务器是同一类问题：MLX 把释放的缓冲区留着，上限接近整机可用内存；长的改写 / 回答之后进程占用会比权重多出 1 GB 甚至更多，而且一直到空闲卸载 | 照语音模型的做法传 `CacheLimitMB: 512`（或 256，Mac 上量过再定） | S / 低：语音服务器的经验是 512 MB 不降速 |
| 3 | **Whisper 的服务器没有缓存上限** | `internal/omni/models.go:104`：只有 qwen3_asr_server 认 `--cache-limit-mb` | Whisper 是“仅文件”，一次长录音的 Final 会像 #20 之前的 Qwen3-ASR 那样涨到数 GB | 给 whisper_server 加同样的参数（native 代码），或在每次请求后 `clear_cache` | M / 低；要在 Mac 上重建 omni 运行时（MEMORY 里 Whisper 的 Mac 测试本来就待做） |
| 4 | **主窗口关到菜单栏后只是隐藏，不释放** | `tray.go:202–206`：`OnClose` 里 `e.PreventDefault(); a.win.Hide()`。MyGo 在空闲时会释放画好的帧（`third_party/mygo/ui/window.go:339`），但窗口的控件树、字形缓存、图标位图（见 #7）、GPU 表面都还在 | 未量化；大多数时间窗口是关着的，所以这块内存一直占着。要 Mac 上量 VoxtMini 进程窗口开 / 关时的占用才知道值不值得做 | 先量；若关窗后仍多占 > 50 MB，再做“关窗即销毁、点开再建”（MyGo 支持多窗口，状态都在 `app` 里） | 量：S；做：L / 中（窗口生命周期、托盘、Dock、引导都牵涉） |
| 5 | **辅助进程空闲时每秒醒一次** | `helper/Sources/VoxtMiniHelper/Hotkey.swift:207` 每 1 s 的 warm-up（容差 0.5 s）；`:233–250` 每 2 s 的 `AXIsProcessTrusted()` | 这是 11-12 点击卡死事件后的有意设计（让热键路径的内存页不被换出），值得保留；但 1 Hz + 0.5 Hz 的唤醒在电池上一直存在 | 先在 Mac 上用 `powermetrics` / 活动监视器的“空闲唤醒”量；若明显，把两个计时器合成一个 2 s、容差 1 s 的计时器（warm-up 2 s 一次也足以保住页面），不改热键逻辑 | S / 中：碰的是事件 tap 线程，必须跑 `HotkeyAllocationTests` 和 coexist 脚本 |
| 6 | **每一行日志都让主窗口整页重建** | `shell.go:134`：`LogAppended` → `a.post` → `Window.Update`（每批一次重建），不管当前是不是日志页 | 窗口开着时，一次会话的几十行日志（omni 的 stderr 每行都按 warning 进日志：`internal/omni/process.go:106`）各触发一次整页重建；窗口隐藏时 MyGo 不画，代价小 | 只在日志页显示时才 post；其他时候只置脏位 | S / 低 |
| 7 | **应用图标位图只增不减** | `appicons.go:31` `iconBmps map[string]*ui.Bitmap`，从不清理 | 打开过“添加 app”对话框（列出全部已装 app）后，几百个 64 px 图标（@2x 约 64 KB 一个）一直留在内存：约 10–15 MB | 离开应用增强页时清掉 `iconBmps`（磁盘缓存还在，再显示很快） | S / 低 |
| 8 | **模型页内存测量在窗口隐藏后仍每 3 s 排一次计时器** | `models_memory.go:19,105–114`：隐藏时不测，但 `a.later(memPoll, a.memTick)` 继续 | 每 3 s 一次空唤醒，直到切页 | 窗口不可见时停掉，`modelsShown` / 窗口重新显示时再开 | S / 低 |
| 9 | **一次长录音的临时内存** | Linux 实测 5 分钟录音的 Final 路径（高通 + 电平 + VAD 压缩 + WAV + multipart）：68 ms，**一次分配 49 MB**（9.6 MB 音频复制约 5 次）；`internal/omni/final.go:50`、`internal/asr/audio.go:31`、`internal/dsp/level.go:160,250` | CPU 可忽略；内存是瞬时的，但 Go 不会马上还给系统（GOGC 默认，下一轮 GC 和后台回收之后才还），长录音后 VoxtMini 进程会多占几十 MB 一段时间 | multipart 用 `io.MultiReader`（头 + PCM）流式发送，不再拼 `bytes.Buffer`；dsp 原地处理；录音超过 60 s 的会话结束后 `debug.FreeOSMemory()` | S–M / 低 |
| 10 | **助手 / LLM 的 stderr 日志运行中不轮转** | `internal/helper/supervise_proc.go:145`、`internal/llmrt/proc.go:308`：只在进程启动时检查 > 10 MB；app 的 `voxtmini.log` 5 MB × 3 有上限（`internal/logs/logs.go:32`） | 一般很小（helper 只有 19 处日志），但辅助进程可连续运行数周，出错循环时无上限 | 写入时按大小截断/轮转（同 `internal/logs` 的做法），或启动时以外每次 restart 也检查 | S / 低 |
| 11 | **数据库迁移前的备份永不删除；删历史后文件不缩小** | `internal/store/sqlitestore/sqlitestore.go:155` 每升一次 schema 版本留一个 `voxtmini.db.bak-<v>`（已有 7 个迁移）；Linux 实测 5000 条历史 = 9.1 MB 库 + 5.1 MB WAL，清理掉 3561 条后库仍 9.1 MB（没有 VACUUM / auto_vacuum） | 每条约 1.8 KB，几万条也只有几十 MB；备份是整库复制，多次升级后翻倍 | 迁移成功后只保留最近一个 `.bak`；清理删除超过 1000 条后做一次 `VACUUM`（插件拒绝的只是 `VACUUM INTO`，需在 Linux 上验证普通 VACUUM 可用），或 `PRAGMA incremental_vacuum` | S / 低 |
| 12 | **历史清理只在启动和改设置时跑** | `history_settings_ops.go:48–51,60–61` | app 连续运行数周时，“保留 90 天 / 每类 N 条”不会生效直到重启 | 挂到已有的每小时循环（`history_retry.go:183–197`，`expireRecordings`）里，每天跑一次 | S / 低 |
| 13 | **保留录音时是未压缩 WAV** | `internal/session/run.go:377`：每分钟 1.9 MB | 默认关（`settings.go:382` 只开了 KeepHistory），开了之后重度使用 90 天可到数 GB；失败录音保留供重试，有到期清理 | 打开时编码为 AAC（辅助进程的 AVAudioConverter，约 1/10）或纯 Go FLAC（约 1/2）；或在设置 → 存储里直接显示录音占用 | M / 中（协议改动、回放路径）；优先级低 |
| 14 | **词典替换是“文本长度 × 别名数”** | `internal/dict/alias.go:68–92`；实测 500 个词条（1500 个模式）、约 1 KB 文本：**9.3 ms**，词条和文本都大时线性放大 | 每次会话一次，几千词条的长口述也就几十 ms；不是现在的问题 | 词条超过 ~1000 时按首词建索引（map），等有用户真有这么大词典再做 | S / 低；优先级最低 |

### 已经做得好的（不用动）

- **启动时不加载任何模型**：Omni 和 voxtmini-llm 都在第一次使用时启动（`models_omni.go:105` 的 Warm 只在打开模型时），语言模型在用户说话时预热。
- **会话没有泄漏**：Linux 上用假服务各跑 31 次会话（流式 + LLM、文件 + LLM、LLM 超时），之后 goroutine 数不变（3 → 3）。
- **语音服务器的 MLX 缓存已限 512 MB**（#20，eval.md“Speech server memory”：10.3 GB → 3.9 GB）。
- **提示缓存有界**：6 个快照 / 512 MB，随模型卸载（`MLXEngine.swift:10–14`）。
- **网络很少**：公告 15 s 后一次、之后每 6 小时（`internal/announce/announce.go:104`）；更新检查每天一次（MyGo updater 默认）；家族文件靠 Darwin 通知而不是轮询；没有其他后台请求。
- **空闲计时器少**：Go 侧只有每分钟检查一次 app 包是否被更新（`updates.go:116`，读一个 Info.plist）和每小时的录音到期清理。动画（悬浮条 20 fps、计时 4 Hz）只在录音 / 处理中；主页、历史页每分钟重画一次相对时间，窗口隐藏时 MyGo 不画（`third_party/mygo/internal/darwin/surface.go:221`）。
- **事件 tap 设计稳妥**：鼠标只用 listen-only tap、只听按下；中键 / 侧键的主动 tap 只在绑定了鼠标键时存在（`Hotkey.swift` 开头的说明）。
- **SQLite 查询快**：5000 条时，最近 500 条 4.3 ms，全文搜索 4–14 ms，托盘的最近 20 条 0.2 ms。
- **繁转简**：137 MB/s，无需转换的文本零分配。

### 需要在 Mac 上量的（请 main 问用户；每项都很轻，可以一次做完）

都只读、无 GUI 启动，按规则先看 `sysctl vm.swapusage`。

1. **空闲占用**（VoxtMini 已在运行，窗口关着、没用过模型 ≥ 15 分钟）：
   `top -l 2 -s 5 -stats pid,command,cpu,mem,idlew,power -pid <VoxtMini> -pid <voxtmini-helper>`，以及 `footprint <pid>`（或 `vmmap -summary`）。看 CPU、空闲唤醒、电量影响、内存占用。
2. **窗口开 / 关对比**：同上，窗口打开停在主页 1 分钟后再量一次（回答 #4 值不值得做）。
3. **辅助进程唤醒**：`sudo powermetrics --samplers tasks --show-process-wakeups -i 5000 -n 3 | grep -A2 -i voxt`（回答 #5）。
4. **用完模型后的占用**：一次约 30 s 的转录（开清理），结束后马上和 4 分钟后各 `footprint` 一次 `qwen3_asr_server` 和 `voxtmini-llm`（回答 #1、#2 的实际数字）。
5. **磁盘**：`du -sh ~/Library/Logs/VoxtNew/* ~/Library/Application\ Support/VoxtNew/* ~/Library/Caches/VoxtNew`；`ls ~/Library/Application\ Support/VoxtNew/*.bak-*`。
6. **启动时间**：`log show --last 5m --predicate 'process == "VoxtMini"'` 不够准时，看 `voxtmini.log` 里启动第一行到“helper ready”的时间差即可（下次重启 app 时顺带看）。

### 需要决定的

- #1 的阈值（压力 warn 或 swap ≥ 80% → 60 s 卸载）是否接受？会让下一次口述在 swap 很满时多 1–2 s 加载；但那种情况下留着的模型本来就已被换出。
- #13（录音压缩）要不要排进来，还是只在设置 → 存储显示录音占用。

---

## Part 2: Work plan (English)

Each package is independent: the files listed do not overlap between packages. None touches `internal/compose`, `feature_*.go` or `internal/settings` prompts (#27's area); the one that needs a settings change (WP-A) is marked "after #27" for that part only.

Every package: `CGO_ENABLED=0 go test -p 8 ./...` once at the end (check `uptime` first), `go tool mygo vet .`, and `TestKeyParity` when a string changes. Mac checks go through main.

### WP-A — Memory-aware unloading of idle models (finding 1) · M · after #27 merges if it adds a setting

- Files: `models_keep.go`, `family_runtime.go` (only `unloadIdleModel`'s caller side, if reused), new `models_pressure.go` (+ test); `internal/macx/sysmem*.go` (read-only use); `internal/omni/runtime.go`, `internal/llmrt/runtime.go` only if an "unload now if idle ≥ d" call is missing (both already have `Unload`/`UnloadIdle` for TopicMemory).
- Design: while at least one model is loaded (`a.loadedModels()` non-empty) a 30 s ticker reads `macx.SysMemory()`; when the level is warn/critical or swap used ≥ 80% of total, models idle ≥ 60 s are unloaded through the existing `a.unloadIdleModel` path. Only when `KeepLoaded` is Automatic. The ticker stops when nothing is loaded (no idle cost). Log one line per unload with the numbers. behavior.md row (Voxt keeps its own timers).
- Test: pure decision function `shouldUnload(sys, idleFor, keep) bool` table-tested; app test with a fake `sysMemory` seam and fake runtimes (as `models_memory_test.go` does) that a pressured state unloads and a calm one does not; the ticker does not run with nothing loaded.
- Mac: measurement 4 before/after.

### WP-B — Bound the language model's MLX cache (finding 2) · S

- Files: `llm_link.go` (set `CacheLimitMB`), a constant beside it; `docs/llm-models.md` (a line), `docs/behavior.md` row.
- Test: `llm_link` unit test that the config carries the limit (fake llmrt records `LoadParams`; `internal/llmrt/llmrttest` already decodes them). Mac: `TestLLMMeasure` footprint/peak with 512 vs none, `TestEvalLLM` quick for speed parity.

### WP-C — Whisper server cache limit (finding 3) · M · Mac rebuild of omni

- Files: `sglang_omni_mlx/native/src/whisper*` server (add `--cache-limit-mb`, as qwen3_asr_server.cpp:135–175), `internal/omni/models.go` (CacheMB for Whisper specs), `internal/omni` test of the args.
- Test: Go test on args and the older-runtime probe (`TestOlderServer` exists); Mac: rebuild `~/voxtmini-runtime/omni` (one heavy job, ask main), long-clip footprint. Can merge with the pending Whisper Mac test.

### WP-D — UI idle costs (findings 6, 7, 8) · S

- Files: `shell.go` (LogAppended: post only when the logs page shows, else set a dirty flag read on `pageShown`), `models_memory.go` (stop `memTick` while `!a.win.IsVisible()`, restart from window show), `appicons.go` (drop `iconBmps` on leaving App enhancement; `pageShown` hook in `pages.go` — one line), tests in `shell_logs_*_test.go`, `models_memory_test.go`, `appicons_test.go`.
- Test: Tester counts of builds while logs arrive on another page (expect 0 posts), `laterHook` shows no re-arm while hidden, icons map empty after navigation and re-filled from the disk cache.

### WP-E — Helper wakeups (finding 5) · S · measure first

- Gate: only if Mac measurement 3 shows the helper among top idle wakers.
- Files: `helper/Sources/VoxtMiniHelper/Hotkey.swift` (one 2 s timer, tolerance 1 s, running warm-up then the trust check on the tap thread's run loop, the trust check moved off it as now via `DispatchQueue.global`), comment block at the top.
- Test: `swift test` (HotkeyAllocationTests, HotkeyTapKitTests), `scripts/mac-hotkey-coexist.sh`; Mac only — one build, ask main.

### WP-F — Disk hygiene (findings 10, 11, 12) · S

- Files: `internal/store/sqlitestore/sqlitestore.go` (keep only the newest `.bak-*` after a successful migration), `internal/store/sqlitestore/history_retention.go` (VACUUM after a prune that removed ≥ 1000 rows — check the plugin allows plain VACUUM on Linux first; else `auto_vacuum=INCREMENTAL` needs a one-time VACUUM, skip), `history_retry.go` (daily prune in the existing hourly loop, posting `pruneHistory` to the main thread), `internal/helper/supervise_proc.go` + `internal/llmrt/proc.go` (shared size-capped writer: rotate at 10 MB while running; a small type in each package or `internal/logs` if `imports.txt` allows — `internal/logs` is a leaf, so add the import lines).
- Test: sqlitestore tests with a temp dir (bak files, file size after prune+vacuum), a fake clock for the daily prune, a writer test that rotates at the cap.

### WP-G — Session transient memory (finding 9) · S–M · low priority

- Files: `internal/omni/final.go` (stream the multipart body: `io.Pipe` or `io.MultiReader` of header + WAV header + PCM + trailer, `ContentLength` set), `internal/asr/audio.go` (`WAVHeader(n, rate) []byte`), `internal/dsp/level.go`/`highpass.go` (process into one buffer), `session_link.go` (`debug.FreeOSMemory` after a session with > 60 s of audio, on a goroutine).
- Test: the existing omni fake-server tests (body bytes identical), dsp golden tests (bit-identical output), the 5-minute benchmark from this audit (allocations 49 MB → target < 25 MB): add it as `BenchmarkFinal5Min` in `internal/dsp`.

### WP-H — Measure the hidden window, then decide (finding 4) · S to measure, L to fix

- Measurement only (Mac 1–2). If closing the window saves > 50 MB, a separate thread designs "destroy on close, rebuild on show" in `tray.go`, `shell.go`, `dock.go`; not before.

### Later / only if asked

- Finding 13 (compressed recordings): helper protocol change (`capture` or a new `audio.encode`), all five protocol places, History playback. Or just show the recordings' size in Settings → Storage (it already measures folders).
- Finding 14 (dictionary index): `internal/dict/alias.go`, when someone has > 1000 terms.
- Announcements: send `If-None-Match` (`internal/announce`) — bytes are tiny, cosmetic.

### Suggested order

WP-B and WP-D first (S, low risk, no Mac build), WP-F in parallel; WP-A after the Mac numbers (measurement 4) confirm the size; WP-E and WP-H only on measurement; WP-C with the Whisper Mac test; WP-G whenever a thread is free.

## Appendix: Linux measurements (go1.27.1, CGO_ENABLED=0, 192-core host, load ~24)

| What | Result |
|---|---|
| `dict.ReplaceChanges`, 500 terms (1500 patterns), ~1 KB text | 9.25 ms, 296 KB, 2026 allocs |
| `dict.Hints`, 500 terms | 0.12 ms |
| `compose.BuildParts`, 300-term glossary, ~1.2 KB mixed text | 0.32 ms, 212 KB |
| 5-min Final path (`dsp.ProcessFile` + `vad.Compact` + `asr.WAV` + multipart) | 68.8 ms, **49.3 MB allocated**, 133 allocs |
| `vad` Analyze+Compact 10 min (existing benchmark) | 8.0 ms, 11.4 MB |
| `hans.Simplify` (existing) | 137 MB/s |
| Session manager, 31 sessions × 3 kinds with fakes | goroutines 3 → 3 (no leak) |
| SQLite, 5000 history entries | put 130 µs each; DB 9.1 MB + WAL 5.1 MB; List 500 4.3 ms, List 20 0.23 ms, search 4.3–14 ms; prune 3561 rows: DB size unchanged |

The scratch benchmarks were removed after measuring (zz_perf_test.go in dict, compose, dsp, session, sqlitestore); WP-G re-adds the Final one as a kept benchmark.
