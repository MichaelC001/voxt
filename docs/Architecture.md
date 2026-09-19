# Source layout and runtime boundaries

Voxt is a macOS 15+ menu bar application, built with the shared `Voxt` Xcode scheme. Build instructions live in [CONTRIBUTING](../CONTRIBUTING.md); dependency pins and model validation requirements live in [MLXAudioDependency](MLXAudioDependency.md).

## Source map

| Directory | Responsibility |
| --- | --- |
| `Voxt/App/` | Application lifecycle, dependency assembly, feature routing, text delivery and window coordination |
| `Voxt/App/Recording/` | Recording start/stop, capture and output routing |
| `Voxt/Core/` | Shared values, persistence, preferences, prompts, security, logging and runtime support |
| `Voxt/Core/LLM/` | Execution plans, provider payloads, remote requests, parsing and answer models |
| `Voxt/Core/History/`, `Voxt/Core/Dictionary/` | Domain stores, repositories and query/value support |
| `Voxt/Core/Notes/` | Note storage, external export state and Obsidian/Reminders sync |
| `Voxt/Core/Models/` | Local LLM/GGUF loading, installation, downloads and model storage support |
| `Voxt/Core/Transcription/` | Capture metrics, VAD planning, context and transcript support |
| `Voxt/Transcription/` | Speech and MLX transcription, ASR model management |
| `Voxt/Transcription/RemoteASR/` | Remote ASR capture, upload and streaming protocols |
| `Voxt/Meeting/` | Meeting coordination and live sessions; capture, processing and speaker-analysis subdomains |
| `Voxt/Hotkey/` | Event handling, trigger rules, preferences and shortcut recording |
| `Voxt/Settings/` | SwiftUI settings and onboarding; `Shell/` owns navigation and shared presentation |
| `Voxt/Windows/` | AppKit windows, overlays, detail views and their view models |
| `Voxt/Resources/`, `*.lproj`, assets | Bundled prompts, runtimes, localization and visual resources |
| `VoxtTests/` | Tests; `TestSupport/` contains shared fixtures, doubles and model-test gates |
| `tools/`, `Config/`, `packaging/` | Validation/build scripts, signing configuration and packaging inputs |

`Voxt/Voxt/Info.plist` is referenced by the build settings; the nested directory is not an orphan. There is no `Voxt/UI/` directory. Both targets use file-system-synchronized Xcode groups. Preserve membership exceptions and resource references when moving files.

## Main paths

- `AppDelegate` assembles model managers, stores, transcribers and UI coordinators. Its extensions route hotkeys, recording, translation, rewrite, notes and meetings.
- Recording selects a `TranscriberProtocol` implementation. MLX planning, merging, preview text, capture buffers, detached inference and structured segment conversion are in separate files under `Transcription/`. `MLXTranscriber` still owns task cancellation, session revision and model leases; the split does not change that lifetime boundary.
- LLM tasks compile into requests before local or remote execution. `RemoteLLMRuntimeClient` routes compiled requests; sibling extensions own Responses execution, chat completion execution, request construction, provider settings and runtime policy. Existing endpoint/message/parser helpers remain shared with connectivity checks.
- Final output is prepared once as an immutable `SessionFinalizeContext`, then delivered to the input target or answer UI. The delivery callback records history and dictionary evidence. Session-end orchestration remains under `App/Recording/`; there is no generic finalize-stage runner.
- Remote ASR retains recording/generation ownership in `RemoteASRTranscriber`, with file requests and provider streams in sibling files. Provider response actors freeze terminal text and respect cancellation. Dictation and meetings share bounded Doubao framing/gzip and Aliyun endpoint helpers, not their different transcript projection policies.
- Meeting coordination combines microphone/system audio, live transcription and final processing. Remote provider classes are separate; their base session owns ordered buffering, stop deadlines and exactly-once cleanup. App-level session/token ownership remains in the coordinator and is still a refactoring boundary.
- History and dictionary persistence use repositories backed by `VoxtDatabase` (GRDB/SQLite), with legacy-data migration. Notes and external sync have separate stores/coordinators.

## Current limitations and maintenance rules

This is one app target, not a set of independently isolated Swift modules. `AppDelegate` still owns substantial cross-feature state, and some shared preference types reside under `Settings`. Directory names do not enforce a dependency boundary.

Keep framework/UI work at presentation and integration boundaries; prefer pure value/planning logic for testable decisions. Reuse existing managers and stores before adding abstractions. Split by ownership or protocol responsibility, not arbitrary line counts. Do not broaden every `private` member merely to enable file splitting.

The project enables default MainActor isolation and approachable concurrency. Preserve explicit `nonisolated`, `Sendable`, task cancellation and ownership assumptions when extracting code. Existing `@unchecked Sendable` types require case-by-case review, not mechanical annotation removal.

Settings sidebar/header/footer and notification/feedback dialogs have dedicated files. Onboarding step implementations and presentation components are separated, but SwiftUI state remains owned by `OnboardingGuideView`; cross-file extensions are not independent state owners.

See the [refactoring assessment (中文)](RefactoringAssessment.zh-CN.md) and [phased implementation record](RefactoringProgress.zh-CN.md) for measured hotspots, completed cleanup, pending work and verification limits.
