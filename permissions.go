package main

// Settings → Permissions (/settings/permissions): what macOS lets
// VoxtMini do, why each permission matters, and how to grant it. The
// helper, which records and posts keys, reads and asks for them
// (permissions.*): macOS checks its permissions against the app, so they
// are VoxtMini's. Before the helper is ready the Microphone and
// Accessibility are read here (internal/permission). The section reads
// them again when it opens, when VoxtMini comes to the front and with
// Check again. Browser addresses are read through Accessibility only, so
// there is no Automation permission to ask for. Speech Recognition shows
// only while a feature uses Apple Speech (appleSpeechInUse), as a row
// like the others.

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/permission"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// System Settings panes.
const (
	privacyMicrophone    = "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone"
	privacyAccessibility = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	privacySpeech        = "x-apple.systempreferences:com.apple.preference.security?Privacy_SpeechRecognition"
)

// permissionsState is what the section last read.
type permissionsState struct {
	permSys permission.System
	perms   struct {
		mic     permission.State
		access  bool
		checked bool
		// asking: a permissions.status is on its way (one at a time).
		asking bool
		// speech is Speech Recognition (Apple Speech's), as the helper
		// reports it; Unknown from a helper without it.
		speech permission.State
	}
}

// micState is a permission.State from the helper's microphone state.
func micState(s string) permission.State {
	switch s {
	case proto.PermGranted:
		return permission.Granted
	case proto.PermDenied:
		return permission.Denied
	case proto.PermRestricted:
		return permission.Restricted
	case proto.PermNotDetermined:
		return permission.NotDetermined
	}
	return permission.Unknown
}

// helperPerms takes the helper's answer to permissions.*.
func (a *app) helperPerms(r proto.PermissionsResult, err error) {
	if err != nil {
		if !a.perms.checked {
			a.localPerms() // nothing to keep: the app's own read
		}
		return // else the last answer stays
	}
	a.perms.mic, a.perms.access, a.perms.checked = micState(r.Microphone), r.Accessibility, true
	a.perms.speech = micState(r.SpeechRecognition)
}

// localPerms reads the Microphone and Accessibility from the app's own
// process (cheap system calls).
func (a *app) localPerms() {
	a.perms.mic, a.perms.access, a.perms.checked = a.permSys.Microphone(), a.permSys.AccessibilityTrusted(), true
}

func (a *app) openPermissions() {
	if a.permSys == nil {
		a.permSys = permission.Native()
	}
}

// recheckPermissions reads the Microphone and Accessibility again: as
// the helper sees them when it can say (what showed stays until it
// answers), else here at once.
func (a *app) recheckPermissions() {
	if !a.hstatus.Has(proto.MethodPermissionsStatus) {
		a.localPerms()
		return
	}
	if a.perms.asking {
		return
	}
	a.perms.asking = true
	helperCall(a, proto.MethodPermissionsStatus, nil, 5*time.Second, func(r proto.PermissionsResult, err error) {
		a.perms.asking = false
		a.helperPerms(r, err)
	})
}

// askPermission shows macOS's prompt for the Microphone or Accessibility
// (proto.Permission*), through the helper, else from here.
func (a *app) askPermission(which string) {
	if a.hstatus.Has(proto.MethodPermissionsRequest) {
		helperCall(a, proto.MethodPermissionsRequest, proto.PermissionsRequestParams{Permission: which}, 5*time.Minute, a.helperPerms)
		return
	}
	sys := a.permSys
	if which == proto.PermissionSpeechRecognition {
		return // only the helper can ask
	}
	if which == proto.PermissionAccessibility {
		sys.PromptAccessibility()
		a.localPerms()
		return
	}
	a.async(func() { sys.RequestMicrophone(func(bool) { a.post(a.localPerms) }) })
}

// openPrivacy opens a pane of System Settings.
func (a *app) openPrivacy(c *ui.Context, url string) {
	if a.win == nil {
		c.Services().OpenURL(url)
		return
	}
	go mygo.Shell.OpenExternal(url)
}

