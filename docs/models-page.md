# The Models page (design)

*User request, 2026-11-12 (thread #7): one Models page like Voxt's instead of Local models and Remote models, and Voxt's local models back in the catalogue.*

## Voxt's page

Voxt's model settings (`Voxt/Settings/Models/ModelSettingsView.swift`, `ModelCatalogSnapshotBuilder.swift`, `ModelSettingsSupport.swift`) are one catalogue: a tab picker ASR / LLM, a row of filter chips (location: Local, Remote; then status and quality tags; Hidden), and one list of rows, local models and remote providers alike, models in use first. A row's ⋯ hides or shows it (`hiddenModelVisibilityIDs`, keys `model:<id>`); the Hidden chip lists the hidden ones. Voxt never hides an installed model (the action is not offered, and a hidden one shows again once installed). The feature pages' model selector uses the same hidden set.

## VoxtMini's page

- **One page, `/models`, "Models" (模型)** in the sidebar's Resources group (replacing Local models and Remote models). Its header keeps Network… (the downloads' and remote requests' proxy).
- **A segmented switch: Speech models (语音模型) / Language models (语言模型).** Remembered while the app runs. The filter chips share its row, on the right (they wrap under it, still right-aligned, in a narrow window); no description under the title, only the title's ⓘ (thread #17).
- **Filter chips: All / Local (本地) / Remote (远程)**, each with its count, and, when any model of the tab is hidden, a **Show hidden (n)** chip (显示已隐藏 (n)).
- **The list**: local models first, then the remote services.
  - Local: **Recommended** (推荐) first — the curated models of today's catalogue (Qwen3-ASR 1.7B and 0.6B; Qwen3.5 4B and 2B, Hy-MT2), each with a Recommended chip — then **More models** (更多模型): the rest of Voxt's catalogue and the family's models, in the catalogue's order. A card is about three lines (behavior.md 3.33a): the name with its marks (shared with other family apps, loaded: icons whose tooltip says it) and chips, what it is good at (one line), and a caption of the real name (e.g. "Qwen3-ASR 1.7B · 8-bit", for the curated ones whose name is a plain one) and the size or the download's state. The one visible button is what the state needs (Download / Resume, Cancel, Use it; none once installed); Settings, Delete (or Remove), deleting a partial download and Hide are in ⋯ and the context menu. **Settings** opens the model's own dialog: the knobs its family takes (behavior.md 3.31a).
  - A model not yet checked on a Mac with the real runtime has an **Experimental** chip (its ⓘ says so).
  - Remote: the Remote models page's service cards (set up first, then the rest), opening the same service dialog.
- **Built into macOS** (macOS 自带), between Recommended and More models: Apple Speech on the Speech tab, Apple Intelligence on the Language tab ([apple-models.md](apple-models.md)), once the helper said what this Mac has. Counted as Local, hideable; no Download or Delete: the card says whether it can be used and why not, Apple Speech's lists English, Chinese and Japanese with their state, **Download languages** (Apple's download of the user's languages) and **Allow speech recognition**.
- **Hide / Show**: every card's ⋯ menu and context menu has Hide; hidden cards are left out of the list and of the feature pages' model selects. With Show hidden they come back dimmed with a Hidden chip and Show in the menu.
- **Hiding never deletes files.** Unlike Voxt, an installed model can be hidden too (the user asked that hidden models leave the selects): it stays installed and keeps working. A feature that chose it keeps it (its select still shows it, as the current choice), Automatic still picks among all installed models (hiding is about the list, not the session), and hiding a model a feature uses says so in a toast ("… is hidden. Transcribe still uses it.").
- Persisted in the settings: `models.hidden` (`settings.Models.Hidden`), keys: a local model's catalogue id (`asr.accurate`), a remote service's `remote:asr.<id>` / `remote:llm.<id>`. No version bump: a missing field reads empty.

## Routes

- `/models` is the page; `/models/speech`, `/models/language`, `/models/local`, `/models/remote` open it on that tab or filter (deep links `voxtnew://models/remote`).
- `/remote` (the old Remote models page) opens `/models` with the Remote filter; `/models/network` still opens Network….

## Catalogue

`internal/models` gains, per model, `Title` (its real name, not translated), `Recommended` and `Experimental` (not yet run on a Mac with VoxtMini's runtimes). (`Planned`, "Coming soon" models without a runtime, went with the models it marked on 2026-11-13: below.) Ids are stable settings keys. Voxt's models and what VoxtMini does with each are in [llm-models.md](llm-models.md) / [architecture.md](architecture.md); in short:

- Speech, runnable now (qwen3_asr_server): Qwen3-ASR 1.7B 6-bit.
- Speech, runnable now (whisper_server, thread #9, experimental until checked on a Mac): Whisper large-v3-turbo, large-v3, small. No live text (the server has no realtime API): the text comes after Stop ("Recognizing…"); their cards say so. Each needs its `openai/whisper-*` tokenizer, an auxiliary repo (`models.Model.Aux`) downloaded, claimed and deleted with it (large-v3-turbo and large-v3 share one), and runs from a folder of links in VoxtMini's data folder (behavior.md 3.36).
- Language, runnable with today's voxtmini-llm (model types in mlx-swift-lm's registry), experimental until checked on a Mac: Qwen3.5 9B OptiQ, Qwen3.6 27B, Ministral 3 3B, LFM2 1.2B, LFM2 8B-A1B, Gemma 4 E2B, Gemma 4 E4B (now downloadable, was borrowed), Gemma 4 12B OptiQ.
- Not added: Qwen3-VL 4B (`qwen3_vl` needs MLXVLM in voxtmini-llm), Hy-MT2 Q8_0 (the same repo as `llm.translate`: the family's claims are per repo and commit).

The catalogue's pins follow the family pins table where it has the repo; the new repos are pinned to their current commits, to be added to actnowkit's `pins.json` (`TestFamilyPins` lists them as pending).

## Languages (2026-11-13)

*User decision, 2026-11-13: VoxtMini speaks English, Chinese (Simplified) and Japanese, nothing else; models that do not support the three are not needed.* Every local model of the catalogue claims all three on its card; the rest went, with the "Coming soon" ones (no runtime, and no reason left to plan one):

| Model | Kind | Why it went |
|---|---|---|
| Parakeet TDT 0.6B v3 | speech, Coming soon | 25 European languages: no Chinese, no Japanese |
| MOSS Transcribe Diarize | speech, Coming soon | Chinese and English only; diarization is for meetings, out of scope |
| SenseVoice Small | speech, Coming soon | knows the three, but no runtime: Qwen3-ASR covers it |
| Cohere Transcribe 03-2026 | speech, Coming soon | knows the three (14 languages), but no runtime, and 4 GB |
| Nemotron 3.5 ASR streaming 0.6B | speech, Coming soon | knows the three (28 languages), but no runtime (its realtime protocol is missing) |
| GLM-4 9B 0414 | language, Experimental | its card claims Chinese and English; no Japanese |

Kept, each claiming English, Chinese and Japanese on its card: Qwen3-ASR 1.7B 8-bit / 0.6B 4-bit / 1.7B 6-bit (30 languages and 22 dialects), Whisper large-v3-turbo / large-v3 / small (99 languages), Silero VAD (language-free); Qwen3.5 4B / 2B / 9B and Qwen3.6 27B (201 languages), Hy-MT2 1.8B (33 languages, translation only), Qwen3 4B Instruct 2507 (119), Gemma 4 E2B / E4B / 12B (140+), Ministral 3 3B (its card lists Chinese and Japanese among its "dozens"; weaker in them than in English, which its description says), LFM2 1.2B and 8B-A1B (English, Arabic, Chinese, French, German, Japanese, Korean, Spanish).

As every model knows the three, the cards no longer show a languages line, and the speech models no longer list their languages (`omni.Spec.Languages`): the hint is the language's code. `scripts/build-omni.sh` stages only `qwen3_asr_server` and `whisper_server`. The family pins table (actnowkit v0.17.0) still lists the dropped repos for VoxtMini (`voxtnew`), all six VoxtMini's only: `TestFamilyPins` tolerates them (`droppedPins`) until a later actnowkit release removes the rows. Remote services stay (a service, not a model of ours); their language hint is one of the three.
