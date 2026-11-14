# Audit: package split, dead code, big files, test speed (thread #36, 2026-11-14)

Base: main 9943ec6. Linux host (192 cores, load 3–8 during the runs), Go 1.27.1, `CGO_ENABLED=0`, `-p 8`. Nothing in the product code was changed; the measuring tools live in `/tmp/c36` (not committed). Part 1 is the report for the user (Chinese), Part 2 the work plan (English), Part 3 the raw evidence.

---

## Part 1 · 报告（给用户）

### 一句话结论

慢的不是编译，也不是“包太大”本身，而是**根包（package main）里约 21 个端到端测试按真实时间等待**（假辅助进程按脚本 sleep 300–1500 ms 再发快捷键），而且它们串行跑。全量 `go test ./...` 81 秒，其中根包 69 秒；**不改一行产品代码，把根包测试拆成 8 个进程并行跑，69 秒 → 8.9 秒**（已实测三次，全部通过）。Go 自带的测试缓存已经做到“没改的包不重跑”（全缓存时 0.9 秒），所以真正要做的是：让根包自己的测试变快，并把根包里能独立测试的逻辑搬到 `internal/`，让它们的测试被缓存。

### 测量结果

| 项目 | 数值 |
|---|---|
| 全量 `go test -count=1 -p 8 ./...` | 81 秒墙钟（用户态 95 秒：几乎全耗在根包的等待上） |
| 根包 `.` | 69 秒，280 个测试；≥1 秒的 21 个合计 57 秒，<0.5 秒的 254 个合计 9 秒 |
| 其他包最慢 | session 16.4 s，omni 10.0 s，llmrt 7.2 s，helper 4.9 s，其余都 <2 s |
| 全缓存重跑 `go test ./...` | 0.9 秒（Go 的缓存已经只重跑改过的包） |
| 根包测试二进制编译+链接 | 3.5 秒；`go build` 1.9 秒；`go vet .` 1.3 秒 |
| 根包测试按耗时分 8 个进程 | **8.9 秒**（3 次全过）；按名字轮转分 8 个 15 秒，12 个 12.8 秒 |
| `t.Parallel()` | 全仓库 0 处 |
| 快照测试 | 默认已跳过（要 `VOXTMINI_SNAPSHOTS`），不是慢的原因 |

最慢的测试：`TestOverlayFlows` 7.2 s、`TestOverlayFlowsTrace` 7.2 s、`TestSessionAppEnhancement` 6.1 s、`TestSessionMutesOutput` 3.8 s、`TestFamilyRuntimeOmni` 3.2 s，以及十几个 1.5–2.7 s 的会话端到端测试。原因都一样：`helpertest` 的事件脚本 `{"afterMs": 400}` 是真的 `time.Sleep`（internal/helper/helpertest/fake_text.go:99），录音按真实帧率 100 ms 一帧推送（fake_capture.go:157）。internal/session 的 `TestCleanupDeadlines`（6.6 s）、`TestWriteDeadlines`（3.2 s）是进程内的真实计时器，适合用 `testing/synctest`（仓库已在 snapshot_env_test.go 用过）。

### 根包现状

根包 119 个源文件 23,767 行 + 94 个测试文件 19,144 行。按函数分类（函数体行数，合计 16.3k）：

- 视图（用到 `ui.`/`mygo.`）：约 9.2k 行，必须留在根包（MyGo 只构建根包；视图要 `*app` 和窗口线程）。
- “控制器”（`*app` 的方法，但不碰 UI）：397 个函数、5.4k 行，例如 `askFollowUp`、`hotkeyFired`、`cardNow`、`readinessList`、`listModels`、`importFile`。它们读写 `app` 的字段，要搬走得先定义窄接口，按区域逐个来。
- 纯函数（不碰 `app` 也不碰 UI）：177 个、1.7k 行，大多是“把领域值变成文字”，按规则本来就该在 `internal/uitext`。

我不建议一次把根包拆成很多包：`app` 是中心，几乎每个文件都挂在它上面，大拆等于全量改写，会和所有在跑的线程冲突。建议**分批小步**：先把测试变快（零风险，立刻见效），再把纯逻辑和文字搬进 `internal/`（带着它们的单元测试），最后按区域把控制器做成小的内部包。UI 组件库（ui_*.go，1.3k 行，几乎不依赖 `app`）可以独立成 `internal/uikit`，但要改架构规则（目前只允许 `icons` 用 `mygo/ui`），而且会碰每个视图文件：**需要你决定**，我倾向于暂不做。

### 死代码 / 遗留物

**可以直接删（有证据，无人引用，测试也不用）**：

