package main

// Settings → Integrations (/settings/integrations): segmented tabs for
// MCP (settings_integrations_mcp.go) and External trigger (main toggle, URL
// sub-toggle, quick reference for voxtmini-ctl, example URL, and current
// microphones with UID copy buttons).

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/ctlbridge"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/settings"
)

const exampleTriggerURL = "voxtmini://trigger/transcribe?action=toggle&device=<uid>"

func (a *app) integrationsSettings(c *ui.Context) {
	t := c.Theme()
	segmentedFit(c.Key("integ-tab"), &a.integTab, i18n.T("MCP"), i18n.T("External trigger"))
	ui.Column(c.Key("integ-body")).Gap(px(t, sp4)).Children(func() {
		if a.integTab == 0 {
			a.integrationsMCPTab(c)
		} else {
			a.integrationsTriggerTab(c)
		}
	})
}

func (a *app) integrationsTriggerTab(c *ui.Context) {
	t := c.Theme()
	k := tok(t)

	settingRow(c, "integ-trig-on", i18n.T("Allow external apps to trigger"),
		i18n.T("Let companion apps, Shortcuts or scripts start Transcribe, Translate or Rewrite with a chosen microphone."),
		func() {
			if ui.Switch(c.Key("integ-trig-on"), &a.settingsForm.triggerEnabled).Label(i18n.T("Allow external apps to trigger")).Changed() {
				on := a.settingsForm.triggerEnabled
				a.updateSettings(func(s *settings.Settings) { s.Trigger.Enabled = on })
			}
		})

	settingRow(c, "integ-trig-url", i18n.T("Allow URL triggers"),
		i18n.T("Accept voxtmini://trigger links from other apps."),
		func() {
			if ui.Switch(c.Key("integ-trig-url"), &a.settingsForm.triggerAllowURL).
				Label(i18n.T("Allow URL triggers")).
				Disabled(!a.prefs.Trigger.Enabled).Changed() {
				on := a.settingsForm.triggerAllowURL
				a.updateSettings(func(s *settings.Settings) { s.Trigger.AllowURL = &on })
			}
		})

	sectionTitle(c, i18n.T("Quick reference"))
	copyRefRow(c, "integ-ctl-path", i18n.T("CLI path"), ctlbridge.CtlPath(a.resourcesDir), a.toast)
	copyRefRow(c, "integ-url-example", i18n.T("Example URL"), exampleTriggerURL, a.toast)

	ui.Row(c).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
		sectionTitle(c, i18n.T("Microphones"))
		ui.Spacer(c)
		if iconButton(c.Key("integ-mics-refresh"), icons.Refresh, i18n.T("Refresh")).Clicked() {
			a.loadDevices()
		}
	})
	if len(a.audio.devices) == 0 {
		ui.Text(c, i18n.T("No microphones detected")).FontSize(px(t, fsCaption)).TextColor(k.Text2)
		return
	}
	card(c).Padding(px(t, sp2), px(t, sp3)).Gap(px(t, sp2)).Children(func() {
		for i, d := range a.audio.devices {
			if i > 0 {
				ui.Divider(c)
			}
			ui.Row(c.Key(fmt.Sprintf("integ-mic-%d", i))).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
					label := d.Name
					if d.Default {
						label += " · " + i18n.T("Default")
					}
					truncText(c, label).FontSize(px(t, fsBody))
					truncText(c, d.ID).FontSize(px(t, fsMicro)).TextColor(k.Text2)
				})
				if ui.Button(c.Key(fmt.Sprintf("integ-mic-copy-%d", i)), i18n.T("Copy UID")).Shrink(0).Clicked() {
					c.Services().WriteClipboard(d.ID)
					a.toast(i18n.T("Copied"))
				}
			})
		}
	})
}

func copyRefRow(c *ui.Context, key, label, value string, toast func(string)) {
	t := c.Theme()
	k := tok(t)
	ui.Column(c.Key(key)).Gap(px(t, sp1)).Children(func() {
		caption(c, label)
		ui.Row(c).AlignItems(ui.Center).Gap(px(t, sp2)).Padding(px(t, sp1), px(t, sp2)).
			Radius(px(t, radiusControl)).Background(k.Subtle).Border(1, k.Line).Children(func() {
			truncText(c, value).FontSize(px(t, fsMicro)).TextColor(k.Text2).Grow(1)
			if ui.Button(c.Key(key+"-copy"), i18n.T("Copy")).Shrink(0).Clicked() {
				c.Services().WriteClipboard(value)
				toast(i18n.T("Copied"))
			}
		})
	})
}
