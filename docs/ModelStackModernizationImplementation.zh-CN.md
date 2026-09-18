# 模型栈现代化：实施与验证记录

分支：`chore/model-stack-modernization`
实施环境：Linux x86_64，无 Xcode、macOS SDK、Apple GPU。
对应方案：[ModelStackModernizationPlan.zh-CN.md](ModelStackModernizationPlan.zh-CN.md)

**状态：已完成主要清理与局部去阻塞代码；尚未完成全部计划，不能作为已经通过 macOS 验收的发布版本。** 尤其 MLX 升级、完整 Swift 构建与模型质量 / 性能实测仍是阻断项。

## 1. 已落地的源码改动

### FluidAudio 全链路移除

- 从 Xcode package reference、product、Frameworks 引用中删除 FluidAudio。
- 删除 Offline VBx、FluidAudio streaming diarizer、失败回退、共享实例和运行时参数。
- 删除 VBx 模型下载 / 存储实现以及 `canImport(FluidAudio)` 分支。
- 默认独立说话人分离使用已有 Sortformer；MOSS 原生输出不变。
- 删除引擎选择器，保留 Sortformer 状态与现有下载重试入口。
- 删除无效人数提示类型及旧偏好键；没有修改 Sortformer checkpoint 的 `numSpeakers`。
- 旧 `offlineVBx` 通过设置归一化迁移。新增迁移幂等测试。
- 历史会议与缓存文件未删除、未重写；音频 fixtures 未修改。

### 隐藏模型和失效参数清理

- 删除 25 个隐藏 ASR、16 个隐藏 LLM，保留原有可见 ASR 11 个、LLM 12 个和 Hy-MT2 GGUF。
- 删除 `hiddenSupport` 状态及“安装后恢复展示”链路。
- 删除 Voxtral / Canary / Moonshine / MMS / Wav2Vec2 / LASR / Granite / FireRed / GLM-ASR 的本地加载分支、专用能力与参数；远程供应商能力不变。
- ASR loader 使用目录能力注册表，不再通过 repo 子串猜架构。未知模型不走默认 Qwen loader。
- 删除 FireRed 分组 / 推理特判、恒为 nil 的自动 bias 合并、无调用者的参数辅助函数。
- 本地 VLM 能力收敛到实际可加载的目录模型，不再宣称任意匹配名称的 repo 都支持图片。
- 旧 ID 只作替代模型迁移。配置保存覆盖听写、翻译、改写、会议摘要和笔记标题选择。
- 清理失效 ASR family 调参、已退役 LLM per-repo 调参和远程体积缓存。退役模型设置不会覆盖替代模型设置；同 checkpoint 的改名 alias 保留参数，现行 ID 优先。
- 更新模型测试和本地 ASR 清单；远程模型名称 / 图标识别中仍可出现同名品牌，这是共享远程展示能力，不是本地模型运行时。

### 局部链路和 UI 优化

- 模型目录成员不再依赖逐个磁盘安装快照；去掉仅用于隐藏模型展示的重复扫描。
- 模型页生命周期改为分段 `some View` 属性，移除该处多层 `AnyView`，保留原有 debounce 与可见性判断。
- 本地 LLM 预览按 50 ms 限流，首块立即发布，结束补发尾块并去重；最终生成预算和内容不变。
- 会议翻译状态更新只替换对应缓存行 / 说话人分组，不重新计算全文说话人序号；搜索激活时仍走完整过滤，避免遗漏翻译命中的结果。
- 删除会议 VAD 偏好读取中多余的同步主线程桥接，直接调用已有 `nonisolated` 读取方法。
- 删除未被调用的本地 idle warmup 方法；保留会话按需预热，不新增空闲推理。
- 本地 LLM 推荐系列与默认 Qwen 选择一致。

上述优化尚无 Instruments 或延迟对照结果，不宣称已达到某个提速百分比。

### 非模型依赖和发布门禁

- Sparkle 固定到 `2.10.0`。
- GRDB 固定到 `7.11.1`。
- swift-log 固定到 `1.15.1`。
- FaviconFinder 固定到现有 `5.1.5`，避免范围解析漂移。
- PermissionFlow `2.11.2`、llama.swift `2.10549.0` 保持。
- 新增 `tools/audit_model_stack.py`：源码禁用运行时检查、目录检查、resolved 兼容组合校验、macOS App 动态链接 / 文件检查、静态链接映射检查。
- 新增 `tools/resolve_dependencies.sh`：已有 lockfile 则严格使用；不存在时仅在 macOS 生成供审查的实际解析结果，不伪造 lockfile。
- 工作区 `Package.resolved` 路径不再被 gitignore 排除。测试 CI 保存解析结果为 artifact；发布前要求 lockfile 已入库。
- 测试和发布 CI 均指定 Xcode 26.5，打印实际工具链版本。
- Release 构建生成 link map，并审计 App 与静态链接输入，不能只依赖文本 grep。

## 2. 本环境实际完成的验证

| 检查 | 结果 / 边界 |
|---|---|
| `python3 tools/audit_model_stack.py` | 源码审计通过；不是 App 二进制审计 |
| `python3 -B -m unittest discover -s tools -p 'test_*.py' -v` | 7 个 Python 审计 / 隔离快照工具测试通过 |
| `bash -n tools/resolve_dependencies.sh tools/run_vad_damaged_cache_smoke.sh` | 通过 |
| `git diff --check` | 通过 |
| Xcode pbxproj OpenStep 解析及包引用有效性 | 通过静态检查 |
| GitHub Actions YAML 解析 | 通过静态检查 |
| 修改 Swift 文件的 tree-sitter 与基线诊断比较 | 未发现新增语法诊断；解析器存在基线恢复诊断，不等于 Swift 编译或类型检查 |
| XCTest、SPM 解析、Release build、模型推理 | **未执行：环境不支持** |
| App / zip / DMG 体积与速度差值 | **未测量** |