// appleSpeechInUse says whether a feature's speech model is Apple Speech:
// chosen, or Automatic with no other model downloaded. Only then does
// VoxtMini need Speech Recognition, so only then does the section show it.
func (a *app) appleSpeechInUse() bool {
	for _, f := range []string{hotkey.Transcribe, hotkey.Translate, hotkey.Rewrite} {
		if m, ok := a.featureModel(local.ASR, f); ok && m.ID == apple.SpeechID {
			return true
		}
	}
	return false
}

// permissionsSettings is the Permissions section of the settings dialog.
func (a *app) permissionsSettings(c *ui.Context) {
	t := c.Theme()
	if !a.perms.checked {
		a.recheckPermissions()
	}
	badge := func(text string, ok bool) {
		if ok {
			statusMark(c, icons.CheckCircle, t.Success, text)
		} else {
			statusMark(c, icons.DangerTriangle, t.Warning, text)
		}
	}

	micText, micOK := uitext.PermissionText(a.perms.mic)
	settingRow(c, "perm-mic", i18n.T("Microphone"), i18n.T("Transcribe, translate and rewrite by voice record while you hold or tap their shortcut."), func() {
		badge(micText, micOK)
		if a.perms.mic == permission.NotDetermined {
			if ghostButton(c.Key("perm-mic-ask"), i18n.T("Ask")).Clicked() {
				a.askPermission(proto.PermissionMicrophone)
			}
		}
		if !micOK {
			if ghostIconButton(c.Key("perm-mic-open"), icons.SquareArrowRightUp, i18n.T("Open System Settings")).Label(i18n.T("Open System Settings")).Clicked() {
				a.openPrivacy(c, privacyMicrophone)
			}
		}
	})
	accText, accOK := i18n.T("Not allowed"), a.perms.access
	if accOK {
		accText = i18n.T("Allowed")
	}
	settingRow(c, "perm-access", i18n.T("Accessibility"), i18n.T("Types results at your cursor, reads the selected text and the browser tab's address, and allows single-key shortcuts like Right ⌥. Without it, results are copied for you to paste."), func() {
		badge(accText, accOK)
		if !accOK {
			if ghostButton(c.Key("perm-access-ask"), i18n.T("Ask")).Tooltip(i18n.T("Adds VoxtMini to the list in System Settings")).Clicked() {
				a.askPermission(proto.PermissionAccessibility)
			}
			if ghostIconButton(c.Key("perm-access-open"), icons.SquareArrowRightUp, i18n.T("Open System Settings")).Label(i18n.T("Open System Settings")).Clicked() {
				a.openPrivacy(c, privacyAccessibility)
			}
		}
	})
	if a.perms.speech != permission.Unknown && a.appleSpeechInUse() {
		spText, spOK := uitext.PermissionText(a.perms.speech)
		settingRow(c, "perm-speech", i18n.T("Speech Recognition"), i18n.T("Only Apple Speech needs it: macOS recognises your speech on this Mac. The other speech models do not."), func() {
			badge(spText, spOK)
			if a.perms.speech == permission.NotDetermined {
				if ghostButton(c.Key("perm-speech-ask"), i18n.T("Ask")).Clicked() {
					a.askPermission(proto.PermissionSpeechRecognition)
				}
			}
			if !spOK {
				if ghostIconButton(c.Key("perm-speech-open"), icons.SquareArrowRightUp, i18n.T("Open System Settings")).Label(i18n.T("Open System Settings")).Clicked() {
					a.openPrivacy(c, privacySpeech)
				}
			}
		})
	}
	ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Margin(px(t, sp2), 0, 0, 0).Children(func() {
		if secondaryIconButton(c.Key("perm-recheck"), icons.Refresh, i18n.T("Check again")).Label(i18n.T("Check again")).Clicked() {
			a.recheckPermissions()
		}
		infoTip(c.Key("perm-note"), i18n.T("After you change a permission, macOS may ask to quit and reopen VoxtMini. Updating an ad hoc signed build can make macOS forget Accessibility: turn it off and on again."))
	})
}
