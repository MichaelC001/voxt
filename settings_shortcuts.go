package main

// Settings → Shortcuts (/settings/shortcuts): every global shortcut
// in one table (Transcribe's, Translate's and Rewrite's, and Paste the
// last result), with their trigger and what hotkey.Check finds wrong:
// clashes, keys macOS uses, keys that need Accessibility while the event
// tap is missing, keys another ActNow app uses too; over it the presets
// (hotkey.Presets), which set the three features' at once. A row opens its
// feature's page, where it changes. Under it, Escape and the voice end
// command (settings_recording.go).

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// featurePath is the page a shortcut changes on.
func featurePath(feature string) string {
	switch feature {
	case hotkey.Translate:
		return "/translate"
	case hotkey.Rewrite:
		return "/rewrite"
	}
	return "/transcribe" // Transcribe, and Paste the last result
}

// shortcutsSettings is the Shortcuts section of the settings dialog.
func (a *app) shortcutsSettings(c *ui.Context) {
	t := c.Theme()
	k := tok(t)
	a.presetRow(c, "sc-preset")
	caption(c, i18n.T("Click a row to change it on its feature's page."))
	if !a.hotkeys.Installed {
		notice(c, "sc-no-tap", noticeWarn,
			i18n.T("Without Accessibility, only plain key combos work."),
			i18n.T("fn, a single modifier, mouse buttons, double taps and long presses need Accessibility; ⌃⌥⇧⌘ with a key works without it."), func() {
				if ui.Button(c.Key("sc-open-perms"), i18n.T("Open Permissions")).Clicked() {
					a.navigate("/settings/permissions")
				}
			}, nil)
	}
	f := a.prefs.Features
	keysW := px(t, 96)
	card(c.Key("sc-table")).Gap(0).Padding(px(t, sp2)).Children(func() {
		// The head.
		ui.Row(c).Gap(px(t, sp2)).Padding(px(t, sp1), px(t, sp2)).AlignItems(ui.Center).Children(func() {
			head := func(s string) ui.Element {
				return ui.Text(c, s).FontSize(px(t, fsMicro)).FontWeight(600).TextColor(k.Text3).SingleLine().Ellipsis("…").MinWidth(0)
			}
			head(i18n.T("Feature")).Grow(1)
			head(i18n.T("Shortcut")).Width(keysW).Shrink(0)
			ui.Box(c).Width(px(t, shortcutMarksW+14+sp2)).Shrink(0) // the marks and the chevron
		})
		for _, feat := range append(append([]string{}, hotkey.Features...), hotkey.PasteLast) {
			hs := hotkey.HotkeysOf(f, feat)
			problems := uitext.HotkeyProblems(f, feat) // feature_keys_text.go
			if len(hs) == 0 {
				a.shortcutRow(c, feat, 0, nil, nil, keysW)
				continue
			}
			for i := range hs {
				// The other ActNow apps on the same keys (settings_shortcuts_family.go).
				lines := append(slices.Clone(problems[i]), a.conflictLines(feat, i)...)
				a.shortcutRow(c, feat, i, &hs[i], lines, keysW)
			}
		}
	})
	a.recordingSettings(c) // settings_recording.go
}

// shortcutMarksW is the room of a row's marks (Accessibility, a problem).
const shortcutMarksW = 16 + sp2 + 16

