package main

// Apple's models on the Models page (models_view.go's "Built into
// macOS" section, docs/apple-models.md): one card per tab, Apple Speech
// or Apple Intelligence, once the helper said what this Mac has
// (a.appleSt, apple_link.go). A card says whether the model can be used
// and why not; Apple Speech's lists its languages, downloads the user's
// through Apple, and asks for Speech Recognition.

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/permission"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// appleIDs are the tab's Apple models the page shows: none before the
// helper said anything (no helper yet, or an older one).
func (a *app) appleIDs() []string {
	if !a.appleSt.Known {
		return nil
	}
	if a.mdTab == 1 {
		return []string{apple.IntelligenceID}
	}
	return []string{apple.SpeechID}
}

// appleReady says whether Apple's model id can be used now, and why not.
func (a *app) appleReady(id string) (bool, string) {
	st := a.appleSt
	if id == apple.IntelligenceID {
		if st.IntelligenceReady() {
			return true, ""
		}
		return false, uitext.AppleReasonText(st.IntelligenceReason())
	}
	switch {
	case st.Speech.Engine == "":
		return false, uitext.AppleReasonText(st.Speech.Reason)
	case !st.SpeechReady():
		return false, i18n.T("None of its languages is on this Mac yet.")
	}
	return true, ""
}

// appleCard is one of Apple's models.
func (a *app) appleCard(c *ui.Context, id string) {
	t := c.Theme()
	k := tok(t)
	name := uitext.ModelName(id)
	hidden := a.prefs.Models.IsHidden(id)
	ready, why := a.appleReady(id)
	ic := icons.Soundwave
	if id == apple.IntelligenceID {
		ic = icons.ChatRoundLine
	}
	el := card(c.Key("model-" + id))
	if hidden {
		el.Opacity(0.6)
	}
	el.ContextMenu(func(mn *ui.Menu) { a.hideMenu(mn, id, name) })
	el.Children(func() {
		ui.Row(c).Gap(px(t, sp3)).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, ic.Linear).Size(px(t, 20), px(t, 20)).TextColor(k.Text2).Shrink(0).Margin(px(t, 1), 0, 0, 0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, sp1)).Children(func() {
				ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Wrap().Children(func() {
					ui.Text(c, name).FontWeight(600)
					if hidden {
						tagChip(c, i18n.T("Hidden"))
					}
					if ready {
						statusMark(c, icons.CheckCircle, k.Success, i18n.T("Ready"))
					} else {
						statusMark(c, icons.DangerTriangle, k.Warning, i18n.T("Not available"))
					}
				})
				ui.Text(c, uitext.ModelAbout(id))
				ui.Row(c).Gap(px(t, 4)).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icons.Global.Linear).Size(px(t, 14), px(t, 14)).TextColor(k.Text2).Shrink(0)
					caption(c, a.appleLanguagesLine(id)).MinWidth(0)
				})
				caption(c.Key("no-download"), i18n.T("No download from VoxtMini: macOS provides it."))
				if why != "" {
					caption(c.Key("why"), why)
				}
			})
			ui.Row(c.Key("actions")).Gap(px(t, sp2)).Shrink(0).AlignItems(ui.Center).Children(func() {
				if id == apple.SpeechID {
					a.appleSpeechActions(c)
				}
				moreMenu(c.Key("more"), i18n.T("More actions for %s", name), func(mn *ui.Menu) { a.hideMenu(mn, id, name) })
			})
		})
		if id == apple.SpeechID {
			a.appleSpeechNotices(c)
		}
	})
}

// appleLanguagesLine is the card's languages: Apple Speech's with their
// state on this Mac.
func (a *app) appleLanguagesLine(id string) string {
	if id != apple.SpeechID || a.appleSt.Speech.Engine == "" {
		return uitext.AppleLanguages()
	}
	var parts []string
	for _, loc := range apple.Locales {
		state, _ := uitext.AppleLocaleState(a.appleSt.LocaleStatus(loc))
		parts = append(parts, i18n.T("%s: %s", uitext.AppleLocaleName(loc), state))
	}
	return strings.Join(parts, " · ")
}

// appleSpeechActions are Apple Speech's buttons: download the user's
// languages, allow Speech Recognition.
func (a *app) appleSpeechActions(c *ui.Context) {
	if a.appleSt.Speech.Engine == "" {
		return
	}
	if a.perms.speech == permission.NotDetermined && a.hstatus.Has(proto.MethodPermissionsRequest) {
		if ghostButton(c.Key("apple-allow"), i18n.T("Allow speech recognition")).Clicked() {
			a.askPermission(proto.PermissionSpeechRecognition)
		}
	}
	if len(a.appleToDownload()) > 0 && !a.appleDl.busy {
		if secondaryIconButton(c.Key("apple-download"), icons.DownloadMinimalistic, i18n.T("Download languages")).
			Tooltip(i18n.T("Apple downloads your languages for its speech recognition.")).Clicked() {
			a.downloadAppleLanguages()
		}
	}
}

// appleSpeechNotices are Apple Speech's download bar, its failure, and
// Speech Recognition refused.
func (a *app) appleSpeechNotices(c *ui.Context) {
	if a.appleDl.busy {
		ui.Progress(c, a.appleDl.fraction).Label(i18n.T("Downloading languages for Apple Speech"))
	}
	if err := a.appleDl.err; err != nil {
		notice(c, "apple-dl-err", noticeError, i18n.T("Apple could not download the languages. Try again later."), err.Error(), nil, nil)
	}
	if a.appleSt.Speech.Engine != "" && (a.perms.speech == permission.Denied || a.perms.speech == permission.Restricted) {
		notice(c, "apple-perm", noticeWarn, i18n.T("Speech recognition is not allowed."), "", func() {
			if ui.Button(c.Key("apple-perm-open"), i18n.T("Open Permissions")).Clicked() {
				a.navigate("/settings/permissions")
			}
		}, nil)
	}
}
