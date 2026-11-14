package main

// A local model's card on the Models page (models_view.go), with what
// the ActNow family says of it (family spec §7.10.2–§7.10.4): the
// other apps that share it, another app downloading it, the app that
// deleted it for all apps; and the models of other family apps VoxtMini
// can run (models.Model.Borrowed), shown once they are on this Mac.

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/uitext"
	"github.com/hehehai/actnowkit/hfhub"
	fammodels "github.com/hehehai/actnowkit/models"
)

// modelCard is one model: what it is good at, its languages, its size
// and state, and what can be done with it; dimmed while hidden.
func (a *app) modelCard(c *ui.Context, m models.Model) {
	t := c.Theme()
	k := tok(t)
	st := a.modelStatus[m.ID]
	hidden := a.prefs.Models.IsHidden(m.ID)
	ic := icons.Soundwave
	if m.Kind == models.LLM {
		ic = icons.ChatRoundLine
	}
	el := card(c.Key("model-" + m.ID))
	if hidden {
		el.Opacity(0.6)
	}
	el.ContextMenu(func(mn *ui.Menu) { a.hideMenu(mn, m.ID, uitext.ModelName(m.ID)) }) // models_hide.go
	el.Children(func() {
		ui.Row(c).Gap(px(t, sp3)).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, ic.Linear).Size(px(t, 20), px(t, 20)).TextColor(k.Text2).Shrink(0).Margin(px(t, 1), 0, 0, 0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, sp1)).Children(func() {
				ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Text(c, uitext.ModelName(m.ID)).FontWeight(600)
					modelChips(c, m, st, hidden)
				})
				if uitext.ModelName(m.ID) != m.Title { // the plain name of a recommended model: its own below
					caption(c.Key("title"), m.Title)
				}
				ui.Text(c, uitext.ModelAbout(m.ID))
				size, family := modelSizeText(m, st), modelFamilyText(m, st)
				if st.Downloading && st.WaitingFor != "" { // "Downloading in AudioSmith… 43%" says it all
					size, family = family, ""
				}
				if family != "" {
					caption(c.Key("family"), family)
				}
				ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Wrap().Children(func() {
					caption(c, size)
					if mem, ok := a.modelMem[m.ID]; ok && st.Installed { // models_memory.go
						ui.Row(c.Key("loaded")).Gap(px(t, 6)).AlignItems(ui.Center).Children(func() {
							statusDot(c, k.Accent)
							caption(c, modelMemText(mem))
						})
					}
				})
			})
			ui.Row(c.Key("actions")).Gap(px(t, sp2)).Shrink(0).AlignItems(ui.Center).Children(func() {
				a.modelActions(c, m, st)
				moreMenu(c.Key("more"), i18n.T("More actions for %s", uitext.ModelName(m.ID)), func(mn *ui.Menu) {
					a.hideMenu(mn, m.ID, uitext.ModelName(m.ID))
				})
			})
		})
		if st.Downloading {
			frac := 0.0
			if st.Total > 0 {
				frac = float64(st.Done) / float64(st.Total)
			}
			ui.Progress(c, frac).Label(i18n.T("Downloading %s", uitext.ModelName(m.ID)))
		}
		if st.Err != nil && !st.Downloading {
			msg, more := uitext.DownloadErrorText(st.Err)
			notice(c, "model-err", noticeError, msg, more, func() {
				if ui.Button(c.Key("retry"), i18n.T("Try again")).Clicked() {
					a.downloadModel(m.ID)
				}
			}, nil)
		}
	})
}

// modelChips are the chips after a model's name: Recommended,
// Experimental, Hidden, and Installed.
func modelChips(c *ui.Context, m models.Model, st models.Status, hidden bool) {
	k := tok(c.Theme())
	if m.Recommended {
		tagChip(c, i18n.T("Recommended"))
	}
	if m.Experimental {
		tagChip(c, i18n.T("Experimental")).Tooltip(i18n.T("VoxtMini should run it, but it was not yet checked on a Mac. Tell us how it works."))
	}
	if hidden {
		tagChip(c, i18n.T("Hidden"))
	}
	if st.Installed {
		statusMark(c, icons.CheckCircle, k.Success, i18n.T("Installed"))
	}
}

