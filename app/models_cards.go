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

// modelCard is one model, about three short lines: its name with its
// chips and marks (shared with other apps, loaded: an icon with a
// tooltip), what it is good at (one line, all of it in the tooltip), and
// its series and size. The one button is what its state needs
// (Download, Cancel, Use it); the rest is in ⋯ and the context menu.
// Dimmed while hidden.
func (a *app) modelCard(c *ui.Context, m models.Model) {
	t := c.Theme()
	k := tok(t)
	st := a.modelStatus[m.ID]
	hidden := a.prefs.Models.IsHidden(m.ID)
	name := uitext.ModelName(m.ID)
	ic := icons.Soundwave
	if m.Kind == models.LLM {
		ic = icons.ChatRoundLine
	}
	el := card(c.Key("model-" + m.ID))
	if hidden {
		el.Opacity(0.6)
	}
	el.ContextMenu(func(mn *ui.Menu) { a.modelMenu(mn, m, st) })
	el.Children(func() {
		ui.Row(c).Gap(px(t, sp3)).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, ic.Linear).Size(px(t, 20), px(t, 20)).TextColor(k.Text2).Shrink(0).Margin(px(t, 1), 0, 0, 0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
				// The name, its marks, then its chips (they wrap under it in a
				// narrow window rather than cut the name).
				ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Text(c, name).FontWeight(600)
					if fam := modelFamilyText(m, st); fam != "" && !modelFamilyBusy(st) {
						mark := icons.UsersGroupRounded
						if !st.Downloading && !st.OnDisk && st.DeletedBy != "" {
							mark = icons.TrashBinMinimalistic
						}
						iconMark(c.Key("family"), mark, k.Text2, fam)
					}
					if mem, ok := a.modelMem[m.ID]; ok && st.Installed { // models_memory.go
						text := modelMemText(mem)
						ui.Box(c.Key("loaded")).Size(px(t, 16), px(t, 16)).Center().Label(text).Tooltip(text).Shrink(0).Children(func() {
							statusDot(c, k.Accent)
						})
					}
					modelChips(c, m, st, hidden)
				})
				truncText(c, uitext.ModelAbout(m.ID))
				truncText(c.Key("series"), modelSeriesText(m, st)).FontSize(px(t, fsCaption)).TextColor(k.Text2)
			})
			ui.Row(c.Key("actions")).Gap(px(t, sp2)).Shrink(0).AlignItems(ui.Center).Children(func() {
				a.modelActions(c, m, st)
				moreMenu(c.Key("more"), i18n.T("More actions for %s", name), func(mn *ui.Menu) { a.modelMenu(mn, m, st) })
			})
		})
		if st.Downloading {
			frac := 0.0
			if st.Total > 0 {
				frac = float64(st.Done) / float64(st.Total)
			}
			ui.Progress(c, frac).Label(i18n.T("Downloading %s", name))
		}
		if st.Err != nil && !st.Downloading {
			msg, more := uitext.DownloadErrorText(st.Err)
			notice(c, "model-err", noticeError, msg, more, func() {
				a.reportLink(c, failure{key: m.ID, title: i18n.T("The download failed"), code: codeModel, kind: kindModelDownload,
					where: "Models › Model card", message: st.Err.Error()})
				if ui.Button(c.Key("retry"), i18n.T("Try again")).Clicked() {
					a.downloadModel(m.ID)
				}
			}, nil)
		}
	})
}

// modelSeriesText is the card's caption: the model's series when its
// name is a plain one ("Qwen3.5 4B OptiQ · 4-bit"), then its size or
// its download's state.
func modelSeriesText(m models.Model, st models.Status) string {
	size := modelSizeText(m, st)
	if modelFamilyBusy(st) { // "Downloading in AudioSmith… 43%" says it all
		size = modelFamilyText(m, st)
	}
	if uitext.ModelName(m.ID) == m.Title {
		return size
	}
	return m.Title + " · " + size
}

// modelFamilyBusy: another app downloads the model, which the caption
// says instead of the size.
func modelFamilyBusy(st models.Status) bool {
	return st.Downloading && st.WaitingFor != "" || !st.Downloading && !st.OnDisk && st.Elsewhere != ""
}

// modelMenu is a card's ⋯ and context menu: its Settings, deleting
// what it downloaded or the model, hiding it.
func (a *app) modelMenu(mn *ui.Menu, m models.Model, st models.Status) {
	if st.Installed && modelKnobs(m.ID) != nil {
		if mn.Item(i18n.T("Settings")).Chosen() {
			a.openModelSettings(m.ID) // models_settings_view.go
		}
	}
	switch {
	case st.Installed:
		label := i18n.T("Delete")
		if m.Borrowed { // the files are the other app's: VoxtMini lets go of it
			label = i18n.T("Remove")
		}
		if mn.Item(label).Disabled(a.modelDeciding == m.ID).Chosen() {
			a.askDeleteModel(m.ID) // models_delete.go
		}
	case !st.Downloading && !m.Borrowed && !st.OnDisk && st.Done > 0 && st.Elsewhere == "":
		if mn.Item(i18n.T("Delete the partial download")).Chosen() {
			a.deleteModel(m.ID, fammodels.ThisApp)
		}
	}
	a.hideMenu(mn, m.ID, uitext.ModelName(m.ID)) // models_hide.go
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
		statusMark(c, icons.CheckCircle, k.Success, i18n.T("Installed")).Shrink(0)
	}
}

// modelSizeText is a model's size, or how much of it came.
func modelSizeText(m models.Model, st models.Status) string {
	size := uitext.HumanBytes(m.Size())
	switch {
	case st.Downloading:
		return i18n.T("Downloading: %s of %s (%s)", uitext.HumanBytes(st.Done), uitext.HumanBytes(st.Total), fmt.Sprintf("%d%%", percent(st.Done, st.Total)))
	case st.Installed || st.OnDisk:
		return size
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

// modelFamilyText is what the family says of a model ("" nothing): a
// borrowed model VoxtMini does not use yet, one it shares with other apps
// (neither names the apps, actnowkit v0.22.1: the list grows with the
// family), another app downloading what VoxtMini's download waits for or
// the model itself, or the app that deleted it for all apps.
func modelFamilyText(m models.Model, st models.Status) string {
	switch {
	case (m.Borrowed || st.OnDisk) && !st.Installed:
		if len(st.Users) == 0 {
			return i18n.T("Already on this Mac")
		}
		return i18n.T("Installed · Shared across ActNow")
	case st.Installed && len(st.Users) > 0:
		return i18n.T("Shared across ActNow")
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

// modelActions is a card's button for the model's state: Cancel while
// it downloads, Use it for another app's model on this Mac, Download (or
// Resume, Download again) before it is installed; none once installed.
func (a *app) modelActions(c *ui.Context, m models.Model, st models.Status) {
	name := uitext.ModelName(m.ID)
	switch {
	case st.Downloading:
		if ui.Button(c.Key("cancel"), i18n.T("Cancel")).Label(i18n.T("Cancel downloading %s", name)).Clicked() {
			a.cancelModel(m.ID)
		}
	case st.Installed:
	case m.Borrowed || st.OnDisk: // another app's on this Mac: VoxtMini's once used
		if st.OnDisk && ui.Button(c.Key("use"), i18n.T("Use it")).Label(i18n.T("Use %s", name)).Clicked() {
			a.useModel(m.ID)
		}
	default:
		elsewhere := st.Elsewhere != ""
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
