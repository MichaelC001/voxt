# Apple's on-device models (design)

*User decision, 2026-11-13 (thread #10): Apple's on-device speech recognition and Apple Intelligence are full model options, on the Models page and in every feature's selects — not only a stopgap while a download runs. Languages: English, Chinese (Simplified), Japanese.*

Two models that macOS brings, with no download of VoxtMini's:

| Id | Name | What | macOS |
|---|---|---|---|
| `apple.speech` | Apple Speech (Apple 语音识别) | speech recognition, on this Mac only | 15+: `SFSpeechRecognizer` on-device; 26+: `SpeechAnalyzer` + `SpeechTranscriber` |
| `apple.intelligence` | Apple Intelligence (Apple 智能) | language model (Transcribe's clean-up, Translate, Rewrite) | 26+, Apple silicon, Apple Intelligence turned on |

## What Voxt did, and what we keep

Voxt's "Direct Dictation" (`Transcription/SpeechTranscriber.swift`) records with its own `AVAudioEngine` and feeds an `SFSpeechAudioBufferRecognitionRequest` (`taskHint .dictation`, `addsPunctuation`, partial results, `contextualStrings` from a text box of its hint settings). On-device is a preference, **off by default** (`prefersOnDeviceRecognition = false`): Voxt's dictation could go to Apple's servers. The locale follows the user's main language (`zh-hans` → `zh-CN`, `zh-hant` → `zh-TW`, else the base code). After Stop it waits 900 ms for the final result, then takes the last partial; `kAFAssistantErrorDomain` 1110 ("no speech") is an empty text. It needs the Speech Recognition permission and blocks a recording without it.

Voxt's Apple Intelligence (`Core/LLM/TextEnhancer.swift`) checks `SystemLanguageModel.default.availability` (macOS 26: available, or not enabled / device not eligible / model not ready), opens a fresh `LanguageModelSession(instructions:)` per request and asks for a `@Generable` struct `{resultText}` (guided generation) for each of its tasks (clean-up, translation, a "compiled request"); it does not stream, does not map errors, and falls back to the input text when unavailable.

VoxtMini keeps: the availability reasons and their words, the locale mapping, the 900 ms wait for the final result and "1110 is no speech", contextual strings from the dictionary, a fresh session per request. It changes:

- **On-device only, always** (`requiresOnDeviceRecognition`; SpeechAnalyzer is on-device by design). A local model never sends audio away (AGENTS.md). No preference.
- **No settings of its own**: no hint text box (the dictionary's terms are the contextual strings, as for every speech model), no punctuation or partial-results switches (always on), no locale override (the main language, as for every model).
- **One recording**: the helper records once (`capture.start`) and Go feeds Apple's recogniser with the same PCM the other models get, through the session's usual stream (VAD gate, DSP). Voxt ran a second audio engine for dictation.
- **Plain text, streamed** for Apple Intelligence: compose's prompts are the same for every language model; `streamResponse` gives the live draft (Translate's and Rewrite's card); `llm.Client` cleans up and repairs JSON as for the others. No `@Generable` wrapper (it would need a struct per task and gives no stream of the text).
- **macOS 26's SpeechAnalyzer** where it knows the language (its long-form model is the better one), the old recogniser otherwise; Voxt used only the old one.

## Where each runs: the helper

Both run in **voxtmini-helper**, behind new `apple.*` methods (helper protocol 1, [helper-protocol.md](helper-protocol.md)). Go keeps the policy (which locale, which hints, token budget, temperature, when to give up).

- **Speech**: the helper is the Swift process that already holds the Mac's audio and the app's privacy identity (the Speech Recognition prompt names VoxtMini, as the Microphone's does). Go sends the session's PCM to an `apple.speech` job; the job sends partials back.
- **Apple Intelligence: the helper, not voxtmini-llm.** voxtmini-llm and `internal/llmrt` are built around weights *VoxtMini* holds in memory: the family's memory budget (`BeforeLoad` / `Reserve`), the runtime file's loaded models and their bytes, idle stop to give memory back, one model at a time, MLX's prompt cache, per-model sampling profiles. Apple's model lives in a system daemon: none of that applies, and each would need an exception. In the helper it is a small job (`apple.chat`, deltas as notifications, `job.cancel`) beside the speech, and both share one availability check, one SDK gate (`#if canImport(FoundationModels)`, macOS 26 SDK) and no extra process start: the helper runs anyway, so the Models page knows at once whether Apple Intelligence is there, without starting voxtmini-llm. A FoundationModels failure is an XPC error inside the call, not a crash of the helper; generation runs in a Swift task, away from the event tap's thread.

## The protocol

| Method | Params → result |
|---|---|
| `apple.status` | `{locales: ["en-US","zh-CN","ja-JP"]}` → `{speech{engine?, reason?, locales[{locale, status}]}, intelligence{status, reason?, languages?}}` |
| `apple.install` **job** | `{locales}` → `{locales[{locale, status}]}`; notifications `apple.progress {job, fraction}` |
| `apple.speech` **job** | `{locale, hints?}` → `{text, locale, engine, audioMs}`; notifications `apple.speech.started {job, locale, engine}`, `apple.partial {job, text}` |
| `apple.speech.audio` | `{job, data}` (base64 PCM16 LE mono 16 kHz) → `{}` |
| `apple.speech.finish` | `{job}` → `{}`; the job answers with the final text after it |
| `apple.chat` **job** | `{messages[{role, content}], maxTokens?, temperature?}` → `{text, finish}`; notifications `apple.delta {job, text}` |

- `engine`: `analyzer` (SpeechAnalyzer) or `recognizer` (SFSpeechRecognizer); absent: no speech recognition on this Mac (`reason`).
- A locale's `status`: `installed` (ready), `available` (Apple can download it: `apple.install`), `downloading`, `unsupported`. With the recogniser, `installed` means `supportsOnDeviceRecognition`, and nothing is `available`: macOS 15 downloads dictation languages itself (System Settings → Keyboard → Dictation).
- `intelligence.status`: `available` or `unavailable` with `reason` `notEnabled`, `deviceNotEligible`, `modelNotReady`, `unsupportedOS` (before macOS 26, or a helper built without the macOS 26 SDK), `other`; `languages`: the base codes it supports (`en`, `zh`, `ja`…).
- `apple.speech` starts the recogniser, then sends `apple.speech.started` (the stream is open); audio requests are handled in arrival order on the helper's reader thread (Go sends one after the other). After `apple.speech.finish` it waits for the final result at most 900 ms after the recogniser's last word (Voxt's), then answers with the last partial. No speech is `text: ""`.
- New error codes: `context_length` (Apple Intelligence's window, 4096 tokens, prompt and answer together), `refused` (its guardrails); `unsupported` with `data.capability` `speech` / `intelligence` / `locale`; `permission_denied` with `data.permission: "speechRecognition"`.
- `permissions.status` gains `speechRecognition` (a Perm* state; absent from an older helper); `permissions.request {permission: "speechRecognition"}` shows macOS's prompt.
- An older helper without `apple.status` in hello's `methods` means no Apple models: protocol 1 stays.

## In Go: `internal/apple`

`apple.Services` wraps the local models (`local.Services`) as `internal/remote` does, and `remote.Services` wraps it in turn:

- `Installed(kind)`: the downloaded local models, then `apple.speech` / `apple.intelligence` when usable, (then the remote services). Memory only: the last `apple.status` the app read (after each helper start, when the Models page shows, after an install, after a permission prompt).
  - Apple Speech is usable when the helper has an engine and at least one of the three locales is `installed`.
  - Apple Intelligence when `intelligence.status` is `available`.
- **Automatic** (`local.Choose`, unchanged) takes the first local model: a downloaded one when there is one, else Apple's. So on a Mac with Apple Intelligence and nothing downloaded, every feature works at once; once a recommended model is installed, it is the automatic one again. The setup guide still offers the recommended downloads (`obReady` leaves Apple's models out).
- `ASR(ctx, "apple.speech")`: an `asr.Streamer` (`Caps{Stream, Hints}`, not `File`: a session streams to it; its `Transcribe`, which a history entry's retry calls, streams a whole recording too). The locale is the session's language hint mapped to `en-US`, `zh-CN` or `ja-JP` (`apple.Locale`; another language: `asr.KindConfig` wrapping `apple.ErrLanguage`); hints are the dictionary's terms (at most 100, Apple's advice for contextual strings). The stream is a `Salvager` (the last partial).
- `LLM(ctx, "apple.intelligence")`: `llm.Client` (one attempt, JSON repair, thinking stripped) over `apple.Chat`, a `llm.Backend` over `apple.chat`. Temperature 0 is greedy. `maxTokens` is the request's, else what the window leaves after the prompt (estimated at 1 token per CJK character, per 4 other characters), at most 2048. A prompt that does not fit is `llm.CodeContextLength` before anything is sent; compose already sends speech in pieces of at most 300 characters, so only a long Rewrite selection gets there.
- Errors wrap sentinels the UI words: `apple.ErrPermission` (Speech Recognition not allowed: the notice opens Permissions), `apple.ErrLanguage` (the language is not ready: Models), `apple.ErrUnavailable` (Apple Intelligence went away: Models), `apple.ErrRefused` (guardrails).

Imports: `apple ──▶ asr, llm, local, helper/proto`.

## In the UI

- **Models page**: a section **Built into macOS** (macOS 自带) between Recommended and More models, with one card per tab: Apple Speech on Speech models, Apple Intelligence on Language models. Counted as Local; hideable like any card. The card says "No download: macOS provides it", its languages, and its state:
  - Speech: one line with each language and its state ("English: Ready · Chinese (Simplified): Ready · Japanese: Not downloaded"; Downloading…, Not supported on this Mac); **Download languages** installs the user's languages (the main and other ones among the three) that are `available` (Apple's download, with a bar, then a toast); **Allow speech recognition** while it was never asked; refused, a notice that opens Settings → Permissions.
  - Intelligence: Ready, or why not ("Turn on Apple Intelligence in System Settings…", "This Mac does not support Apple Intelligence", "Apple Intelligence is still preparing its model…", "Needs macOS 26 or later").
- **Feature selects**: they list Apple's models among the installed ones, by name.
- **Settings → Permissions**: a Speech Recognition row ("Only Apple Speech needs it"), shown only while a feature uses Apple Speech (its speech model, or Automatic with no speech model downloaded) and the helper reports the permission (user, 2026-11-13); its controls are the Microphone row's (Ask while not asked yet, Open System Settings until allowed).
- **Sessions**: a Speech Recognition refusal fails with "VoxtMini may not use speech recognition" and Open Permissions; the helper asks once on the first use (as the microphone's).

## Prompts and limits

- Compose's prompts are unchanged (system, the clean-up example turn, the text). The helper turns the messages into a `Transcript` (instructions, then prompt / response turns) and streams the last user message's answer.
- 4096 tokens in all; no tokenizer is exposed, so Go estimates (above).
- No prompt cache, no prefill (`llm.Prefiller` is not implemented), no sampling profiles beyond the temperature.
- Apple Intelligence's languages (`intelligence.languages`) are shown on the card; Go does not refuse other languages (it answers `unsupported` with `capability: locale`, worded as "does not support this language").

## Mac checks (not run yet)

The Swift side is type-checked on Linux against stubs of the SDK (Swift 6, strict concurrency) and its pure half (`AppleKit.swift`, the wire types against the fixtures) tested there. On a Mac with macOS 26 (and once on macOS 15):

- the SDK names compile as the stubs guessed them: `SpeechTranscriber(locale:transcriptionOptions:reportingOptions:attributeOptions:)`, `AnalysisContext.contextualStrings[.general]`, `AssetInventory.status(forModules:)` / `assetInstallationRequest(supporting:)` (and whether `AssetInventory.reserve(locale:)` is needed before a download), `SpeechAnalyzer.bestAvailableAudioFormat(compatibleWith:)`, the `Transcript` initialisers, `GenerationOptions(sampling:temperature:maximumResponseTokens:)`, `ResponseStream`'s snapshots' `content`, `GenerationError`'s cases;
- SpeechAnalyzer's languages (en-US, zh-CN, ja-JP) and their download; whether it needs the Speech Recognition permission at all (we ask in both cases);
- the partials' cadence and the text's spacing (`SpeechText`: no space next to CJK text);
- Apple Intelligence's Chinese and Japanese clean-up, translation and rewrite through the eval (`docs/eval.md`), against `llm.fast`.
