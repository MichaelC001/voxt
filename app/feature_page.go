package main

// The feature pages: Transcribe (/transcribe), Translate
// (/translate) and Rewrite (/rewrite) share one structure: the feature's
// shortcuts (and, on Transcribe, the shared "paste the last result"),
// Translate's and Rewrite's own settings (Translate's target language,
// clean-up and card; Rewrite's instruction clean-up), the models it uses,
// Transcribe's clean-up layers, the user's rules with a preview of the
// whole prompt, and a Try area that runs a session whose result stays on
// the page. "Add a shortcut" is in the header.
//
// Every change goes through a.updateSettings, which also sends the
// shortcuts to the helper again. A page keeps only what its controls
// edit (the selects' labels, a custom shortcut being typed, the rules
// before they are saved), set again from the settings when they change
// elsewhere; each feature has its own (featureUI).

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/store"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// transcribeState is the feature pages' view state, one featureUI per
// feature (hotkey.Transcribe, Translate, Rewrite).
type transcribeState struct {
	features map[string]*featureUI
}

// featureUI is a feature page's view state.
type featureUI struct {
	// rows are the shortcuts' controls, paste paste-last's (Transcribe);
	// seen is what they were last set from.
	rows  []keyRow
	paste keyRow
	seen  string
	// asr and llm are the models' selects; svcSeen what they were
	// last set from.
	asr, llm string
	svcSeen  string
	// lang is Translate's target select.
	lang string
	// prompt is the task prompt as typed; promptSeen what it was last
	// set from (the saved prompt, or the built-in one); promptDirty an
	// edit not saved yet, promptSeq which edit (its timer saves only the
	// last one); caret the editor's last selection, in runes, where a
	// variable goes (caretSet once it was focused).
	prompt      string
	promptSeen  string
	promptDirty bool
	promptSeq   int
	caret       [2]int
	caretSet    bool
	preview     bool
	// block says why the Try area's Start did not start.
	block *sessionNotice
}

// fui is feature's page state.
func (a *app) fui(feature string) *featureUI {
	if a.features == nil {
		a.features = map[string]*featureUI{}
	}
	u := a.features[feature]
	if u == nil {
		u = &featureUI{}
		a.features[feature] = u
	}
	return u
}

// featureKind is a feature's session kind.
func featureKind(feature string) store.Kind {
	switch feature {
	case hotkey.Translate:
		return store.KindTranslate
	case hotkey.Rewrite:
		return store.KindRewrite
	}
	return store.KindTranscribe
}

// ---- The page ----

// transcribePage builds the Transcribe page.
func (a *app) transcribePage(c *ui.Context) { a.featurePage(c, hotkey.Transcribe) }

// translatePage builds the Translate page.
func (a *app) translatePage(c *ui.Context) { a.featurePage(c, hotkey.Translate) }

// rewritePage builds the Rewrite page.
func (a *app) rewritePage(c *ui.Context) { a.featurePage(c, hotkey.Rewrite) }

// featureTip is the ⓘ beside a feature page's title: how it works, and
// what its shortcuts do besides (the page has no Shortcuts title of its
// own: its first card is the shortcuts, whose triggers tell their own
// tooltips).
func (a *app) featureTip(feature string) string {
	var tip string
	switch feature {
	case hotkey.Translate:
		tip = i18n.T("Select text and press the shortcut to translate it in place. With nothing selected, speak: the translation is typed at your cursor.")
	case hotkey.Rewrite:
		tip = i18n.T("Select text and say how to change it (\"shorter\", \"more formal\"), or with nothing selected say what to write. The answer is typed at your cursor.") +
			" " + i18n.T("While the answer card shows an answer, the shortcut records a follow-up that changes it (\"shorter\", \"in English\").")
	default:
		tip = i18n.T("Press a shortcut anywhere and speak: the text is typed at your cursor.")
	}
	if !a.prefs.Features.EscapeOff { // Settings → Shortcuts → Escape cancels (settings_recording.go)
		tip += " " + i18n.T("Esc cancels a recording.")
	}
	return tip
}

// featurePage builds a feature's page.
func (a *app) featurePage(c *ui.Context, feature string) {
	t := c.Theme()
	p := uitext.FeaturePrefix(feature)
	pageHeaderTip(c, uitext.FeatureTitle(feature), a.featureTip(feature), func() { a.featureAddKey(c, feature) })
	if a.sess.Try && a.sess.State == session.Recording {
		c.After(250 * time.Millisecond) // the Try area's clock
	}
	ui.Scroll(c.Key(p + "-scroll")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(0, px(t, sp6), px(t, sp6)).Gap(px(t, sp3)).MaxWidth(px(t, 880)).Children(func() {
			// No titles over the shortcuts and the feature's own settings,
			// which would only repeat the page's (翻译, 转写): they are its
			// first card and the rows under it, before the Models section.
			a.featureShortcuts(c, feature)
			switch feature {
			case hotkey.Translate:
				a.translateSettings(c)
			case hotkey.Rewrite:
				a.rewriteSettings(c)
			}
			a.featureModels(c, feature)
			if feature == hotkey.Transcribe {
				a.trEnhance(c) // its Clean-up section: the language model's work
			}
			a.featurePrompt(c, feature)
			a.featureTry(c, feature)
		})
	})
}
