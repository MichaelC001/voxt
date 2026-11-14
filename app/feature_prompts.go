package main

// Translate's and Rewrite's own settings and every feature's prompt, with
// the preview of the whole prompt (feature_page.go).

import (
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/compose"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// trySample is the input the prompt preview shows.
func trySample() string {
	return i18n.T("um so the meeting moves to Thursday at three, no, at four")
}

// ---- Translate and Rewrite ----

// translateSettings are Translate's own: the target language, the
// clean-up of speech, the card.
func (a *app) translateSettings(c *ui.Context) {
	t := c.Theme()
	u := a.fui(hotkey.Translate)
	f := &a.prefs.Features.Translate
	opts := []string{i18n.T("Automatic: %s", uitext.TargetLanguageLabel(session.TranslateTarget(settings.Settings{General: a.prefs.General}, i18n.Locale(), "")))}
	for _, code := range compose.TargetLanguages {
		opts = append(opts, uitext.TargetLanguageLabel(code))
	}
	cur := opts[0]
	if j := slices.Index(compose.TargetLanguages, f.Target); j >= 0 {
		cur = opts[j+1]
	} else if f.Target != "" {
		cur = uitext.TargetLanguageLabel(f.Target)
		opts = append(opts, cur)
	}
	u.lang = cur // the settings say; a choice saves at once
	settingRow(c, "tl-target", i18n.T("Translate into"),
		i18n.T("Automatic is English, or Chinese when your main language is English (Settings → General). The answer card can translate the same text into another language."), func() {
			if ui.Select(c.Key("tl-target-sel"), &u.lang, opts).Label(i18n.T("Target language")).Width(px(t, 260)).Shrink(1).MinWidth(px(t, 160)).Changed() {
				code := ""
				if j := slices.Index(opts, u.lang); j > 0 && j <= len(compose.TargetLanguages) {
					code = compose.TargetLanguages[j-1]
				}
				a.updateSettings(func(s *settings.Settings) { s.Features.Translate.Target = code })
			}
		})
	settingRow(c, "tl-cleanup", i18n.T("Basic clean-up"),
		i18n.T("Cleans up what you say before translating it: fillers, false starts and corrections go, spoken punctuation becomes marks. Selected text is translated as it is."), func() {
			if ui.Switch(c.Key("tl-cleanup-switch"), &f.Cleanup).Label(i18n.T("Basic clean-up")).Changed() {
				on := f.Cleanup
				a.updateSettings(func(s *settings.Settings) { s.Features.Translate.Cleanup = on })
			}
		})
	settingRow(c, "tl-card", i18n.T("Show the result in a card"),
		i18n.T("Shows the translation in the answer card, to read, copy or insert, instead of typing it (or replacing the selection). Selected text that cannot be edited, such as a web page, always goes to the card."), func() {
			if ui.Switch(c.Key("tl-card-switch"), &f.ShowCard).Label(i18n.T("Show the result in a card")).Changed() {
				on := f.ShowCard
				a.updateSettings(func(s *settings.Settings) { s.Features.Translate.ShowCard = on })
			}
		})
}

// rewriteSettings are Rewrite's own: the instruction's clean-up. Every
// answer shows in the answer card (Voxt's), nothing to choose there.
func (a *app) rewriteSettings(c *ui.Context) {
	f := &a.prefs.Features.Rewrite
	settingRow(c, "rw-cleanup", i18n.T("Clean up the instruction"),
		i18n.T("Reads past fillers, false starts and corrections in what you say (\"shorter, no, more formal\"). It never changes the selected text by itself."), func() {
			if ui.Switch(c.Key("rw-cleanup-switch"), &f.Cleanup).Label(i18n.T("Clean up the instruction")).Changed() {
				on := f.Cleanup
				a.updateSettings(func(s *settings.Settings) { s.Features.Rewrite.Cleanup = on })
			}
		})
}

// ---- The prompt ----

// variableInfo says what a prompt variable holds.
func variableInfo(name string) string {
	switch name {
	case "USER_MAIN_LANGUAGE":
		return i18n.T("The language you mostly speak (Settings → General).")
	case "USER_OTHER_LANGUAGES":
		return i18n.T("The other languages you mix in.")
	case "TARGET_LANGUAGE":
		return i18n.T("The language to translate into (Translate only).")
	case "APP_NAME":
		return i18n.T("The app in front (with App enhancement).")
	}
	return ""
}

// featureTask is the composer's task of a feature.
func featureTask(feature string) compose.Task {
	switch feature {
	case hotkey.Translate:
		return compose.Translate
	case hotkey.Rewrite:
		return compose.Rewrite
	}
	return compose.Transcribe
}

// promptOf is a feature's saved prompt ("" = the built-in one).
func promptOf(s settings.Settings, feature string) string {
	return featureOf(s, feature).Prompt
}

// defaultPrompt is the feature's built-in prompt in the main language's
// set, as the editor shows it.
func (a *app) defaultPrompt(feature string) string {
	return compose.DefaultPrompt(featureTask(feature), compose.TemplateSet(session.MainLanguage(a.prefs, i18n.Locale())))
}

// promptSync sets the editor from the settings when they changed
// elsewhere (or the main language, which picks the built-in prompt) and
// nothing typed waits to be saved.
func (a *app) promptSync(feature string) {
	u := a.fui(feature)
	saved := promptOf(a.prefs, feature)
	shown := saved
	if shown == "" {
		shown = a.defaultPrompt(feature)
	}
	if !u.promptDirty && shown != u.promptSeen {
		u.prompt, u.promptSeen = shown, shown
	}
}

// savePrompt saves the prompt as typed: the built-in one (in either
// language) or nothing is saved as "", which follows the main language.
func (a *app) savePrompt(feature string) {
	u := a.fui(feature)
	u.promptDirty = false
	v := strings.TrimSpace(u.prompt)
	if compose.IsDefault(featureTask(feature), v) {
		v = ""
	}
	if v == "" {
		u.prompt = a.defaultPrompt(feature)
	}
	// What promptSync compares with: the saved prompt as it will show, so
	// the text as typed (and its caret) stays.
	u.promptSeen = v
	if v == "" {
		u.promptSeen = u.prompt
	}
	if v == promptOf(a.prefs, feature) {
		return
	}
	a.updateSettings(func(s *settings.Settings) {
		switch feature {
		case hotkey.Translate:
			s.Features.Translate.Prompt = v
		case hotkey.Rewrite:
			s.Features.Rewrite.Prompt = v
		default:
			s.Features.Transcribe.Prompt = v
		}
	})
}

// insertVariable puts token at the editor's last caret (its end when it
// was never focused), replacing a selection, and saves.
func (a *app) insertVariable(feature, token string) {
	u := a.fui(feature)
	r := []rune(u.prompt)
	start, end := len(r), len(r)
	if u.caretSet {
		start, end = min(u.caret[0], len(r)), min(u.caret[1], len(r))
		if start > end {
			start, end = end, start
		}
	}
	u.prompt = string(r[:start]) + token + string(r[end:])
	at := start + len([]rune(token))
	u.caret, u.caretSet = [2]int{at, at}, true
	a.savePrompt(feature)
}

// promptSaveDelay is how long the prompt waits after the last key before
// it is saved (it is saved at once when the field loses focus).
const promptSaveDelay = 800 * time.Millisecond

// featurePrompt is the feature's task prompt, the user's to edit (it
// replaces the built-in one; the rest of the request stays the app's,
// which the full prompt below shows), with the variables to insert.
func (a *app) featurePrompt(c *ui.Context, feature string) {
	t := c.Theme()
	k := tok(t)
	p := uitext.FeaturePrefix(feature)
	u := a.fui(feature)
	a.promptSync(feature)
	sectionTitleTip(c, i18n.T("Prompt"), i18n.T("Tells the model what to do. VoxtMini adds the rest itself: what the input is, your dictionary, App enhancement's scene and the output format."))
	in := ui.TextArea(c.Key(p+"-prompt"), &u.prompt).Label(i18n.T("Prompt for %s", uitext.FeatureTitle(feature))).MinHeight(px(t, 200))
	if in.Focused() {
		s, e := in.TextSelection()
		u.caret, u.caretSet = [2]int{s, e}, true
	}
	if in.Changed() {
		u.promptDirty = true
		u.promptSeq++
		if seq := u.promptSeq; a.win != nil {
			time.AfterFunc(promptSaveDelay, func() {
				a.post(func() {
					if u.promptDirty && u.promptSeq == seq {
						a.savePrompt(feature)
					}
				})
			})
		}
	} else if u.promptDirty && !in.Focused() {
		a.savePrompt(feature) // it lost the focus
	}
	ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Children(func() {
		// The variables as chips, inserted at the caret; what each holds
		// is its tooltip.
		ui.Row(c.Key(p + "-vars")).Wrap().Gap(px(t, 6)).Grow(1).MinWidth(0).Children(func() {
			for _, name := range compose.TaskVariables(featureTask(feature)) {
				token := "{{" + name + "}}"
				ui.Row(c.Key("var-" + name)).AlignItems(ui.Center).Children(func() {
					b := ui.ButtonBase(c.Key("insert")).Padding(px(t, 2), px(t, 6)).Radius(px(t, radiusChip)).Background(k.Hover).Label(i18n.T("Insert %s", token)).Tooltip(variableInfo(name)).Shrink(0)
					if b.Hovered() {
						b.Background(k.Pressed)
					}
					b.Children(func() {
						ui.Text(c, token).FontSize(px(t, fsCaption)).Font("monospace").SingleLine()
					})
					if b.Clicked() {
						a.insertVariable(feature, token)
					}
				})
			}
		})
		custom := promptOf(a.prefs, feature) != "" || u.promptDirty && !compose.IsDefault(featureTask(feature), u.prompt)
		if ghostIconButton(c.Key(p+"-prompt-reset"), icons.Refresh, i18n.T("Reset to default")).Disabled(!custom).Shrink(0).Clicked() {
			u.prompt = ""
			a.savePrompt(feature)
		}
	})
	collapsible(c.Key(p+"-preview"), i18n.T("Show the full prompt"), &u.preview, nil, func() {
		a.previewBody(c, feature)
	})
}

// rewriteSample is the instruction and the selection Rewrite's preview
// shows.
func rewriteSample() (instruction, selection string) {
	return i18n.T("um make it shorter, no, more formal"), i18n.T("hey, the release slips to thursday because the tests are still running, sorry!")
}

// composedPreview is the request a session would send with the settings
// and the prompt as typed, for the sample text.
func (a *app) composedPreview(feature string) (compose.Output, error) {
	g := a.prefs.General
	prompt := a.fui(feature).prompt
	if compose.IsDefault(featureTask(feature), prompt) {
		prompt = ""
	}
	in := compose.Input{
		TaskPrompt: prompt, MainLanguage: session.MainLanguage(a.prefs, i18n.Locale()), OtherLanguages: g.OtherLanguages,
	}
	switch feature {
	case hotkey.Translate:
		f := a.prefs.Features.Translate
		in.Task, in.Cleanup, in.Text = compose.Translate, f.Cleanup, trySample()
		in.TargetLanguage = session.TranslateTarget(a.prefs, i18n.Locale(), "")
	case hotkey.Rewrite:
		in.Task, in.Cleanup = compose.Rewrite, a.prefs.Features.Rewrite.Cleanup
		in.Instruction, in.Text = rewriteSample()
	default:
		f := a.prefs.Features.Transcribe
		in.Task, in.Cleanup, in.Structure, in.Text = compose.Transcribe, f.Cleanup, f.Structure, trySample()
	}
	return compose.Build(in)
}

func (a *app) previewBody(c *ui.Context, feature string) {
	t := c.Theme()
	k := tok(t)
	p := uitext.FeaturePrefix(feature)
	switch feature {
	case hotkey.Transcribe:
		if !a.prefs.Features.Transcribe.Enhance {
			caption(c, i18n.T("Clean-up is off: nothing is sent to a language model."))
		}
	case hotkey.Translate:
		caption(c, i18n.T("Spoken text, as below; selected text is translated without the clean-up and keeps its form."))
	case hotkey.Rewrite:
		caption(c, i18n.T("With a selection, as below; with nothing selected the model writes from the instruction and answers with a title and the text."))
	}
	out, err := a.composedPreview(feature)
	if err != nil {
		caption(c, err.Error())
		return
	}
	caption(c, i18n.T("Your dictionary's terms and App enhancement's scene are added when they apply."))
	block := func(key, title, text string) {
		ui.Column(c.Key(key)).Gap(px(t, 4)).Children(func() {
			ui.Text(c, title).FontSize(px(t, fsCaption)).FontWeight(600).TextColor(k.Text2)
			fillCard(c).Padding(px(t, sp3)).Children(func() {
				ui.Text(c, text).FontSize(px(t, fsCaption)).Font("monospace").Selectable()
			})
		})
	}
	block(p+"-preview-system", i18n.T("System prompt"), out.System)
	block(p+"-preview-user", i18n.T("User message (an example)"), out.User)
}
