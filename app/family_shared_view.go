package main

// The line under a setting VoxtMini shares with the other ActNow apps
// (family spec §9.3: the interface language under Settings → General,
// the downloads' proxy and source in the Network dialog).

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// familyLine says under a shared setting whether it applies to every
// ActNow app, with the link that changes it (follow), or, once after the
// first start, that the family uses another value (other, §9.4) with
// "Use theirs"; dismiss, when set, closes that notice (×). With no other
// ActNow app installed it says nothing but the conflict (which another
// app's value implies) or a newer app's read-only directory.
func (a *app) familyLine(c *ui.Context, key, other string, follow func(key string, on bool), dismiss func()) {
	t := c.Theme()
	follows := a.prefs.Family.Follows(key)
	if slices.Contains(a.prefs.Family.Conflicts, key) && !follows {
		notice(c, "fam-conflict-"+keySuffix(key), noticeInfo, other, "", func() {
			if ui.Button(c.Key("theirs"), i18n.T("Use theirs")).Clicked() {
				follow(key, true)
			}
		}, dismiss)
		return
	}
	if !a.familyOthers() && !(follows && a.fam.ReadOnly()) {
		return
	}
	ui.Row(c.Key("fam-line-" + keySuffix(key))).Gap(px(t, sp2)).AlignItems(ui.Center).Wrap().Children(func() {
		if follows && a.fam.ReadOnly() {
			caption(c, i18n.T("Another ActNow app updated the shared format. Update this app to change these settings."))
			return
		}
		if follows {
			caption(c, i18n.T("Applies to all ActNow apps"))
			if ui.Link(c.Key("own"), i18n.T("Use a different setting for this app"), "").FontSize(px(t, fsCaption)).Clicked() {
				follow(key, false)
			}
			return
		}
		caption(c, i18n.T("Only VoxtMini uses this setting."))
		if ui.Link(c.Key("follow"), i18n.T("Use the same setting as the other ActNow apps"), "").FontSize(px(t, fsCaption)).Clicked() {
			follow(key, true)
		}
	})
}

// languageFamilyLine is the interface language's line (Settings →
// General). Following or not changes the language at once
// (settingsChanged → applyLocale).
func (a *app) languageFamilyLine(c *ui.Context) {
	if a.fam == nil {
		return
	}
	fam := a.fam
	a.familyLine(c, actnow.KeyLanguage, i18n.T("Other ActNow apps use a different language."), func(key string, on bool) {
		a.updateSettings(func(s *settings.Settings) { fam.Follow(s, key, on) })
		a.loadSettingsForm()
	}, func() {
		a.updateSettings(func(s *settings.Settings) {
			s.Family.Conflicts = slices.DeleteFunc(slices.Clone(s.Family.Conflicts), func(k string) bool { return k == actnow.KeyLanguage })
		})
	})
}