- UI 组件里没用的 9 个：`labelOrIcon`、`dropText`、`iconToggle`、`playToggle`（ui_buttons.go）、`popoverWithFooter`（ui_forms.go）、`filterBar`（ui_layout.go）、`panelFits`、`panelHeader`（ui_panels.go），以及应用里没人用、只有测试在用的 `numberInput`（ui_forms.go:37，连同只测它的测试一起删）。
- `a.showPage`（shell.go:194，架构文档的表里还写着它）、`a.transcribeBlocker`（session_control.go:29）、`dictionary_term.go:31` 的 `title` 字段、`sqlitestore.ms`（sqlitestore.go:299）、doubao 的 `codeSuccess`（payload.go:309）、`hotkey.ProblemsOf`（check.go:148）、`llm.Skills`（skills.go:18）、`llm.SecretsFunc`/`llm.EnvSecrets`（presets.go:234/250，读 `AURALINK_*` 环境变量，Auralink 遗留）。
- `secrets.Mask`（secrets.go:128）：与 `remote.Mask` 一字不差，应用只用后者。
- 4 个图标：`ArrowToDownLeft`、`Cloud`、`Export`、`Eye`（names.txt 里注释说有用，实际没有任何引用）。

**需要你决定**：

1. `llm.Meter`（usage.go，约 140 行，AudioSmith 的用量统计）：生产代码从不设置它，只有测试用。规则里“主页不做统计”，建议删。
2. 约 45 个“只有测试在用”的导出函数（`dict.Replace`、`omni.Models`、`models.Hub`、`outmute.Controller.Wait/Status`、`i18n.Date`、`lang.All`、`vad.Detector.Segments`……完整列表见第 3 部分）：删、改成测试文件里的辅助函数，或保留。
3. **发布地址仍是 voxtnew**：release/mygo.release.json 的更新地址 `https://actnow.dev/updates/voxtnew`、release.sh 的 `--app voxtnew`。按记忆，网站上 `/updates/voxtnew` 已经 404、`--app voxtnew` 会被拒。这不是死代码而是**下一次发布会失败**；bundle id、数据目录、URL scheme 的 voxtnew 是有意保留的，不动。voxtmini 产品在网站上暂缓，所以要你定发布走哪个 slug。
4. `scripts/release-github.sh`：全仓库没有任何地方提到它（从 Auralink 带来的），删还是写进 docs/release.md？
5. 文档：`docs/voxt-gap-analysis.md`（没有任何文档链接它，第 3–4 节已过时，你的决定已经写进 RULES）和 `docs/voxt-migration.md`（Voxt 用户公告草稿）：归档到 `docs/archive/` 还是删除？
6. `settings.Audio.SoundPreset`：只为迁移旧设置文件保留的字段；保留（安全）还是定个期限删除。
7. `droppedPins`（internal/models/models_test.go:99）：等 actnowkit 下一版去掉那 6 行后删（记忆里已有），不用现在做。

已经有测试兜底的：i18n 未使用的翻译（`TestKeyParity`）、协议夹具（helper/Fixtures、llm/Fixtures 都被测试引用）、testdata/eval（全部在 clips.json 里）。

### 重复逻辑

- **子进程管理**：internal/helper（supervise_io.go、supervise_proc.go）和 internal/llmrt（proc.go）各有一份几乎一样的 `readLine`、`lineTail`、`stderrLevel`、`openLog`（逐行 diff，只差一个常量名）。抽成一个小包 `internal/procio`。
- **协议信封**：`helper/proto` 和 `llmrt/wire` 的 `Decode`/`Encode`/`NewRequest`/`NewResult`/`NewError`/`ParamsFor`/`ResultFor`/`IsJob` 结构相同。两边各自对应一套 Swift 类型和夹具，合并收益小、牵连大：建议保持现状（需要你点头）。
- `isCJK` 三份（compose、eval、asr/gemini，前两份一模一样）、`itoa` 三份、`Mask` 两份、asr 各家的 `mustJSON`/`clipRunes`/`vocabulary`、三个假进程包的 `Env`/`Enabled`/`Opt`：小件，可顺手合并。

### 大文件、长函数

没有文件超过 600 行硬上限（allowlist 是空的）。超过 400 行建议线的源文件：根包 5 个（remote_dialog.go 457、shell_settings.go 411、feature_keys.go 411、settings_storage.go 409、shell_commands.go 406），internal 12 个（compose.go 553、settings.go 550、helpertest/fake.go 523、helpertest/output.go 520、outmute.go 477、session.go 460、omni/live.go 456、datadir.go 430、session/run_listen.go 426、hotkey.go 409、session/run.go 406）。超过 80 行的函数 31 个，最长的是 `session.(*run).listen` **263 行**，其次 helpertest 的 `serve` 152、`audioSettings` 135、`overlayCheck` 134、`compose.Build` 134、`historyDetail` 117。

### 建议的顺序

1. **测试提速**（马上可做，互不冲突）：根包分片脚本 → 假辅助进程改成“等测试放行”而不是按时间 sleep → session 里的计时测试用 synctest → 带子进程的 internal 包加 `t.Parallel`。预计全量从 81 秒到 15 秒以内，改根包后的重测从 69 秒到 10 秒以内。
2. **删死代码**（一个小线程，半天）。
3. **抽重复**（procio 等）。
4. **根包瘦身**：文字函数进 uitext；日志选择、深链接路由、就绪列表、诊断包、模型列表等纯逻辑各自成包，带着单元测试走。
5. **拆长函数/大文件**，纯移动，用 declhash 和快照对比证明。

