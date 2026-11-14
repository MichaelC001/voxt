package main

// Settings → Shortcuts, "While recording" (settings_shortcuts.go): whether
// Escape cancels (settings.Features.EscapeOff) and the voice end command
// (settings.EndCommand: a word that stops the recording, internal/session
// endcmd.go): its switch, then its word, one of session.EndWords or typed.

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// loadRecordingForm sets the section's controls from the settings. A word
// being typed (endCustom) stays so until a preset is chosen.
func (a *app) loadRecordingForm() {
	f := &a.settingsForm
	s := a.prefs
	f.escCancels = !s.Features.EscapeOff
	f.endCommand = s.Features.EndCommand.On
	word := session.EndWord(s, i18n.Locale())
	if !f.endCustom && !slices.Contains(session.EndWords, word) {
		f.endCustom = true
	}
	if !f.endCustom {
		f.endChoice = word
		return
	}
	f.endChoice, f.endWord = i18n.T("Custom…"), word
}

// recordingSettings is the section.
func (a *app) recordingSettings(c *ui.Context) {
	t := c.Theme()
	f := &a.settingsForm
	sectionTitle(c, i18n.T("While recording"))
	settingRow(c, "esc-cancels", i18n.T("Escape cancels"), i18n.T("Escape cancels a recording and closes the answer card. Off, Escape goes to the app you are in."), func() {
		if ui.Switch(c.Key("esc-cancels-switch"), &f.escCancels).Label(i18n.T("Escape cancels")).Changed() {
			off := !f.escCancels
			a.updateSettings(func(s *settings.Settings) { s.Features.EscapeOff = off })
		}
	})
	settingRow(c, "end-command", i18n.T("Voice end command"), i18n.T("Say the end word, then pause: the recording stops by itself and the word is left out of the result. It stops by itself only with a speech model that shows text as you speak (not Whisper); with any model, the word at the end is left out."), func() {
		if ui.Switch(c.Key("end-command-switch"), &f.endCommand).Label(i18n.T("Voice end command")).Changed() {
			on := f.endCommand
			a.updateSettings(func(s *settings.Settings) { s.Features.EndCommand.On = on })
		}
	})
	if !f.endCommand {
		return
	}
	settingRow(c, "end-word", i18n.T("End word"), "", func() {
		custom := i18n.T("Custom…")
		opts := append(slices.Clone(session.EndWords), custom)
		if ui.Select(c.Key("end-word-select"), &f.endChoice, opts).Label(i18n.T("End word")).Width(px(t, 140)).Changed() {
			if f.endChoice == custom {
				f.endCustom = true
				f.endWord = session.EndWord(a.prefs, i18n.Locale())
				return
			}
			f.endCustom = false
			word := f.endChoice
			a.updateSettings(func(s *settings.Settings) { s.Features.EndCommand.Word = word })
		}
		if !f.endCustom {
			return
		}
		in := ui.TextInput(c.Key("end-word-input"), &f.endWord).Label(i18n.T("Your end word")).Placeholder("over").Width(px(t, 140))
		if in.Submitted() || in.Changed() && !in.Focused() {
			word := strings.TrimSpace(f.endWord)
			a.updateSettings(func(s *settings.Settings) { s.Features.EndCommand.Word = word })
		}
	})
}
