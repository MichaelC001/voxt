package main

// The Models page's view (/models, docs/models-page.md; Voxt's model
// settings): a switch between speech and language models, the filter
// chips (All, Local, Remote, and Show hidden while any is hidden), then
// the local models, recommended first (models_cards.go), and the remote
// services (remote_page.go's cards). Routes: /models/speech,
// /models/language, /models/local, /models/remote, and the Remote models
// page's /remote, open it on that tab or filter (modelsRoute).

import (
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// The Models page's filters (modelsState.mdFilter).
const (
	filterAll = iota
	filterLocal
	filterRemote
)

// modelsRoute is where path goes when it is one of the Models page's
// routes: the page, on the tab or filter the path names; false for any
// other path.
func (a *app) modelsRoute(path string) (string, bool) {
	switch path {
	case modelsPath:
	case modelsPath + "/speech":
		a.mdTab = 0
	case modelsPath + "/language":
		a.mdTab = 1
	case modelsPath + "/local":
		a.mdFilter = filterLocal
	case remotePath, modelsPath + "/remote":
		a.mdFilter = filterRemote
	default:
		return "", false
	}
	return modelsPath, true
}

// modelsList is what one tab of the page lists, as the filter and the
// hidden models leave it (memory only).
type modelsList struct {
	rec, more     []models.Model     // the local models: recommended, the others
	apples        []string           // Apple's models (apple.SpeechID…), models_apple.go
	mine, rest    []providers.Preset // the remote services: set up, not
	local, remote int                // how many of each are not hidden
	hidden        int                // how many are hidden
}

// modelsKinds are the tab's kinds of model and of remote service.
func (a *app) modelsKinds() (models.Kind, providers.Kind) {
	if a.mdTab == 1 {
		return models.LLM, providers.LLM
	}
	return models.ASR, providers.ASR
}

// listModels is the tab's list: hidden entries only with Show hidden.
func (a *app) listModels() modelsList {
	kind, pkind := a.modelsKinds()
	var l modelsList
	show := func(hidden bool) bool {
		if hidden {
			l.hidden++
		}
		return !hidden || a.mdShowHidden
	}
	for _, m := range models.Catalog() {
		if m.Kind != kind || m.Borrowed && !a.modelStatus[m.ID].OnDisk { // another app's: only when it is on this Mac
			continue
		}
		hidden := a.prefs.Models.IsHidden(m.ID)
		if !hidden {
			l.local++
		}
		if !show(hidden) || a.mdFilter == filterRemote {
			continue
		}
		if m.Recommended {
			l.rec = append(l.rec, m)
		} else {
			l.more = append(l.more, m)
		}
	}
	for _, id := range a.appleIDs() { // models_apple.go
		hidden := a.prefs.Models.IsHidden(id)
		if !hidden {
			l.local++
		}
		if show(hidden) && a.mdFilter != filterRemote {
			l.apples = append(l.apples, id)
		}
	}
	for _, pr := range providers.Presets(pkind) {
		hidden := a.prefs.Models.IsHidden(settings.HiddenRemote(string(pkind), pr.ID))
		if !hidden {
			l.remote++
		}
		if !show(hidden) || a.mdFilter == filterLocal {
			continue
		}
		if _, ok := providers.Setup(a.prefs, pkind, pr.ID); ok {
			l.mine = append(l.mine, pr)
		} else {
			l.rest = append(l.rest, pr)
		}
	}
	return l
}

func (a *app) modelsPage(c *ui.Context) {
	t := c.Theme()
	pageHeaderTip(c, i18n.T("Models"), i18n.T("Local models run on this Mac. A remote service gets your audio or text only when you choose it for a feature; its key stays in the Keychain."), func() {
		if ghostButton(c.Key("models-network"), i18n.T("Network")).Tooltip(i18n.T("How model downloads and remote services reach the internet")).Clicked() {
			a.openNetwork()
		}
	})
	_, pkind := a.modelsKinds()
	ui.Scroll(c.Key("models-scroll")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(0, px(t, sp6), px(t, sp6)).Gap(px(t, sp3)).MaxWidth(px(t, 880)).Children(func() {
			l := a.listModels()
			a.modelsToolbar(c, l)
			if len(l.rec)+len(l.apples)+len(l.more)+len(l.mine)+len(l.rest) == 0 {
				emptyState(c, icons.EyeClosed, i18n.T("Every model here is hidden"), i18n.T("Show the hidden models to bring one back."), nil)
			}
			if len(l.rec) > 0 {
				sectionTitleTip(c, i18n.T("Recommended"), a.modelsTip())
				for _, m := range l.rec {
					a.modelCard(c, m)
				}
			}
			if len(l.apples) > 0 {
				sectionTitleTip(c, i18n.T("Built into macOS"), i18n.T("Apple's own models: macOS provides them, so there is nothing to download from VoxtMini, and they run on this Mac."))
				for _, id := range l.apples {
					a.appleCard(c, id) // models_apple.go
				}
			}
			if len(l.more) > 0 {
				sectionTitleTip(c, i18n.T("More models"), i18n.T("More local models. Experimental ones are not yet checked on a Mac."))
				for _, m := range l.more {
					a.modelCard(c, m)
				}
			}
			if len(l.mine)+len(l.rest) > 0 {
				a.providerCards(c, pkind, l.mine, l.rest) // remote_page.go
			}
			if a.mdFilter != filterRemote {
				a.modelsAdvanced(c) // models_keep.go
			}
		})
	})
	a.modelDeleteDialogs(c)  // models_delete.go
	a.networkDialog(c)       // models_network.go
	a.providerDialogView(c)  // remote_dialog.go
	a.modelSettingsDialog(c) // models_settings_view.go
}