涉及 compose、settings、feature_*.go、models_page/keep、shell_settings.go 的都排在 #27 合并之后。

### 需要在 Mac 上测（请主线程问用户）

1. mac-air 上 `CGO_ENABLED=0 go test -p 4 ./...` 的总时间和根包时间（darwin 构建标签不同，Mac 上更慢的话分片更有价值）。
2. 同一台机器上根包分 4 个进程跑一次（`scripts/test-main.sh -n 4`，做完工作包 T1 以后），看内存和 swap 是否能承受。
3. `cd helper && swift build` / `swift test`、`scripts/build-llm.sh --test` 的冷/热时间（判断 Swift 那边要不要也拆 target）。
4. `go tool mygo build` 的时间（链接、签名占多少）。

---

## Part 2 · Work plan (English)

Work packages (WP) are independent unless "Depends" says so; each names the files it touches. "After #27" = do not start before thread #27 (editable prompts) is merged: it changes `internal/compose`, `internal/settings`, `internal/llmrt/{backend,runtime,tuning}.go`, `internal/session/text.go`, `feature_{models,page,prompts}.go`, `appenhance_group.go`, `models_{keep,page}.go`, `models_llm_params.go`, `library.go`, `llm_link.go`, `shell_settings.go`, `write_pages_test.go` and `internal/i18n/locales/zh-Hans.json`. Sibling audits #37 (performance), #38 (security), #39 (docs/README) may touch docs: WP-D2's doc moves wait for #39.

Every move-only WP proves itself with `go run ./internal/tools/declhash . ./internal/...` (equal sets, `-nocomments` where file names in comments change) and, for view code, `scripts/snapshots.sh check`. Every WP keeps `CGO_ENABLED=0 go test ./...`, `go tool mygo vet .` and archtest green, and adds `imports.txt` lines for new internal imports.

### Track T — test speed (highest value; no product code)

**WP-T1 · Sharded runner for the root package + `test-changed.sh`** — S, risk low.
- Finding: root package 69.1 s of the 81.2 s suite; 280 top-level tests, none parallel; build is 3.5 s. Process shards are isolated (package-level seams such as `newKeyring`, `openBrowser`, `i18n` locale stay per process), so no test changes are needed.
- Evidence: 8 shards balanced by the previous run's durations: 8.8 / 8.9 / 8.9 s, all pass; round-robin by name: 15.1 s (8), 12.8 s (12); 4 balanced shards 17.5 s. Floor = the longest single test (`TestOverlayFlows` 7.2 s; its 9 subtests can be sharded as `-test.run '^TestOverlayFlows$/^name$'`).
- Fix: `scripts/test-main.sh [-n N] [-race] [-run RE]`: `go test -c -o $TMP/main.test .`, `-test.list '.*'`, balance by `build/test-times.json` (written from `-json` output of the last full run; fall back to round-robin), run N processes from the package directory, merge output, exit non-zero on any failure. `scripts/test-changed.sh [BASE]`: plain `go test -p 8 $(affected)` where affected = packages whose `go list -test -deps` include a package with a changed `.go`/embedded file (testdata, templates, locales, `helper/Fixtures` → `internal/helper/proto`, `llm/Fixtures` → `internal/llmrt/wire`); the root package goes through `test-main.sh`. Note: Go's test cache already skips unchanged packages (a fully cached `go test ./...` takes 0.9 s), so `test-changed.sh` is mainly about not paying `-count=1` habits and routing the root package to the sharded runner; document "do not use `-count=1` by default".
- Files: `scripts/test-main.sh`, `scripts/test-changed.sh` (new), `docs/architecture.md` §6, `AGENTS.md` Commands, optionally the CI workflow at the repository root.
- Test: run both on a clean tree and with one injected failing test (non-zero exit, failure printed); compare the set of test names run against `-test.list`.

