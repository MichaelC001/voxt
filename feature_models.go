package main

// A feature page's Models section and Transcribe's clean-up layers
// (feature_page.go).

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// ---- Models ----

func (a *app) featureModels(c *ui.Context, feature string) {
	t := c.Theme()
	p := uitext.FeaturePrefix(feature)
	u := a.fui(feature)
	f := featureOf(a.prefs, feature)
	enhance := feature != hotkey.Transcribe || a.prefs.Features.Transcribe.Enhance
	// The installed local models, then the ready remote services
	// (internal/remote): a remote one only when the user picks it; not
	// those hidden on the Models page, but the current choice
	// (models_hide.go).
	allA, allL := a.installed(local.ASR), local.ForFeature(a.installed(local.LLM), feature)
	readyA, readyL := a.shownModels(allA, f.ASR), a.shownModels(allL, f.LLM)
	optsA, autoA := modelOptions(readyA, allA)
	optsL, autoL := modelOptions(readyL, allL)
	seen := strings.Join([]string{f.ASR, f.LLM, strings.Join(optsA, "\x00"), strings.Join(optsL, "\x00")}, "\x01")
	if u.svcSeen != seen {
		u.svcSeen = seen
		u.asr, u.llm = modelLabel(readyA, f.ASR, autoA), modelLabel(readyL, f.LLM, autoL)
	}
	sectionTitleTip(c, i18n.T("Models"), i18n.T("Automatic picks the best installed local model. A remote service is used only if you choose it here."))
	opened := false // one "Open Models" per page
	openModels := func() {
		if opened {
			return
		}
		opened = true
		if ui.Button(c.Key(p+"-open-models"), i18n.T("Open Models")).Clicked() {
			a.navigate(modelsPath)
		}
	}
	mA, okA := local.Choose(allA, f.ASR)
	mL, okL := local.Choose(allL, f.LLM)
	if !okA {
		switch feature {
		case hotkey.Translate:
			notice(c, p+"-no-asr", noticeInfo, i18n.T("No speech model: only selected text can be translated."), "", openModels, nil)
		case hotkey.Rewrite:
			notice(c, p+"-no-asr", noticeWarn, i18n.T("No speech model installed."), "", openModels, nil)
		default:
			notice(c, p+"-no-asr", noticeWarn, i18n.T("No speech model installed."), "", openModels, nil)
		}
	}
	if !okL {
		switch feature {
		case hotkey.Translate:
			notice(c, p+"-no-llm", noticeWarn, i18n.T("No language model installed."), "", openModels, nil)
		case hotkey.Rewrite:
			notice(c, p+"-no-llm", noticeWarn, i18n.T("No language model installed."), "", openModels, nil)
		}
	}
	var asrDesc, asrLabel, llmDesc, llmLabel string
	switch feature {
	case hotkey.Translate:
		asrDesc, asrLabel = i18n.T("Turns what you say into text, when nothing is selected."), i18n.T("Speech model for Translate")
		llmDesc, llmLabel = i18n.T("Translates the text."), i18n.T("Language model for Translate")
	case hotkey.Rewrite:
		asrDesc, asrLabel = i18n.T("Turns your spoken instruction into text."), i18n.T("Speech model for Rewrite")
		llmDesc, llmLabel = i18n.T("Rewrites the selection, or writes what you ask for."), i18n.T("Language model for Rewrite")
	default:
		asrDesc, asrLabel = i18n.T("Turns your recording into text."), i18n.T("Speech model for Transcribe")
		llmDesc, llmLabel = i18n.T("Cleans up the recognised text when \"Clean up with a language model\" is on."), i18n.T("Language model for Transcribe")
	}
	settingRow(c, p+"-asr", i18n.T("Speech recognition"), asrDesc, func() {
		sel := ui.Select(c.Key(p+"-asr-sel"), &u.asr, optsA).Label(asrLabel).Width(px(t, 260)).Shrink(1).MinWidth(px(t, 160)).Disabled(len(allA) == 0)
		if sel.Changed() {
			id := modelID(optsA, readyA, u.asr)
			a.updateSettings(func(s *settings.Settings) { setFeatureModel(s, local.ASR, feature, id) })
		}
	})
	if okA && mA.Remote {
		notice(c, p+"-remote-asr", noticeInfo, i18n.T("Your recordings are sent to %s to be recognised.", modelChoice(mA)), "", nil, nil)
	}
	settingRow(c, p+"-llm", i18n.T("Language model"), llmDesc, func() {
		sel := ui.Select(c.Key(p+"-llm-sel"), &u.llm, optsL).Label(llmLabel).Width(px(t, 260)).Shrink(1).MinWidth(px(t, 160)).
			Disabled(len(allL) == 0 || !enhance)
		if sel.Changed() {
			id := modelID(optsL, readyL, u.llm)
			a.updateSettings(func(s *settings.Settings) { setFeatureModel(s, local.LLM, feature, id) })
		}
	})
	if okL && mL.Remote && enhance {
		notice(c, p+"-remote-llm", noticeInfo, i18n.T("Recognised and selected text is sent to %s.", modelChoice(mL)), "", nil, nil)
	}
	if feature == hotkey.Transcribe && !okL && enhance {
		notice(c, "tr-no-llm", noticeInfo, i18n.T("No language model: the text is typed as recognised."), "", openModels, nil)
	}
}

