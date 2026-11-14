package main

// Settings → Storage's clean-up of the models only uninstalled ActNow
// apps claimed (family spec §7.6 step 4, §7.7; internal/models,
// orphans.go): a row under the model folder's usage says how much they
// take, "Clean up" lists them, all picked, and deletes those still picked.
// Each deletion decides again (an app that claims or loads one meanwhile
// keeps it).

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/uitext"
	fammodels "github.com/hehehai/actnowkit/models"
)

// orphanState is the clean-up dialog's state.
type orphanState struct {
	orphanOpen bool
	orphanPick map[fammodels.Ref]bool
	cleaning   bool
}

// orphanBytes is what the orphans r lists take.
func orphanBytes(list []models.Orphan) int64 {
	var n int64
	for _, o := range list {
		n += o.Bytes
	}
	return n
}

// openOrphans opens the dialog with every model picked.
func (a *app) openOrphans() {
	a.orphanOpen = true
	a.orphanPick = map[fammodels.Ref]bool{}
	for _, o := range a.stReport.orphans {
		a.orphanPick[o.Ref] = true
	}
}

// pickedOrphans are the models picked, and what they take.
func (a *app) pickedOrphans() (refs []fammodels.Ref, bytes int64) {
	if a.stReport == nil {
		return nil, 0
	}
	for _, o := range a.stReport.orphans {
		if a.orphanPick[o.Ref] {
			refs = append(refs, o.Ref)
			bytes += o.Bytes
		}
	}
	return refs, bytes
}

// cleanOrphans deletes the picked models, then measures again.
func (a *app) cleanOrphans() {
	refs, _ := a.pickedOrphans()
	a.orphanOpen = false
	if len(refs) == 0 {
		return
	}
	a.cleaning = true
	m := a.models
	a.async(func() {
		r, err := m.DeleteOrphans(context.Background(), refs)
		a.post(func() {
			a.cleaning = false
			a.stReport = nil // measured again
			switch {
			case err != nil:
				if bid, busy := models.Busy(err); busy && bid != "" {
					a.toast(i18n.T("%s is downloading or using this model. Try again later.", ownerName(bid)))
				} else if busy {
					a.toast(i18n.T("Another app is downloading or using this model. Try again later."))
				} else {
					a.toast(i18n.T("Could not delete every model: %v", err))
				}
			case r.Kept > 0:
				a.toast(i18n.N(r.Kept, "Freed %s. %d model is in use again and stays.", "Freed %s. %d models are in use again and stay.", uitext.HumanBytes(r.Freed), r.Kept))
			default:
				a.toast(i18n.T("Freed %s", uitext.HumanBytes(r.Freed)))
			}
		})
	})
}

// orphanRow is the usage grid's row of the orphans, with Clean up (r's
// orphans not empty).
func (a *app) orphanRow(c *ui.Context, r *storageReport) {
	t := c.Theme()
	ui.Row(c.Key("st-orphans-title")).Gap(px(t, 6)).AlignItems(ui.Center).Children(func() {
		ui.Text(c, i18n.T("Left by uninstalled apps"))
		infoTip(c, i18n.T("Models only apps that are no longer installed used. Neither VoxtMini nor an installed ActNow app needs them."))
	})
	ui.Text(c, uitext.HumanBytes(orphanBytes(r.orphans))).TextColor(c.Theme().TextMuted).TextAlign(ui.End)
	ui.Row(c.Key("st-orphans-act")).Children(func() {
		if ui.Button(c.Key("st-orphans-clean"), i18n.T("Clean up")).Disabled(a.cleaning).Clicked() {
			a.openOrphans()
		}
	})
}

// orphanDialog lists the orphans to pick from.
func (a *app) orphanDialog(c *ui.Context) {
	if !a.orphanOpen {
		return
	}
	if a.stReport == nil || len(a.stReport.orphans) == 0 {
		a.orphanOpen = false
		return
	}
	t := c.Theme()
	refs, bytes := a.pickedOrphans()
	dialogFrame(c, "st-orphans-dlg", &a.orphanOpen, i18n.T("Clean up models"), "", 480, func() {
		for _, o := range a.stReport.orphans {
			pick := a.orphanPick[o.Ref]
			ui.Row(c.Key("orphan-" + o.Ref.Repo)).Gap(px(t, sp3)).AlignItems(ui.Center).Children(func() {
				if ui.Checkbox(c.Key("box"), &pick, "").Label(o.Name).Shrink(0).Changed() {
					a.orphanPick[o.Ref] = pick
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
					truncText(c, o.Name)
					caption(c, strings.Join(o.Apps, ", "))
				})
				ui.Text(c, uitext.HumanBytes(o.Bytes)).TextColor(t.TextMuted).Shrink(0)
			})
		}
	}, func() {
		ui.Spacer(c)
		if ui.Button(c.Key("st-orphans-cancel"), i18n.T("Cancel")).AutoFocus().Clicked() {
			a.orphanOpen = false
		}
		label := i18n.T("Delete")
		if len(refs) > 0 {
			label = i18n.T("Delete (%s)", uitext.HumanBytes(bytes))
		}
		if recordButton(c.Key("st-orphans-ok"), label).Disabled(len(refs) == 0).Clicked() {
			a.cleanOrphans()
		}
	})
}
