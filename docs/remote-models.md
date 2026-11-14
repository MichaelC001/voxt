# Remote models (design)

*User request, 2026-11-11: VoxtMini gets Voxt's remote speech and language models back, beside the local ones. Local stays the default.*

## Rule

- Local models are the default and keep working unchanged. **Audio and text leave the Mac only for a feature whose model the user picked as a remote one.** "Automatic" never picks a remote model, even when no local model is installed.
- The microphone's usage description (`mygo.json`, `helper/Support/Info.plist`: what macOS shows when it asks for the microphone) says so: recognised on this Mac unless the user picks a remote speech service for a feature (security audit M5; Apple Speech's description stays "on this Mac only", which holds for it).
- Keys live in the macOS Keychain (service `VoxtMini`, accounts `asr.<id>` / `llm.<id>`, `internal/secrets`), never in the settings, the database or the logs (an error that quotes a key is masked).
- Requests follow the family's network setting (`network.proxy`, the same proxy rule as model downloads: automatic = environment, then the Mac's system proxy; loopback never proxied). Endpoints must be `https://` / `wss://`, plain `http://` only on loopback (Ollama, LM Studio, oMLX).

## Providers

Voxt's lists (`Voxt/Core/Models/RemoteModelConfiguration.swift`, 1.15.0) against what VoxtMini ports (Auralink's `internal/providers`, `internal/asr/*`, `internal/llm` transports):