**WP-T2 · Event-driven fake helper scripts instead of real-time sleeps** — M, risk medium (test-only).
- Finding: e2e tests drive the fake helper with `helpertest.Script{AfterMS}` lines that sleep sequentially (internal/helper/helpertest/fake_text.go:99): e.g. `TestSessionEndToEnd` sleeps 0+400+1500 ms (session_e2e_test.go:173–177) and takes 1.98 s; 15 test files use `script(t, …)` / `hotkeyLine` / `hotkeyIn` (≈45 call sites). Extra fixed sleeps: overlay_flows_test.go:212, rewrite_e2e_test.go:48, session_e2e_rewrite_test.go:41,86 (400 ms each), mute_e2e_test.go:112 (300 ms).
- Fix: add a script gate to the fake: a line `{"waitFor":"stop"}` blocks until the test creates `<dir>/stop` (or writes a line to a FIFO given by `Opt("control", path)`); a test helper `a.release("stop")` after `a.pump(... state == Recording)`. Keep `AfterMS` for the tests that test timing (hold vs tap, double tap, paste checks). Recording frames: keep `frame-ms` but default it lower where the duration does not matter.
- Files: `internal/helper/helpertest/fake_text.go`, `fake.go` (option parsing), `session_e2e*_test.go`, `overlay_flows_test.go`, `overlay_sessions_test.go`, `mute_e2e_test.go`, `appenhance_e2e_test.go`, `llm_e2e_test.go`, `rewrite_e2e_test.go`, `family_runtime_test.go`, `models_omni_test.go`, `apple_test.go`. Not the helper protocol (no wire change).
- Expected: the 21 tests ≥1 s (57 s) to well under 1 s each (estimate: root package 69 s → ~15 s serial, ~4 s sharded).
- Test: each converted test still checks the same states; run converted tests 20× (`-count=20`) and under `-race` once to catch new flakiness.
- Depends: none; conflicts only with threads editing those test files.