新增 / 修改 XCTest 覆盖：全部退役 ID 迁移、目录不再隐藏兼容、失效配置清理、同模型 alias 参数优先级、VBx 迁移、本地预览节流、翻译更新缓存及搜索一致性。它们需要在 Mac 上运行才能视为通过。

## 3. 阻断项与尚未完成的工作

### MLX fork 候选已准备，正式切换尚未完成

用户提供本地 fork 后，已在 `../mlx-audio-swift` 的 `chore/voxt-model-stack-modernization` 分支继续实施：

- 从 Voxt 使用的 `.12` tag 出发合入上游至 `3e97855`，保留结构化 ended、Qwen KV / language 和 Nemotron 流式补丁。
- Manifest 升为 Swift 6.3，候选组合为 `mlx-swift 0.31.6`、LM `c6446cf`、transformers `1.3.4`、huggingface `0.10.2`。
- 适配新 LM 可抛错的缓存 API，保留双方测试并新增流式输出契约测试。
- 删除与候选不匹配的旧 fork lockfile，CI 改为真实解析后严格构建 / 测试，保存解析图 artifact。
- Voxt 新增隔离联调快照工具和 API 适配 patch；保留预量化加载，适配新模型 `prepare()`、EOS / chat conventions 和 typed prefill。
- fork 候选已作本地提交 `0bfe9f31e6473b461f3bdf2c3382f2478792976e`，未创建发布 tag、未推送远程。生成隔离快照已成功；7 个 Python 工具测试通过。fork 与 Voxt 候选都仍需 macOS 编译和模型回归。

共享工程不引用尚未发布的 fork，正式兼容组合仍保持：

- `mlx-audio-swift`: `0.1.3-voxt.12`
- `mlx-swift`: audio fork 的 **exact 0.31.4**
- `mlx-swift-lm`: `d2424294a6c3bbd0de37a0761d80efc05e6813dd`

`mlx-swift 0.31.6` 要求 Swift 6.3。即便 Xcode 26.5 满足工具链，现有 fork 的 exact pin 仍需更新。Audio fork 含结构化 ended 输出、Qwen KV / language、Nemotron streaming 等产品所需改动，不能简单换上游 tag 或 main 丢掉这些 API。

下一步必须：

1. 在 macOS 确认 `xcrun swift --version`。
2. 在 Mac 验证现有 fork 候选，修复编译 / 回归问题后再提交并发布不可变 revision / 新 tag；本次已修改本地 fork，但没有推送远程或创建 tag。
3. 使用 `tools/prepare_mlx_upgrade.py` 联调 audio / mlx-swift / lm 及 `tools/mlx-next.patch`，验证预量化 loader、OptiQ、VLM、Qwen / MOSS / Nemotron ended 输出；通过后将已验证改动切入正式工程。
4. 更新 audit 中的兼容组合，再运行完整 XCTest 与模型回放。
5. `mlx-swift-lm #620` 是 cache 管理变更，不能在未测量时直接称为首 token 提速。

### macOS 构建与发布图未验证

- 当前工作区仍无实际生成的 `Package.resolved`。需在 Mac 生成、检查并提交；发布门禁会拒绝缺失的 lockfile。
- 新依赖 API 兼容、Swift actor / Sendable / SwiftUI 类型检查均需真实编译确认。
- FluidAudio 独占包资源与二进制是否完全消失，要通过 Release 链接映射和 App 审计确认。
- Sortformer 的多人 / 重叠讲话 / 长会议身份稳定性需要真实模型验证，不默认接受替换质量退化。

### 更大范围性能重构暂未实施

- 模型管理器的所有安装校验尚未整体迁移到后台 actor；本次删除了隐藏目录扫描和重复目录成员扫描。
- 会议 live 全量快照处理尚未改为完整增量索引；本次优化了翻译状态更新。
- shared MLX allocator 的同步回收、全局推理仲裁、原生流式背压与取消需要真机 profile 后再调整，不能靠增加 detached task 破坏所有权和串行保障。

这些是未完成事项，不应在发布说明中写成已提速或全部重构完成。

## 4. Mac 上的接续验证

```bash
export DEVELOPER_DIR=/Applications/Xcode_26.5.app/Contents/Developer
bash tools/resolve_dependencies.sh
# 检查生成的 Package.resolved；确认兼容组合与所有传递依赖后再入库。
xcodebuild build -project Voxt.xcodeproj -scheme Voxt \
  -destination 'platform=macOS' -onlyUsePackageVersionsFromResolvedFile \
  CODE_SIGNING_ALLOWED=NO
xcodebuild test -project Voxt.xcodeproj -scheme Voxt \
  -destination 'platform=macOS' -onlyUsePackageVersionsFromResolvedFile \
  CODE_SIGNING_ALLOWED=NO
```

有权重的 Apple Silicon Mac 再运行 `VOXT_RUN_MODEL_TESTS=1` 的模型回放，并按方案记录首包、Final、LLM TTFT、峰值内存、长会议质量与 UI 主线程时间。使用相同架构、工具链、Release 配置比较包体，不能将下载的 XCFramework 压缩包大小当作 App 减量。