| Speech (Voxt `RemoteASRProvider`) | VoxtMini | Mode | Code |
|---|---|---|---|
| OpenAI Transcribe (gpt-4o-mini-transcribe, gpt-4o-transcribe, whisper-1) | ✅ OpenAI-compatible | upload | `asr/openai` (Auralink) |
| Doubao (Volcengine bigmodel ASR 2.0 / 1.0) | ✅ | stream (WebSocket) | `asr/doubao` (Auralink) |
| Aliyun Bailian (Qwen3-ASR realtime, Fun-ASR, Paraformer) | ✅ | stream | `asr/aliyun` (Auralink) |
| StepFun | ✅ (OpenAI-compatible upload) | upload | `asr/openai` with StepFun's endpoint |
| Z.ai GLM-ASR | ✅ (OpenAI-compatible multipart, not streamed) | upload | `asr/openai` with GLM's endpoint |
| Xiaomi MiMo ASR (chat completions with `input_audio`) | ✅ | upload | `asr/mimo` (new, from Voxt's `xiaomiMiMoASRPayload`) |
| Google Gemini (Live API, gemini-3.5-transcribe-live) | ✅ (2026-11-14) | stream (WebSocket) | `asr/gemini` (new, from Voxt's `GeminiLive*`): setup → setupComplete → `realtimeInput` audio, `audioStreamEnd`; dictionary as `customVocabulary` |

| Language (Voxt `RemoteLLMProvider`) | VoxtMini preset |
|---|---|
| Anthropic | `anthropic` (Messages API) |
| Google | `gemini` (OpenAI-compatible endpoint) |
| OpenAI | `openai` |
| DeepSeek, OpenRouter, xAI (Grok), Z.ai, Volcengine, Kimi, MiniMax, Aliyun Bailian, StepFun, Xiaomi MiMo | `deepseek`, `openrouter`, `xai`, `zhipu`, `volcengine`, `moonshot` (+ intl), `minimax`, `dashscope` (+ intl), `stepfun`, `mimo` |
| Ollama, LM Studio, oMLX (local servers) | `ollama`, `lmstudio`, `omlx`: OpenAI-compatible on loopback, key optional |
| Codex (ChatGPT account) | — (added 2026-11-14, removed the same day at the user's request: not now) |
| — | Auralink's extra presets kept: SiliconFlow, Groq, Mistral, Together, Fireworks, Cerebras, NVIDIA NIM, Custom (OpenAI-compatible) |

Since 2026-11-14 (user):

- **Responses API**, per service (`settings.Provider.API`, "" = its chat completions): OpenAI, xAI, Volcengine Ark, DashScope (both sites) and Custom offer it (`llm.Preset.Responses`); `internal/llm/responses*.go`.
- **Web search**, per service (`settings.Provider.WebSearch`, off by default), **for Rewrite only** (Transcribe and Translate never search: latency): over the Responses API (`tools:[{type:"web_search"}]`: OpenAI, xAI, Ark, DashScope), Anthropic's `web_search_20250305` (at most 5 searches; the answer is the text after the last tool block) and Zhipu's chat tool. Gemini's OpenAI-compatible endpoint has no search tool for chat. The dialog shows the switch only where the service searches over the API chosen (`providers.SearchSupported`).
- **Codex (ChatGPT account)**: removed (user, 2026-11-14: not now). A setup of it in older settings is dropped on load (`settings.dropRemovedProviders`); a feature that used it goes back to Automatic (local). Its Keychain item `llm.codex`, if any, stays unread.

## Settings

`settings.Providers{ASR, LLM []Provider}` (Auralink's document, back in v4 without a version bump: a missing field reads empty). A `Provider` is one service the user set up: `ID` (the preset's, or `custom-2`), `Preset`, `Name`, `Enabled`, `Endpoint`, `Model`, `AppID` (Doubao), and the advanced parameters:

- speech: `Prompt` (services that take one), `NoHints` (don't send the dictionary as hot words / prompt), `Language` ("" = the main language, as local models);
- language: `Temperature`, `TopP` (nil = VoxtMini's per-skill default), `MaxTokens` (0 = default), `ExtraBody` (JSON merged into every request body over the preset's, e.g. `{"thinking":{"type":"enabled"}}`), `API` ("" / "responses"), `WebSearch`;
- `CredentialRef`: the ActNow family's key the service uses ("actnow:vendor.deepseek"), "" its own item (below).

A feature's `ASR` / `LLM` (`settings.Feature`) holds a local model id, `""` (automatic: the recommended installed local model) or `remote:<provider id>`.

## Code

- `internal/secrets` (Auralink's, service `VoxtMini`).
- `internal/asr`: `endpoint.go`, `ws.go` back (WebSocket dial, endpoint policy); `asr.Config.Proxy` is a proxy function (the family's), not Auralink's netx mode. `asr/openai`, `asr/doubao`, `asr/aliyun`, `asr/asrtest` from Auralink, `asr/mimo` new.
- `internal/llm`: Auralink's OpenAI and Anthropic transports, presets (`presets.json`), SSE, `Probe`, `NewBackend`.
- `internal/providers`: presets, setups, `Ready`, `NewASR`, `NewLLM`, `Test` (Auralink's, with the advanced parameters).
- `internal/remote`: `remote.Services` wraps `local.Services` and is what the app's `a.local` is: `Installed` lists the local models, then each **ready** remote service as a `local.Model{ID: "remote:<id>", Remote: true}`; `ASR` / `LLM` open a remote id through `providers` with its key (`Keys`, read off the main thread), a local id through the wrapped services. `local.Choose("")` only picks a local model. The session, its resolver and the pages are unchanged in shape: they still ask `local.Services`.
- Keys are read once in the background (masks and "has a key" kept in memory; the build pass reads only those).

## Keys shared with the ActNow family (2026-11-14)

Family spec §8 (actnowkit `credentials`): a vendor's key can live once in the login Keychain (service `ActNow`, account `vendor.<id>`) with its hint and users in `credentials.json`, for every family app.

- **Vendor**: VoxtMini's services are Auralink's (same accounts), so the family's table of Auralink's accounts decides (`asr.aliyun` and `llm.dashscope` → `dashscope`, `asr.doubao` → `volcengine-speech`, OpenAI only at api.openai.com…), plus VoxtMini's own: `asr.gemini` → `gemini`, `asr.glm` → `zhipu`, `asr.mimo` → `mimo` (`internal/actnow/credentials.go`). Custom services and local servers share nothing.
- **Use**: when the family has the vendor's key and the service has none, the dialog shows "Use the DeepSeek key from Auralink (sk-…a1b2)" with Use this key / Enter another key in place of the key field. The key is read only after the click (`Store.Use`: VoxtMini joins its users), not even a test reads it before. Saved, the service keeps `CredentialRef` and no key of its own; closed unsaved, VoxtMini leaves the users again.
- **Share**: under the key field, "Let other ActNow apps on this Mac use this key" (only when another ActNow app is installed; on by default for a new key). Shared, the key is written to the family's item only (no copy of VoxtMini's own: one key, changed everywhere); unchecked later, it moves back to VoxtMini's item.
- **Read**: `actnow.Ring` (behind `remote.Keys`) reads VoxtMini's own item first, then the family's for a service with `CredentialRef`, only once VoxtMini agreed; a family key gone or not agreed to is no key. Another app's change (`TopicCredentials`) re-reads the masks.
- **Remove**: VoxtMini stops using the family's key (other apps keep it); when no other app uses it, it is deleted. Simpler than Auralink's "Stop using it here / Delete for all apps" question.

## UI

- **The Models page** (since 2026-11-12, [models-page.md](models-page.md); `/remote` opens it on its Remote filter): below the local models of the same tab (Speech models, Language models), cards per preset (set-up ones first): state dot, name, model, key mask, Test. A card opens the **service dialog**: key (Keychain; or the family's key offered, and the share checkbox), endpoint, model (suggestions + any id), App ID for Doubao, and a collapsed **Advanced** section (speech: prompt, language, dictionary hints; language: API where there is a choice, "Search the web when rewriting" where supported, temperature, top-p, max output tokens, extra body JSON), Test, Remove, Save.
- **Feature pages** (Transcribe, Translate, Rewrite): the Models selects list the installed local models, then the ready remote services ("DeepSeek · deepseek-chat", tagged remote). Choosing one shows an info notice: "Audio/Text is sent to X." App enhancement and the dictionary are layers on those features, so they follow them.
- Home's readiness, the setup guide and the session's blocker count a chosen remote service as ready.