// modelChoice is how a select names a model: a local model's name, a
// remote service's name and model.
func modelChoice(m local.Model) string {
	if m.Remote {
		return i18n.T("%s · %s (remote)", m.Name, m.Detail)
	}
	return m.Name
}

// modelOptions are a model select's labels: "Automatic: X" (the
// recommended local model of all, hidden or not: hiding changes no
// choice), then the models of ready (the installed models and the ready
// remote services the select offers).
func modelOptions(ready, all []local.Model) (opts []string, auto string) {
	auto = i18n.T("Automatic")
	if m, ok := local.Choose(all, ""); ok {
		auto = i18n.T("Automatic: %s", m.Name)
	}
	opts = []string{auto}
	for _, m := range ready {
		opts = append(opts, modelChoice(m))
	}
	return opts, auto
}

// modelLabel is the select's label of the model id ("" automatic).
func modelLabel(ready []local.Model, id, auto string) string {
	for _, m := range ready {
		if m.ID == id {
			return modelChoice(m)
		}
	}
	return auto
}

// modelID is the id a select's label stands for, "" for automatic.
func modelID(opts []string, ready []local.Model, label string) string {
	if j := slices.Index(opts, label); j > 0 {
		return ready[j-1].ID
	}
	return ""
}

// ---- Clean-up ----

func (a *app) trEnhance(c *ui.Context) {
	sectionTitle(c, i18n.T("Clean-up"))
	f := &a.prefs.Features.Transcribe
	settingRow(c, "tr-enhance", i18n.T("Clean up with a language model"),
		i18n.T("Sends the recognised text to the language model with the prompt below, and types its answer. Off, the text is typed as recognised. If the model fails or is slow, the recognised text is typed."), func() {
			if ui.Switch(c.Key("tr-enhance-switch"), &f.Enhance).Label(i18n.T("Clean up with a language model")).Changed() {
				on := f.Enhance
				a.updateSettings(func(s *settings.Settings) { s.Features.Transcribe.Enhance = on })
			}
		})
	settingRow(c, "tr-cleanup", i18n.T("Basic clean-up"),
		i18n.T("Removes fillers and false starts, keeps your corrections, fixes punctuation and capitals, and turns spoken punctuation (\"comma\", \"new line\") into marks."), func() {
			if ui.Switch(c.Key("tr-cleanup-switch"), &f.Cleanup).Label(i18n.T("Basic clean-up")).Disabled(!f.Enhance).Changed() {
				on := f.Cleanup
				a.updateSettings(func(s *settings.Settings) { s.Features.Transcribe.Cleanup = on })
			}
		})
	settingRow(c, "tr-structure", i18n.T("Structure"),
		i18n.T("Makes a list when you clearly say steps or several items, and splits a long text into paragraphs. Off, the text keeps the form you said it in."), func() {
			if ui.Switch(c.Key("tr-structure-switch"), &f.Structure).Label(i18n.T("Structure")).Disabled(!f.Enhance).Changed() {
				on := f.Structure
				a.updateSettings(func(s *settings.Settings) { s.Features.Transcribe.Structure = on })
			}
		})
}
