# Voxt Tests

XCTest and Swift Testing coverage for Voxt app behavior, services, settings, transcription, meeting, and persistence logic.

## Responsibilities

- Verifies business logic, settings state, provider behavior, model support, and integration boundaries.
- Keeps deterministic tests close to production contracts while using shared test support utilities.
- Separates skipped model/fixture-heavy diagnostics from default CI-safe coverage.

## Remote LLM suites

The former combined streaming suite is split by behavior, preserving all 78 original test methods and assertions:

- `RemoteLLMRuntimeClientEndpointsTests`
- `RemoteLLMRuntimeClientStreamingTests` (response parsing and stream payloads)
- `RemoteLLMRuntimeClientMessagesTests`
- `RemoteLLMRuntimeClientResponsesRequestTests`
- `RemoteLLMRuntimeClientCodexTests`
- `RemoteLLMRuntimeClientLocalProviderPayloadsTests`
- `RemoteLLMRuntimeClientGenerationSettingsTests`

Running only the old streaming suite no longer runs all remote LLM cases. A focused refactoring check on macOS is:

```bash
xcodebuild test -project Voxt.xcodeproj -scheme Voxt \
  -destination 'platform=macOS' CODE_SIGNING_ALLOWED=NO \
  -only-testing:VoxtTests/RemoteLLMRuntimeClientEndpointsTests \
  -only-testing:VoxtTests/RemoteLLMRuntimeClientStreamingTests \
  -only-testing:VoxtTests/RemoteLLMRuntimeClientMessagesTests \
  -only-testing:VoxtTests/RemoteLLMRuntimeClientResponsesRequestTests \
  -only-testing:VoxtTests/RemoteLLMRuntimeClientCodexTests \
  -only-testing:VoxtTests/RemoteLLMRuntimeClientLocalProviderPayloadsTests \
  -only-testing:VoxtTests/RemoteLLMRuntimeClientGenerationSettingsTests \
  -only-testing:VoxtTests/RemoteModelConfigurationTests \
  -only-testing:VoxtTests/RemoteModelConfigurationASRTests \
  -only-testing:VoxtTests/RemoteModelConfigurationCredentialLoadingTests \
  -only-testing:VoxtTests/RemoteModelConfigurationCredentialWritingTests \
  -only-testing:VoxtTests/RemoteModelConfigurationCredentialMigrationTests \
  -only-testing:VoxtTests/RemoteModelConfigurationCodexTests \
  -only-testing:VoxtTests/RemoteModelConfigurationEndpointMigrationTests \
  -only-testing:VoxtTests/RemoteEndpointSecurityPolicyTests \
  -only-testing:VoxtTests/LLMExecutionPlanCompilerTests \
  -only-testing:VoxtTests/SessionTextIOTests \
  -only-testing:VoxtTests/RewriteAnswerContentNormalizerTests \
  -only-testing:VoxtTests/RewriteAnswerPayloadParserTests
```

Then run the full shared scheme. Test-method text preservation does not replace compilation and XCTest discovery checks.

## Other split suites

- `HotkeyManager*Tests`: event routing, note shortcuts, modifiers, recovery, double taps, paste, long presses, mouse and common-stop behavior (75 preserved tests).
- `RemoteModelConfiguration*Tests`: provider configuration, ASR, credential reads/writes/migrations, Codex and endpoint migration (81 preserved tests).
- `MLXModelManager*Tests` + `CustomLLMModelConfigurationTests`: catalog policy, model lifetime, installation/storage and local LLM configuration (53 preserved tests).
- `MeetingDetailViewModel*Tests`: summaries, live updates, transcript edits and translation (21 preserved tests).

Shared setup is in `TestSupport/*TestCase.swift`. Base classes contain no test methods; only concrete suites are selected. Default restoration, actor annotations and controlled async gates are preserved. The original suite names now select only their remaining domain, not every former case.

Run the complete focused refactoring set, including core, with:

```bash
bash tools/run_local_regression_matrix.sh refactor
```

This command includes all split family files and related onboarding, settings, security and persistence suites. Follow with the full Xcode test scheme and verify test discovery/counts on macOS. See the [phase record](../docs/RefactoringProgress.zh-CN.md) for pending validation.

## Remote ASR / meeting transport contracts

`DoubaoPacketCodecTests`, `RemoteASRResponseStateTests`, `RemoteASRCompletionTests`, and `MeetingRemoteSessionLifecycleTests` add 36 deterministic cases for framing, bounded gzip, terminal text, handshake failure/timeout, cancellation, generation isolation and ordered drain. Meeting tests override the existing provider boundary and use a controllable deadline; they do not open sockets or record microphone audio.

These suites and existing ASR/meeting support tests are included in the `refactor` group. Real URLSession/provider behavior, credentials and device transitions still require separate acceptance.

## Task and session ownership contracts

`TrackedTaskStoreTests`, `LLMRequestLifecycleTests`, `MeetingLiveSessionRegistryTests`, `MLXCorrectionPassCoordinatorTests`, and the added capture-epoch regression cover stage 5A ownership (20 new cases). `TestSupport/ManualTaskBarrier` deliberately holds cancelled work until explicit release to model native cleanup. They are included in `refactor`; full recording/meeting and model lifecycle acceptance remains separate.

Stage 5B adds `HotkeyManagerLifetimeTests` (4), `HotkeyEventTapRunLoopTests` (4) and `MLXNativeLiveRuntimeTests` (7): stale queued actions, independent thread retirement, stream replacement/reentrant cleanup and model-use release. These 15 cases are in `refactor`. They exercise Voxt-owned tasks and CF sources, not real system taps or internal model workers.

Stage 5C adds `SharedModelLoadCoordinatorTests` (6), `MeetingImportedFileAnalyzerTests` (7), `RecordingSessionLifecycleTests` (6), and `MeetingFinalizationContextTests` (3): 22 cases for retained cancelled loads, import cancellation/cleanup windows, stale session output/end and stable checkpoint metadata. Three existing `SessionEndFlowTests` now exercise the real lifecycle transitions instead of a test-only static helper. The focused group includes the existing file queue and checkpoint persistence suites as well.

## Keep useful coverage

Reuse [TestSupport](TestSupport/README.md), isolated defaults and temporary directories. Delete a test only when its contract is retired or equivalent coverage is identified; do not discard cancellation, migration, security or provider-specific regressions as duplication.

Model tests are opt-in through `VOXT_RUN_MODEL_TESTS=1`; `VOXT_MODEL_STORAGE_ROOT` can point to existing checkpoints. Report skips separately from passes. See the [regression matrix](../docs/LocalRegressionMatrix.md) for hardware/model checks.
