---
name: voxtmini
description: Use VoxtMini, the user's local-first macOS voice input app, through its MCP server (`voxtmini_overview`, `list_history`, `upsert_terms`, `transcribe_audio`, `translate_text`, `rewrite_text`, `trigger_session`...). Use when the user asks about their dictation history, custom dictionary terms, app enhancement rules, local speech/language models, microphones, or voice sessions.
license: Apache-2.0
metadata:
  version: "{{VERSION}}"
---

# VoxtMini

VoxtMini is the user's local-first macOS voice input app (Transcribe, Translate, Rewrite, Dictionary, App enhancement). Its local MCP server (`voxtmini`) exposes read access when enabled, and — only when the user enables each scope in VoxtMini Settings — data management, local/remote model calls, and recording triggers.

## Rules

1. **Start with `voxtmini_overview`.** It shows which scopes are enabled, counts of history/terms/rules, ready models, and available microphones.
2. **Local first, privacy transparent.** Audio and text stay on this Mac unless the user explicitly enables remote models. Never ask for or expect API keys — VoxtMini never exposes them.
3. **Ask before modifying data.** Before calling `upsert_terms`, `delete_term`, `upsert_category`, `upsert_app_group`, `delete_app_group`, or `delete_history`, summarize what you plan to change. Batch dictionary terms in one `upsert_terms` call instead of calling once per term.
4. **Never automate VoxtMini's UI.** Do not search the web for VoxtMini or click its windows with screen/GUI automation. When VoxtMini asks the user to approve you or toggle a setting, tell the user and wait.
5. **Respect error codes.**
   - `user_declined`: the user said no — do not retry unless they ask you to.
   - `not_confirmed`: the prompt timed out — ask the user to check VoxtMini before retrying.
   - `scope_off` / `remote_off`: tell the user which permission is off in VoxtMini Settings.
   - `rate_limited`: wait `retryAfter` seconds before calling again.
   - `busy`: another recording session is active — check `session_status`.
   - `invalid_path` / `too_large`: `transcribe_audio` accepts local `.wav`, `.m4a`, `.mp3`, `.flac`, or `.ogg` files up to 50 MB inside the user's home or temp folder.

## Tools

| Area | Tools |
| --- | --- |
| Overview & Read | `voxtmini_overview`, `list_history`, `get_history`, `list_terms`, `list_categories`, `list_app_groups`, `list_models`, `list_microphones` |
| Manage (`AllowManage`) | `upsert_terms`, `delete_term`, `upsert_category`, `upsert_app_group`, `delete_app_group`, `delete_history` (single entry only) |
| Models (`AllowLocalModels` / `AllowRemoteModels`) | `transcribe_audio`, `translate_text`, `rewrite_text`, `chat_model` |
| Session (`AllowTrigger`) | `trigger_session`, `session_status` |

## Common workflows

- **Add project jargon to dictionary:** Extract key terms and likely misrecognitions from the codebase, show the list, then call `upsert_terms` once with all entries.
- **Review recent dictations:** Call `list_history` (filter by `kind` or `query`), then `get_history` on specific ids to see raw text, corrections, and follow-ups.
- **Transcribe or translate locally:** Verify ready models via `list_models`, then call `transcribe_audio`, `translate_text`, `rewrite_text`, or `chat_model` (omit `model_id` for automatic local selection).
- **Trigger voice input with a specific mic:** Call `list_microphones` to get the device UID, then `trigger_session` with `feature`, `action`, and `device`.
