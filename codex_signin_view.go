package main

// The service dialog's sign-in field (codex_signin.go), in place of the
// key field for a service that signs in with an account.

import (
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
)

// signInField is "Sign in with ChatGPT" (or Cancel while the browser is
// open), or the account signed in with Sign out.
func (a *app) signInField(c *ui.Context, mask string) {
	t := c.Theme()
	k := tok(t)
	d := &a.rmDlg
	info := i18n.T("Uses the Codex models of your ChatGPT plan. The sign-in is saved in this Mac's Keychain, never in VoxtMini's settings or logs.")
	fieldInfo(c, i18n.T("ChatGPT account"), info, func() {
		ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Children(func() {
			switch {
			case d.signIn.running:
				ui.Spinner(c).Size(px(t, 14), px(t, 14)).Shrink(0)
				caption(c, i18n.T("Waiting for the browser…")).Grow(1).MinWidth(0)
				if ghostButton(c.Key("rm-signin-cancel"), i18n.T("Cancel")).Label(i18n.T("Cancel the sign-in")).Shrink(0).Clicked() {
					a.cancelSignIn()
				}
			case mask != "":
				statusMark(c, icons.CheckCircle, k.Success, i18n.T("Signed in as %s", mask)).Grow(1).MinWidth(0)
				if ghostButton(c.Key("rm-signout"), i18n.T("Sign out")).Shrink(0).Clicked() {
					a.signOut()
				}
			default:
				if ui.Button(c.Key("rm-signin"), i18n.T("Sign in with ChatGPT")).Shrink(0).Clicked() {
					a.signIn()
				}
			}
		})
		if d.signIn.err != "" {
			caption(c, d.signIn.err).TextColor(k.Danger)
		}
	})
}