// modelSizeText is a model's size, or how much of it came.
func modelSizeText(m models.Model, st models.Status) string {
	size := uitext.HumanBytes(m.Size())
	switch {
	case st.Downloading:
		return i18n.T("Downloading: %s of %s (%s)", uitext.HumanBytes(st.Done), uitext.HumanBytes(st.Total), fmt.Sprintf("%d%%", percent(st.Done, st.Total)))
	case st.Installed || st.OnDisk:
		return i18n.T("%s on disk", size)
	case st.Done > 0 && st.Elsewhere == "":
		return i18n.T("Stopped: %s of %s downloaded", uitext.HumanBytes(st.Done), size)
	}
	return i18n.T("%s download", size)
}

// percent is done of total in whole percents (0 without a total).
func percent(done, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(done * 100 / total)
}

// modelFamilyText is what the family says of a model ("" nothing): the
// apps that installed a borrowed model VoxtMini does not use yet, the other
// apps that share it, another app downloading what VoxtMini's download
// waits for or the model itself, or the app that deleted it for all apps.
func modelFamilyText(m models.Model, st models.Status) string {
	switch {
	case (m.Borrowed || st.OnDisk) && !st.Installed:
		if len(st.Users) == 0 {
			return i18n.T("Already on this Mac")
		}
		return i18n.T("Installed by %s", uitext.Names(st.Users))
	case st.Installed && len(st.Users) > 0:
		return i18n.T("Shared with %s", uitext.Names(st.Users))
	case st.Downloading && st.WaitingFor != "":
		return i18n.T("Downloading in %s… %d%%", ownerName(st.WaitingFor), percent(st.Done, st.Total))
	case !st.Downloading && !st.OnDisk && st.Elsewhere != "":
		return i18n.T("Downloading in %s…", ownerName(st.Elsewhere))
	case !st.Downloading && !st.OnDisk && st.DeletedBy != "":
		return i18n.T("Deleted in %s", ownerName(st.DeletedBy))
	}
	return ""
}

// ownerName is the name of the family app of bundleID for a sentence:
// "AudioSmith", "another app" when the holder of a lock is unknown.
func ownerName(bundleID string) string {
	if bundleID == "" || bundleID == hfhub.UnknownOwner {
		return i18n.T("another app")
	}
	return actnow.AppName(bundleID)
}

// modelMemText says a loaded model's memory use (0: not measured).
func modelMemText(b int64) string {
	if b <= 0 {
		return i18n.T("Loaded")
	}
	return i18n.T("Loaded · %s in memory", uitext.MemoryBytes(b))
}

// modelActions are a card's buttons for the model's state.
func (a *app) modelActions(c *ui.Context, m models.Model, st models.Status) {
	name := uitext.ModelName(m.ID)
	switch {
	case st.Downloading:
		if ui.Button(c.Key("cancel"), i18n.T("Cancel")).Label(i18n.T("Cancel downloading %s", name)).Clicked() {
			a.cancelModel(m.ID)
		}
	case st.Installed:
		label, tip := i18n.T("Delete"), i18n.T("Delete %s…", name)
		if m.Borrowed { // the files are the other app's: VoxtMini lets go of it
			label, tip = i18n.T("Remove"), i18n.T("Remove %s…", name)
		}
		if ghostButton(c.Key("delete"), label).Label(tip).Disabled(a.modelDeciding == m.ID).Clicked() {
			a.askDeleteModel(m.ID) // models_delete.go
		}
	case m.Borrowed || st.OnDisk: // another app's on this Mac: VoxtMini's once used
		if st.OnDisk && ui.Button(c.Key("use"), i18n.T("Use it")).Label(i18n.T("Use %s", name)).Clicked() {
			a.useModel(m.ID)
		}
	default:
		elsewhere := st.Elsewhere != ""
		if st.Done > 0 && !elsewhere && ghostButton(c.Key("discard"), i18n.T("Delete")).Label(i18n.T("Delete what was downloaded of %s", name)).Clicked() {
			a.deleteModel(m.ID, fammodels.ThisApp)
		}
		label := i18n.T("Download")
		switch {
		case st.DeletedBy != "":
			label = i18n.T("Download again")
		case st.Done > 0 && !elsewhere:
			label = i18n.T("Resume")
		}
		if st.Err == nil && secondaryIconButton(c.Key("download"), icons.DownloadMinimalistic, label).Label(i18n.T("Download %s", name)).Disabled(elsewhere).Clicked() { // a failure: the notice's Try again
			a.downloadModel(m.ID)
		}
	}
}