**WP-T3 · `testing/synctest` for in-process timer tests** — S/M, risk low.
- Finding: internal/session `TestCleanupDeadlines` 6.6 s, `TestWriteDeadlines` 3.2 s, `TestWarmLLM` 2.1 s; internal/llm `TestProgressDeadlines` 1.3 s: in-process fakes (`pacedLLM` scripts with real `time` pauses, deadlines_test.go:104–150).
- Fix: wrap these tests (or their subtests) in `synctest.Test`; the fakes already use `time` only, no sockets. Where a fake uses a real network or process, leave it.
- Files: `internal/session/deadlines_test.go`, `internal/session/write_test.go` (the deadlines case), the warm-up test file, `internal/llm/*_test.go` (progress deadlines). Does not touch `internal/session/text.go` (#27).
- Expected: session 16.4 s → ~5 s; llm 1.7 s → ~0.4 s.
- Test: same assertions; `-count=10` and `-race`.

**WP-T4 · `t.Parallel` in the subprocess-fake packages** — S, risk low/medium.
- Finding: omni 10.0 s (≈20 tests of 0.4–1.1 s against the fake server), llmrt 7.2 s, helper 4.9 s, outmute 1.2 s; each test waits on its own child process; no package-level state is shared once each test has its own temp dir and port.
- Fix: audit each package for package-level variables the tests assign; add `t.Parallel()` to the tests that do not; `TestMain` unchanged.
- Files: `internal/omni/*_test.go`, `internal/llmrt/*_test.go`, `internal/helper/*_test.go`, `internal/outmute/*_test.go`.
- Test: `go test -count=5 -race` on each package; the wall time per package.

Expected suite after T1–T4: about 15 s wall on this host (from 81 s); a change in the root package retests in about 5–9 s.

### Track D — dead code and leftovers

**WP-D1 · Delete what nothing uses (safe)** — S, risk low.
- Evidence: `staticcheck -checks U1000 ./...` and `deadcode -test ./...` (both include tests):
  - `ui_buttons.go:98 labelOrIcon`, `:121 dropText`, `:188 iconToggle`, `:259 playToggle`; `ui_forms.go:119 popoverWithFooter`; `ui_layout.go:142 filterBar`; `ui_panels.go:28 (*app).panelFits`, `:59 panelHeader`.
  - `shell.go:194 (*app).showPage` (also remove it from the seams table, docs/architecture.md:274); `session_control.go:29 (*app).transcribeBlocker`; `dictionary_term.go:31` field `title`.
  - `internal/store/sqlitestore/sqlitestore.go:299 ms`; `internal/asr/doubao/payload.go:309 codeSuccess`; `internal/hotkey/check.go:148 ProblemsOf`; `internal/llm/skills.go:18 Skills`; `internal/llm/presets.go:234 SecretsFunc`, `:250 EnvSecrets` (reads `AURALINK_*`).
  - `internal/secrets/secrets.go:128 Mask` (identical to `internal/remote/keys.go:114 Mask`, the one the app uses; move its test if any).
  - Icons with no reference: `ArrowToDownLeft`, `Cloud`, `Export`, `Eye` (`internal/icons/names.txt` lines 76, 13, 82, 64), then `go generate ./internal/icons`.
  - `ui_forms.go:37 numberInput` is reached only by tests on darwin (`deadcode` GOOS=darwin without tests): delete with its test if the test only covers it.
- Files: the ones listed, `internal/icons/{names.txt,solar_gen.go}`, docs/architecture.md (one table cell). No #27 overlap.
- Test: `staticcheck -checks U1000 ./...` and `deadcode -test ./...` print nothing for these; `go test ./...`; `scripts/snapshots.sh check` (no picture changes).

**WP-D2 · Decisions, then delete or keep** — S each, after the user decides.
1. `internal/llm/usage.go` `Meter` (+ `Client.Meter`, client.go:68,162–180): set only in internal/llm/llm_test.go:78. Delete (no stats on Home, RULES) or keep.
2. Exported API reached only by tests (deadcode, GOOS=darwin, without tests; the list in Part 3 §3.3). Keep the deliberate test seams (`permission.Fake`, `settings.NewMemory`, `actnow.FakeKeychain`, proto/wire `ParamsFor`/`ResultFor`/`New*` used by the fixture tests); for the others (`dict.Replace`, `omni.Models`, `omni.ASRModels`, `llmrt.Profiles`, `models.Installed`, `models.Hub`, `notify.Center.MarkRead`, `outmute.Controller.Wait/Status`, `i18n.Date`, `lang.All`, `secrets.ASR/LLM`, `store Path`, `vad Gate.Heard/Detector.Heard/Segments`, `dsp HighPass.Response/Leveler.GainDB`, `hotkey.Action.String`, `llm.Array/Number/Integer/Boolean/Enum`, `llm.HeaderFunc`, `shell_notices.go:45 (*app).notify`): move into `export_test.go`-style helpers or delete.
3. The update feed and publish slug: `release/mygo.release.json:4` (`https://actnow.dev/updates/voxtnew`), `scripts/release.sh:35,77,224` (`--app voxtnew`), `scripts/mac-{beta,update}-e2e.sh`, `scripts/update-server/main.go:51`. MEMORY says `/updates/voxtnew` is 404 and `ingest --app voxtnew` is rejected: the next release cannot publish or update. Keep bundle id, data dir, URL scheme, `actnow.AppID` as they are (deliberate).
4. `scripts/release-github.sh`: referenced nowhere (from the copy, 7b85172). Delete or document in docs/release.md.
5. `docs/voxt-gap-analysis.md` (no inbound link; §3–4 outdated, decisions are in RULES) and `docs/voxt-migration.md` (announcement draft): move to `docs/archive/` or delete. After #39.
6. `settings.Audio.SoundPreset` (settings.go:132): migration-only; keep until no supported version can have written it.
7. `droppedPins` (internal/models/models_test.go:99): goes with the next actnowkit release (already in MEMORY).
8. `helper/proto` vs `llmrt/wire` envelopes: leave apart (two protocols, each mirrored in Swift); recommended "no".
9. `internal/uikit` (ui_*.go as a package): needs the archtest rule "only icons uses mygo/ui" relaxed and renames every UI-kit call in every view (≈1.3k lines moved, hundreds of call sites); recommended "not now".

### Track S — shared code

**WP-S1 · `internal/procio`: the child-process plumbing helper and llmrt share** — S/M, risk low.
- Evidence: `readLine` (internal/llmrt/proc.go:132 vs internal/helper/supervise_io.go:49: differ only in `wire.MaxLine`/`proto.MaxLine`), `openLog` (proc.go:304 vs supervise_proc.go:146, identical), `stderrLevel` (proc.go:317 vs supervise_io.go:132, identical), `lineTail`/`newLineTail` (proc.go:337–343 vs supervise_io.go:260–266, identical).
- Fix: `internal/procio` (imports `core` for `core.Severity`): `ReadLine(r, max)`, `OpenLog(path)`, `StderrLevel(line)`, `LineTail`. Check `internal/omni/process.go` for the same pieces and use it there too.
- Files: `internal/procio/*` (new, with the tests moved from helper/llmrt if any), `internal/helper/supervise_io.go`, `supervise_proc.go`, `internal/llmrt/proc.go`, maybe `internal/omni/process.go`, `internal/archtest/imports.txt` (+ `helper procio`, `llmrt procio`, `procio core`), docs/architecture.md package map and import rules.
- Test: existing helper/llmrt supervisor tests; a unit test for `ReadLine` over-long lines and `LineTail`.

**WP-S2 · Small duplicates** — S, risk low.
- `isCJK`: internal/compose/pieces.go:162 = internal/eval/score.go:137 (identical), internal/asr/gemini/wire.go:314 → `lang.IsCJK` (lang is a leaf; new imports `eval lang`, `asr/gemini lang`). The compose part after #27.
- `itoa`: helpers.go:9, internal/session/manager.go:68, internal/omni/policy.go:208 → `strconv.Itoa`; helpers.go then goes.
- asr helpers: `mustJSON` (gemini/wire.go:125, aliyun/inference.go:222), `clipRunes` (aliyun/text.go:93, doubao/payload.go:233) → `internal/asr` (`asr.ClipRunes`).
- Re-exec fakes' `Enabled`/`Env`/`Opt` (helpertest/fake.go:108–118, llmrttest/fake.go:51–61, omnitest/fake.go:80–94): optional `internal/testproc`; low value, skip unless touching them anyway.
- Test: the packages' tests; declhash shows only the expected removals.

### Track M — shrink package main (logic out, with its tests)

Rule for every WP here: the moved code takes plain values and returns values or codes; words go through `internal/uitext` (the only package that imports `i18n`); `app` keeps a one-line adapter. Its unit tests move with it, so they are cached apart from the root package.

**WP-M1 · Words into `internal/uitext`** — S/M, risk low.
- Pure string functions in main (no `app`, no UI): `session_text.go` (`sessionProblemText` 46 lines, `errText`, `noticeActionLabel`; the whole file is pure), `history_retry.go` (`retryLabel`, `retryStageText`, `recordingKeptText`), `models_cards.go` (`modelSizeText`, `modelFamilyText`, `ownerName`, `modelMemText`, `percent`), `models_delete.go` (`deleteOutcome`, `keptModelsText`), `home_page.go` (`sloganHint`, `groupDigits`, `countChars`), `codex_signin.go` (`signInError`, `keyMaskOf`), `models_notify.go` (`downloadNotes`). After #27: `models_keep.go` (`keepLabels`, `keepLabel`, `keepIdle`).
- Files: those, `internal/uitext/*.go` (+ tests), the root tests that test only these functions move to `internal/uitext`. i18n keys stay the same (same English), so zh-Hans.json is not touched.
- Test: `TestKeyParity`, the moved tests, snapshots unchanged.

**WP-M2 · `internal/logview`: the logs pane's selection and order** — S, risk low.
- `shell_logs_select.go` (`logSeqs`, `entrySeq`, `hit`, `abs32` + the selection state) and `shell_logs_order.go` (`viewOf`, `newLines`); tests shell_logs_select_test.go (446 lines) and shell_logs_order_test.go are mostly logic.
- Files: those two + tests, new `internal/logview`, imports.txt (`logview core`).
- Test: moved tests; `scripts/snapshots.sh check`; the Mac logs check (`scripts/mac-logs-shots.sh`) only if the drag code moves (it should not).

**WP-M3 · Routing: deep links and moved paths** — S, risk low.
- `shell_commands.go` `openURL` (35 lines: scheme check, host/path → router path), `shell_pages.go` `movedPaths`, `section`, `pages()`'s path list (94 % of shell_pages.go is pure). New `internal/route`: `Parse(url) (path string, ok bool)`, `Moved(path)`. The URL scheme `voxtnew` stays.
- Files: shell_commands.go, shell_pages.go, shell.go (`navigate` keeps calling it), tests from shell_nav_test.go / shell_test.go that only parse.
- Test: table test of every link listed in shell_commands.go:368–371 and actnow/manifest.go:20–22.

**WP-M4 · `internal/readiness`: Home's and onboarding's readiness list** — M, risk medium.
- `home_ready.go` `readinessList` (61 lines) decides from helper status, permissions, installed models and remote choices (check whether the setup guide's steps, onboarding_steps.go, can share it). Inputs as a struct of values, output a list of `{Item, State, Action}` codes; words in uitext.
- Files: home_ready.go, home_page_test.go (the readiness cases), new internal/readiness.
- Test: table tests per combination; Home and onboarding snapshots unchanged.

**WP-M5 · `internal/diag`: self-check and diagnostics bundle** — S/M, risk low.
- `settings_about.go` `writeBundle` (53 lines, pure), `diagnosticsText` (33), `runSelfCheck` (58), `sysinfo.go` (`readSysInfo`). (`logs_check.go` is the `logscheck`-tagged screenshot driver: stays.) The view `aboutSettings` (94 lines) stays.
- Files: settings_about.go, sysinfo.go, new internal/diag (+ tests); imports.txt.
- Test: bundle written to a temp dir and listed; the About snapshots unchanged.

**WP-M6 · Models list view-model** — M, after #27 (models_page.go, models_keep.go).
- `models_view.go` `listModels` (51 lines: tab, filter, hidden, recommended first), `models_hide.go` (`shownModels`, hidden rules), `featureModel`/`featureOf` filters (feature_models.go is #27's: after). New `internal/modelsview` over `local.Model`/`providers` values.
- Test: table tests for each tab × filter × hidden; models_view_test.go / models_hide_test.go cases move.

**WP-M7 · Hotkey dispatch and the shortcut editor's rules into `internal/hotkey`** — M, risk medium.
- `session_hotkeys.go` `hotkeyFired` (69 lines: which action runs where; try places from home_try.go), `feature_keys.go` pure helpers (`presetOf`, `newHotkey`, `recordedKeys`, `keyOptions`, `triggerIndex`, `chordLabel`), `settings_shortcuts_family.go` (`shortcutID`, `shortcutOf`, `familyFunction`). feature_keys.go is not in #27's diff; `hotkey.go` (409 lines) may need splitting first (WP-B2).
- Test: table tests from session_hotkeys_test.go; the shortcuts snapshots.

**WP-M8 · Overlay answer card model** — M, risk medium.
- `overlay_card.go` `cardNow` (62 lines), `cardBlocks` (36), `cardTexts`; `overlay_size.go` `overlaySize` (32) → `internal/overlay/layout` (already the overlay's geometry) or a new `internal/overlay/card`.
- Test: overlay_card_test.go's pure cases; `TestOverlayInk`, `TestAnswerCardInk`; overlay snapshots.

**WP-M9 · Small pure pieces** — S each.
- `settings_audio.go` `moveRows`, `deviceInUse`, `recordingCue` → `internal/outmute` (cues) or a tiny `internal/devices`.
- `remote_dialog.go` `setup` (55 lines: dialog fields → `providers.Setup`) → `internal/providers`.
- `family_keys.go` `run`, `reshare`, `leave`, `keyRefs` (86 lines pure) → `internal/remote` (family shared keys).
- `session_link.go` `resolver` (32 lines) → `internal/remote` or `internal/local`.

Not proposed: moving the `*app` controllers wholesale (397 functions, 5.4k lines, all on `app` state) or the views. A controller moves only when a WP above gives it an interface of values.

### Track B — long files and functions (move-only, declhash-proven)

**WP-B1 · `internal/session` run loop** — M, risk medium. `run_listen.go:81 (*run).listen` 263 lines (the longest function; archtest reports >150), `run_audio.go:65 feed` 114, `retry.go:123 retry` 82; files `run_listen.go` 426, `session.go` 460, `run.go` 406. Split `listen` by event kind into methods. Not `text.go` (#27). Test: session tests, `-race`.

**WP-B2 · `internal/hotkey/hotkey.go` (409) and `internal/outmute/outmute.go` (477)** — S, split by topic.

**WP-B3 · Test fakes** — S: `helpertest/fake.go` 523 (`serve` 152 lines → a method table), `helpertest/output.go` 520, `omnitest/server.go Main` 85. Coordinate with WP-T2 (same package; do T2 first or in the same thread).

**WP-B4 · Root views over 100 lines** — M, view code: `settings_audio.go:165 audioSettings` 135, `overlay_check.go:196 overlayCheck` 134 (dev tool), `history_detail.go:48 historyDetail` 117, `smoke.go:31 runSmoke` 110, `appenhance_dialogs.go:40 aeAppsDialog` 103 and `:170 aeBrowsersDialog` 80, `remote_dialog.go:156 providerDialogView` 102 (file 457), `settings_about.go:278 aboutSettings` 94, `main.go:30 main` 92, `history_followup.go:108 askFollowUp` 88, `history_page.go:151 historyPage` 85, `settings_storage.go:284 storageSettings` 83 (file 409), `appenhance_preview.go:18 aePreviewCard` 83; `shell_commands.go` 406. Proof: declhash + `scripts/snapshots.sh check`. Split among threads by file (each file is one area).

**WP-B5 · After #27** — `internal/compose/compose.go` 553 (`Build` 134), `internal/settings/settings.go` 550, `shell_settings.go` 411, `feature_models.go:20 featureModels` 87.

**WP-B6 · Others** — `internal/omni/live.go` 456, `internal/datadir/datadir.go` 430, `internal/recordings/move.go:73 Move` 105, `internal/llm/schema.go:110 validate` 104, `internal/llm/anthropic.go:173 readStream` 101, `internal/llm/client.go:99 Chat` 97, `internal/deliver/diag.go:14 Diag` 93.

### Track A — keep it clean (ratchets)

**WP-A1** — S. In archtest: fail on new functions over 150 lines (allowlist `(*run).listen`, `(*fake).serve` with their sizes as ceilings, like files), report over 100. Add a CI step `go run honnef.co/go/tools/cmd/staticcheck@<pinned> -checks U1000 ./...` and `go run golang.org/x/tools/cmd/deadcode@<pinned> -test ./...` (must print nothing; the decision list of WP-D2 first). Needs approval for the tool versions (they are not in go.mod; `go run pkg@version` is fine in CI).

### Parallel plan

| Wave | Threads in parallel (no shared files) |
|---|---|
| 1 (now) | T1 · T3 · T4 · D1 · S1 |
| 2 | T2 (+ B3, same package) · M1 (minus models_keep) · M2 · M3 · M5 · B1 · B2 |
| 3 (after #27) | M4 · M6 · M7 · M8 · M9 · B4 (by file) · B5 · S2 compose part · D2 (after the user's decisions; docs after #39) · A1 |

Main merges one branch at a time and runs the full suite once per merge (RULES: load).

---

## Part 3 · Evidence

### 3.1 Commands

```sh
export PATH=$HOME/sdk/go1.27.1/bin:$HOME/go/bin:$PATH CGO_ENABLED=0
go test -count=1 -p 8 -json ./... > full.json         # 81.2 s wall, 94.7 s user, 48.2 s sys, 582 MB max RSS
go test -c -o m.test . ; ./m.test -test.list '.*'      # 280 tests; shards: ./m.test -test.run '^(A|B|…)$' from the package dir
go test -p 8 ./...                                      # cached: 0.9 s
staticcheck -checks U1000 ./...                         # 13 findings (Part 2, WP-D1)
deadcode -test ./...                                    # 15 unreachable even from tests
GOOS=darwin deadcode .                                  # 67 unreachable from the app (test-only API, §3.3)
# /tmp/c36/{funclen,unexp,fields,classify}: go/ast and go/packages scripts (function lengths,
# exported objects used outside their package, settings fields' uses, main's functions by kind)
```

### 3.2 Slowest tests (s)

Root package (21 tests ≥ 1 s, 56.7 s together): TestOverlayFlows 7.23, TestOverlayFlowsTrace 7.22, TestSessionAppEnhancement 6.13, TestSessionMutesOutput 3.80, TestFamilyRuntimeOmni 3.19, TestOverlayTwoSessionsEndToEnd 2.70, TestSessionWhisper 2.68, TestSessionOmni 2.67, TestSessionPasteChecked 2.63, TestSessionEndToEnd 1.98, TestCardInsertSwitchedApp 1.98, TestSessionEscapeAndFailures 1.72, TestSessionMuteHelperCrash 1.66, TestTranslateSpeechCard 1.58, TestSessionSwitchedApp 1.57, TestAppleSessionEndToEnd 1.53, TestRewriteConversation 1.51, TestRewriteSpaceFollowUp / SelectionInCard / AnswerFollowUp 1.29 each, TestRecordingMuteWithoutSounds 1.01. 254 of 280 tests take < 0.5 s (9.2 s together).

internal/session: TestCleanupDeadlines 6.62, TestWriteDeadlines 3.23, TestWarmLLM 2.06. internal/omni: TestLiveFinalFailsWithText 1.09, TestSetIdleTimeout 1.06, TestFinalErrors 1.05, TestSlowFinal 1.02. internal/llmrt: TestProgressDeadlines 2.78, TestModelEvents 1.03. internal/helper: TestStuckCancelRestarts 1.52, TestCrashRestartsAndFailsCalls 1.03, TestUpdatedApp 1.02.

### 3.3 Exported API reached only by tests (GOOS=darwin `deadcode .`, minus §WP-D1)

`(*app).notify` (shell_notices.go:45), `numberInput` (ui_forms.go:37); actnow `NewFakeKeychain`/`FakeKeychain.*` (fakekeychain.go); `dict.Replace` (alias.go:21; production uses `ReplaceChanges`); dsp `biquad.response`, `abs`, `HighPass.Response` (highpass.go:50–99), `Leveler.GainDB` (level.go:333); proto `ParamsFor`, `ResultFor`, `NewNotification`, `NewResult`, `NewError` (methods.go:123,206; proto.go:104–119); `hotkey.Action.String` (route.go:33); `i18n.Date` (dates.go:29); `lang.All` (lang.go:54); `llm.HeaderFunc.Headers` (backend.go:62), `llm.Array/Number/Integer/Boolean/Enum` (schema.go:67–72); `llmrt.Profiles` (models.go:161); wire `ParamsFor`, `ResultFor`, `NewNotification`, `NewResult`, `NewError`; `models.Installed`, `models.Hub` (hub.go:125,137); `notify.Center.MarkRead` (notify.go:142); `omni.Models`, `omni.ASRModels` (models.go:184,193); `outmute.Controller.Wait`, `.Status` (outmute.go:153,162); `permission.Fake.*` (permission.go:55–70); `secrets.ASR`, `secrets.LLM` (secrets.go:42–43); `settings.NewMemory`, `memDocs` (service.go:72,233); `sqlitestore.Store.Path` (sqlitestore.go:232); `vad.Gate.Heard` (gate.go:129), `vad.Detector.Heard`, `.Segments` (vad.go:284,288).

### 3.4 Package main by kind (function bodies, lines)

| Area | controller (`*app`, no UI) | pure | view / UI | total |
|---|---|---|---|---|
| settings_* | 685 | 287 | 1280 | 2252 |
| shell_* | 355 | 136 | 1294 | 1785 |
| overlay_* | 350 | 172 | 1013 | 1535 |
| history_* | 641 | 99 | 723 | 1463 |
| models_* | 562 | 164 | 489 | 1215 |
| appenhance_* | 252 | 42 | 621 | 915 |
| ui_* | 0 | 3 | 813 | 816 |
| feature_* | 126 | 141 | 476 | 743 |
| session_* | 548 | 142 | 43 | 733 |
| onboarding_* | 169 | 21 | 472 | 662 |
| dictionary_* | 282 | 32 | 338 | 652 |
| remote_* | 164 | 89 | 362 | 615 |
| family_* | 376 | 123 | 34 | 533 |
| home_* | 176 | 81 | 213 | 470 |
| others | 701 | 103 | 1145 | 1949 |
| **all** | **5,449** | **1,657** | **≈9,200** | **16,325** |

### 3.5 Reverse dependencies (packages whose tests rebuild when it changes)

core 54, asr 36, helper/proto 25, vad 21, store 20, settings 19, llm 19, lang 15, models 13, dict 12, hotkey 11, local 10 (of 122 package variants including tests).
