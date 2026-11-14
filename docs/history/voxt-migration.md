# From Voxt to VoxtMini

*Decided by the user, 2026-10-18.*

VoxtMini is a **new app**: Voxt rebuilt from scratch on Go + MyGo with local models only (Qwen3-ASR on the Omni native runtime, MLX / GGUF language models in `voxtmini-llm`). It keeps Voxt's core, Transcribe, Translate, Rewrite, App enhancement and the Dictionary, and leaves out meetings, notes, file transcription and sync.

*Note, 2026-11-14: VoxtMini is published in Voxt's former GitHub repository, hehehai/VoxtMini (Voxt's history kept up to the tag `voxt-final`; Voxt itself continues as SGLang Omni Voxt). actnow.dev renamed its VoxtNew pages to Voxt, so actnow.dev/voxtnew and its download page are gone; downloads are on GitHub Releases until VoxtMini has a page there.*

*Note, 2026-11-11: remote models are back (the user's decision): local models stay the default, and a feature uses a remote speech or language service only when the user sets one up on the Remote models page and picks it on the feature's page ([remote-models.md](../remote-models.md)). The announcement draft below was written before; if it is published after this lands, its "remote models are not included" / "no API key is needed" lines need updating (e.g. "local by default; remote models optional").*

## What this means for Voxt's users

- **No automatic update.** VoxtMini has its own bundle id (`dev.hehehai.voxtnew`), a different signing team (779M533DL5, that of the ActNow apps) and its own update feed (`https://actnow.dev/updates/voxtnew`). Voxt's updater never offers it.
- **Voxt keeps working.** Nothing in VoxtMini changes or removes Voxt. Users can keep Voxt, switch, or run both: VoxtMini's default shortcuts are on Right Option, so they don't clash with Voxt's.
- **They hear about it through Voxt's announcements.** Voxt fetches notices from `https://voxt.actnow.dev/api/app/notifications` (`VoxtNotificationStore`) and shows them in Settings → Notifications. The service and its admin panel are in the `voxt-service` repo (`notifications` table: title, content, cover image, version, published at, draft / published).
- **Downloads** are on GitHub Releases, https://github.com/hehehai/VoxtMini/releases, not on Voxt's site.

## The announcement (draft for the voxt-service panel)

Voxt shows `content` as plain text (`Text(content)` in `SettingsNotificationDialog.swift`): links are not clickable, so the address is written out to be copied.

**Title (zh):** Voxt 的下一代：VoxtMini 发布了

**Content (zh):**

> 我们把 Voxt 从头重写了一遍，做成了一个全新的 App：VoxtMini。
>
> · 全部本地运行：语音识别和语言模型都在你的 Mac 上，录音和文字不会离开电脑，不需要任何 API Key。
> · 更快更轻：边说边出字；长段口述分段处理，几分钟的录音松开快捷键几秒就出结果。
> · 保留你常用的：转录、翻译、转写、应用增强和词典。会议、笔记、文件转写和远程模型不再包含。
> · 只给最合适的几个模型，参数已经调好，下载即用。
>
> VoxtMini 是一个独立的新 App，Voxt 不会自动更新到它。你可以继续使用 Voxt，不受任何影响；也可以两个一起用（VoxtMini 默认快捷键是右 Option）。
>
> 下载地址：github.com/hehehai/VoxtMini/releases

**Title (en):** Meet VoxtMini, the next Voxt

**Content (en):**

> We rebuilt Voxt from scratch as a new app: VoxtMini.
>
> · Fully local: speech recognition and the language models run on your Mac. Recordings and text never leave it, and no API key is needed.
> · Faster and lighter: text appears as you speak, and a few minutes of dictation is ready seconds after you let go of the shortcut.
> · The parts you use: Transcribe, Translate, Rewrite, App enhancement and the Dictionary. Meetings, notes, file transcription and remote models are not included.
> · A few best models, already tuned: download and go.
>
> VoxtMini is a separate app, and Voxt won't update to it. Keep using Voxt as before, or use both (VoxtMini's default shortcut is Right Option).
>
> Download: github.com/hehehai/VoxtMini/releases

**Version:** the Voxt version the notice is for (the panel's `version` field), e.g. the current release.

Publish it only when a notarised VoxtMini release is on GitHub Releases.

## Open

- **Importing Voxt's data** (dictionary, history, app enhancement groups) is not built. Voxt is sandboxed (`~/Library/Containers/com.voxt.Voxt`), so VoxtMini would need the user's consent to read another app's data (macOS asks). Decide whether it's worth it.
- The name "VoxtMini" is still provisional on actnow.dev; a rename changes the bundle name, the docs and this notice.
