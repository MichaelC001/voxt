package main

// Hiding models and remote services (docs/models-page.md): a hidden one
// is left out of the Models page (until Show hidden) and of the features'
// model selects, but stays installed or set up, and a feature that uses
// it keeps it. The keys are settings.Models.Hidden's: a local model's
// catalogue id, a remote service's settings.HiddenRemote.

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// hideMenu adds Hide, or Show when it is hidden, to the menu of the model
// or service of key, named name.
func (a *app) hideMenu(m *ui.Menu, key, name string) {
	if a.prefs.Models.IsHidden(key) {
		if m.Item(i18n.T("Show")).Chosen() {
			a.setHidden(key, name, false)
		}
		return
	}
	if m.Item(i18n.T("Hide")).Chosen() {
		a.setHidden(key, name, true)
	}
}

// setHidden hides or shows the model or service of key; hiding says
// which features still use it.
func (a *app) setHidden(key, name string, on bool) {
	a.updateSettings(func(s *settings.Settings) { s.Models.Hidden = s.Models.SetHidden(key, on) })
	if !on {
		return
	}
	if users := a.modelUsers(key); len(users) > 0 {
		a.toast(i18n.T("%s is hidden. It stays in use for %s until you choose another model there.", name, uitext.Names(users)))
		return
	}
	a.toast(i18n.T("%s is hidden. Show hidden on the Models page brings it back.", name))
}

// modelUsers names the features that use the model or service of key
// now (chosen or Automatic's pick).
func (a *app) modelUsers(key string) []string {
	var out []string
	for _, f := range []string{hotkey.Transcribe, hotkey.Translate, hotkey.Rewrite} {
		for _, kind := range []local.Kind{local.ASR, local.LLM} {
			if kind == local.LLM && f == hotkey.Transcribe && !a.prefs.Features.Transcribe.Enhance {
				continue // no clean-up: Transcribe uses no language model
			}
			if m, ok := a.featureModel(kind, f); ok && modelKey(m) == key {
				out = append(out, uitext.FeatureTitle(f))
				break
			}
		}
	}
	return out
}

// modelKey is a feature's model's Hidden key.
func modelKey(m local.Model) string {
	if id, ok := settings.IsRemote(m.ID); ok {
		return settings.HiddenRemote(string(m.Kind), id)
	}
	return m.ID
}

// shownModels are the models of list a feature's select offers: not the
// hidden ones, but its current choice want (hidden or not).
func (a *app) shownModels(list []local.Model, want string) []local.Model {
	want = strings.TrimSpace(want)
	var out []local.Model
	for _, m := range list {
		if m.ID == want || !a.prefs.Models.IsHidden(modelKey(m)) {
			out = append(out, m)
		}
	}
	return out
}