// modelsTip is the ⓘ of the Recommended section: what the tab's models do.
func (a *app) modelsTip() string {
	if a.mdTab == 1 {
		return i18n.T("Cleans up, translates and rewrites the text. Without one, Transcribe types the text as recognised.")
	}
	return i18n.T("Turns what you say into text. Every feature needs one to work by voice.")
}

// modelsToolbar is the page's one toolbar row: the speech / language
// switch on the left, the filter's chips on the right (All, Local,
// Remote with their counts, and Show hidden while the tab hides any).
// In a narrow window the chips wrap under the switch, still on the right.
func (a *app) modelsToolbar(c *ui.Context, l modelsList) {
	t := c.Theme()
	ui.Row(c.Key("models-toolbar")).Wrap().GapX(px(t, sp3)).GapY(px(t, sp2)).AlignItems(ui.Center).Children(func() {
		ui.Segmented(c.Key("models-tab"), &a.mdTab, i18n.T("Speech models"), i18n.T("Language models")).Label(i18n.T("Kind of model")).Shrink(0)
		a.modelsFilterChips(c, l)
	})
}

// modelsFilterChips is the filter's chips, as one group.
func (a *app) modelsFilterChips(c *ui.Context, l modelsList) {
	t := c.Theme()
	ui.Row(c.Key("models-filter")).Gap(px(t, sp1)).AlignItems(ui.Center).Wrap().Justify(ui.End).Grow(1).Shrink(1).MinWidth(0).Children(func() {
		for i, label := range []string{i18n.T("All"), i18n.T("Local"), i18n.T("Remote")} {
			n := []int{l.local + l.remote, l.local, l.remote}[i]
			if chip(c.Key("models-filter-"+itoa(i)), label, itoa(n), a.mdFilter == i).Clicked() {
				a.mdFilter = i
			}
		}
		if l.hidden > 0 {
			if chip(c.Key("models-show-hidden"), i18n.T("Show hidden"), itoa(l.hidden), a.mdShowHidden).
				Tooltip(i18n.T("Hidden models are left out of this list and of the features' model choices. Their files stay on this Mac.")).Clicked() {
				a.mdShowHidden = !a.mdShowHidden
			}
		}
	})
}
