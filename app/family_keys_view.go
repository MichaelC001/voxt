package main

// The service dialog's parts about the ActNow family's keys
// (family_keys.go; spec §8.3): the family's key offered in place of the
// key field, and the checkbox that shares a key.

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// sharedKeyOffer is "Use the DeepSeek key from AudioSmith (sk-…a1b2)"
// with Use this key and Enter another key, in place of the key field: the
// key is read only when the user clicks.
func (a *app) sharedKeyOffer(c *ui.Context, name string) {
	t := c.Theme()
	d := &a.rmDlg
	e := d.fam.offer
	text := i18n.T("Use the %s key from %s (%s)", name, actnow.AppName(e.AddedBy), e.Hint)
	fieldInfo(c, uitext.KeyLabel(d.kind, d.id), "", func() {
		banner(c, icons.KeyMinimalisticSquare, tok(t).Text2, text, func() {
			if ui.Button(c.Key("rm-shared-use"), i18n.T("Use this key")).Shrink(0).Clicked() {
				a.useSharedKey()
			}
			if ghostButton(c.Key("rm-shared-other"), i18n.T("Enter another key")).Shrink(0).Clicked() {
				d.fam.other = true
			}
		}).Shrink(0)
	})
}

// sharedKeyNotes go under the key field: the key chosen from the family,
// and the checkbox that shares the key with the other ActNow apps.
func (a *app) sharedKeyNotes(c *ui.Context, mask string) {
	t := c.Theme()
	d := &a.rmDlg
	f := &d.fam
	if f.chosen && strings.TrimSpace(d.key) == "" && f.offer != nil {
		caption(c, i18n.T("Uses the key from %s (%s).", actnow.AppName(f.offer.AddedBy), f.offer.Hint))
	}
	if !a.shareShown(mask) {
		return
	}
	ui.Row(c.Key("rm-share")).Gap(px(t, 6)).AlignItems(ui.Center).Children(func() {
		ui.Checkbox(c.Key("rm-share-box"), &f.share, i18n.T("Let other ActNow apps on this Mac use this key")).Shrink(1)
		infoTip(c, i18n.T("Saved once in the login Keychain for the ActNow apps: changing it changes it in each app that uses it.")).Shrink(0)
	})
	if f.share && f.ref != "" && strings.TrimSpace(d.key) != "" && len(f.others) > 0 {
		caption(c, i18n.T("Also updates it in %s.", strings.Join(f.others, ", ")))
	}
}

// sharedKeyError is the dialog's error when the family's key cannot be
// read.
func sharedKeyError() string {
	return i18n.T("Could not read the shared key: enter the key instead.")
}