// shortcutRow is a row of the table: feature's i-th shortcut h (nil: it
// has none), its keys over its trigger, its problems under it; a click
// opens the feature's page.
func (a *app) shortcutRow(c *ui.Context, feature string, i int, h *settings.Hotkey, problems []string, keysW float32) {
	t := c.Theme()
	k := tok(t)
	title := uitext.FeatureTitle(feature)
	keys := i18n.T("None")
	if h != nil {
		keys = uitext.HotkeyText(h.Keys)
	}
	b := ui.ButtonBase(c.Key("sc-"+feature+"-"+itoa(i))).Padding(px(t, 6), px(t, sp2)).Radius(px(t, radiusControl)).
		AlignItems(ui.Stretch).Label(title + " " + keys).Tooltip(i18n.T("Change on the %s page", pageTitle(featurePath(feature))))
	if b.Hovered() {
		b.Background(k.Hover)
	}
	b.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 4)).Children(func() {
			ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Children(func() {
				ic := kindIcon(featureKind(feature))
				if feature == hotkey.PasteLast {
					ic = icons.Copy
				}
				ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Grow(1).Basis(0).MinWidth(0).Children(func() {
					if i == 0 {
						ui.Icon(c, ic.Linear).Size(px(t, 16), px(t, 16)).TextColor(k.Text2).Shrink(0)
						truncText(c, title)
					}
				})
				ui.Column(c).Width(keysW).Shrink(0).Gap(px(t, 2)).AlignItems(ui.Start).Children(func() {
					if h == nil {
						ui.Text(c, keys).FontSize(px(t, fsCaption)).TextColor(k.Text3).SingleLine()
						return
					}
					kbd(c, keys).MinWidth(0).Ellipsis("…") // shell_commands.go
					trig := uitext.TriggerText(h.Trigger)
					if feature == hotkey.PasteLast {
						trig = uitext.TriggerText(settings.TriggerTap) // always a tap
					}
					ui.Text(c, trig).FontSize(px(t, fsMicro)).TextColor(k.Text2).SingleLine().Ellipsis("…").MinWidth(0)
				})
				ui.Row(c).Width(px(t, shortcutMarksW)).Shrink(0).Gap(px(t, sp2)).Justify(ui.End).Children(func() {
					if h != nil && hotkey.NeedsTap(*h) {
						col := k.Text3
						if !a.hotkeys.Installed {
							col = k.Warning
						}
						iconMark(c, icons.Accessibility, col, i18n.T("Works only while VoxtMini may use Accessibility."))
					}
					if len(problems) > 0 {
						iconMark(c, icons.DangerTriangle, k.Warning, strings.Join(problems, "\n"))
					}
				})
				ui.Icon(c, icons.AltArrowRight.Linear).Size(px(t, 14), px(t, 14)).TextColor(k.Text3).Shrink(0)
			})
			if len(problems) > 0 {
				ui.Column(c).Padding(0, 0, 0, px(t, 16+sp2)).Gap(px(t, 2)).Children(func() {
					problemLines(c, problems) // feature_keys.go
				})
			}
		})
	})
	if b.Clicked() {
		a.navigate(featurePath(feature))
	}
}

// presetRow is the shortcut presets' select (Settings → Shortcuts, the
// setup guide): the one the shortcuts are, "Custom" when none (offered
// only then). Choosing one replaces the three features' shortcuts.
func (a *app) presetRow(c *ui.Context, key string) {
	t := c.Theme()
	opts, ids := presetOptions(hotkey.PresetOf(a.prefs.Features))
	a.presetSel = uitext.PresetText(hotkey.PresetOf(a.prefs.Features))
	help := i18n.T("Sets the shortcuts of Transcribe, Translate and Rewrite at once; Paste the last result stays. Mouse middle: click to transcribe, double-click to rewrite.")
	settingRow(c, key, i18n.T("Preset"), help, func() {
		if ui.Select(c.Key(key+"-sel"), &a.presetSel, opts).Label(i18n.T("Shortcut preset")).Width(px(t, 180)).Changed() {
			if i := slices.Index(opts, a.presetSel); i >= 0 {
				a.choosePreset(ids[i])
			}
		}
	})
}

// presetOptions are the select's labels and their presets: every preset,
// and Custom when cur is.
func presetOptions(cur string) (opts, ids []string) {
	for _, p := range hotkey.Presets {
		opts, ids = append(opts, uitext.PresetText(p.ID)), append(ids, p.ID)
	}
	if cur == hotkey.PresetCustom {
		opts, ids = append(opts, uitext.PresetText(cur)), append(ids, cur)
	}
	return opts, ids
}

// choosePreset sets the three features' shortcuts to preset id's
// (Custom changes nothing).
func (a *app) choosePreset(id string) {
	if id == hotkey.PresetCustom {
		return
	}
	a.updateSettings(func(s *settings.Settings) { hotkey.ApplyPreset(&s.Features, id) })
}
