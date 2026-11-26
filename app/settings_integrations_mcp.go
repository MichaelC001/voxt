package main

// Settings → Integrations → MCP tab: enable toggle with first-time privacy
// confirmation, live status & self-test, permission scopes, one-click agent
// & skill setup with diff preview, connected agents, and audit records.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/agentsetup"
	"github.com/hehehai/VoxtMini/internal/ctlbridge"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/mcp"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

func (a *app) integrationsMCPTab(c *ui.Context) {
	a.mcpMainAndStatus(c)
	a.mcpPermissionsCard(c)
	a.mcpConnectAgentsCard(c)
	a.mcpKnownAgentsCard(c)
	a.mcpAuditCard(c)
	a.mcpEnableConfirmDialog(c)
	a.mcpPreviewDialog(c)
}

func (a *app) mcpMainAndStatus(c *ui.Context) {
	t := c.Theme()
	k := tok(t)

	settingRow(c, "integ-mcp-on", i18n.T("Enable MCP server"),
		i18n.T("Local Unix socket only; no network port is opened."),
		func() {
			if ui.Switch(c.Key("integ-mcp-on"), &a.settingsForm.mcpEnabled).Label(i18n.T("Enable MCP server")).Changed() {
				on := a.settingsForm.mcpEnabled
				if on && !a.prefs.MCP.Acknowledged {
					a.settingsForm.mcpEnabled = false
					a.mcpEnableAsk = true
					return
				}
				a.updateSettings(func(s *settings.Settings) { s.MCP.Enabled = on })
			}
		})

	on := a.prefs.MCP.Enabled
	serving := on && a.mcpSrv != nil && a.mcpSrv.Serving()
	statusColor := k.Text3
	statusText := i18n.T("Off")
	if serving {
		statusColor = k.Success
		statusText = i18n.N(
			len(a.mcpLive),
			"Running · local socket · %d agent connected",
			"Running · local socket · %d agents connected",
			len(a.mcpLive),
		)
	} else if on {
		statusColor = k.Warning
		statusText = i18n.T("Starting…")
	}

	ui.Row(c.Key("integ-mcp-status")).AlignItems(ui.Center).Gap(px(t, sp2)).
		Padding(px(t, sp2), px(t, sp3)).Radius(px(t, radiusControl)).Background(k.Subtle).Border(1, k.Line).
		Children(func() {
			statusDot(c, statusColor)
			ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
				truncText(c, statusText).FontSize(px(t, fsCaption))
				if a.mcpTestText != "" {
					col := k.Success
					if a.mcpTestErr {
						col = k.Danger
					}
					truncText(c, a.mcpTestText).FontSize(px(t, fsMicro)).TextColor(col)
				}
			})
			btnLabel := i18n.T("Test connection")
			if a.mcpTestBusy {
				btnLabel = i18n.T("Testing…")
			}
			if ui.Button(c.Key("integ-mcp-test"), btnLabel).Disabled(!on || a.mcpTestBusy).Shrink(0).Clicked() {
				a.runMCPSelfTest()
			}
		})
}

func (a *app) mcpPermissionsCard(c *ui.Context) {
	t := c.Theme()
	on := a.prefs.MCP.Enabled

	sectionTitle(c, i18n.T("Permissions"))
	card(c).Padding(px(t, sp2), px(t, sp3)).Gap(px(t, sp2)).Children(func() {
		mcpPermCheck(c, "integ-mcp-perm-read", &a.settingsForm.mcpRead,
			i18n.T("Read"),
			i18n.T("Read status, history, dictionary, app groups, models and microphones. API keys are never exposed."),
			true)

		if mcpPermCheck(c, "integ-mcp-perm-manage", &a.settingsForm.mcpManage,
			i18n.T("Manage dictionary, history & rules"),
			i18n.T("Add, edit or delete terms, categories, app groups, or delete individual history entries."),
			!on) {
			v := a.settingsForm.mcpManage
			a.updateSettings(func(s *settings.Settings) { s.MCP.AllowManage = v })
		}

		if mcpPermCheck(c, "integ-mcp-perm-local", &a.settingsForm.mcpLocalModels,
			i18n.T("Call local models"),
			i18n.T("Invoke installed local speech and language models (including Apple models) for transcription, translation, rewrite or chat."),
			!on) {
			v := a.settingsForm.mcpLocalModels
			a.updateSettings(func(s *settings.Settings) {
				s.MCP.AllowLocalModels = v
				if !v {
					s.MCP.AllowRemoteModels = false
				}
			})
		}

		if mcpPermCheck(c, "integ-mcp-perm-remote", &a.settingsForm.mcpRemoteModels,
			i18n.T("Allow remote models"),
			i18n.T("Allow agents to call configured remote model services. Audio or text will leave this Mac."),
			!on || !a.prefs.MCP.AllowLocalModels) {
			v := a.settingsForm.mcpRemoteModels
			a.updateSettings(func(s *settings.Settings) { s.MCP.AllowRemoteModels = v })
		}

		if mcpPermCheck(c, "integ-mcp-perm-trigger", &a.settingsForm.mcpTrigger,
			i18n.T("Trigger recording sessions"),
			i18n.T("Start, stop, toggle or cancel a recording session and inspect session state."),
			!on) {
			v := a.settingsForm.mcpTrigger
			a.updateSettings(func(s *settings.Settings) { s.MCP.AllowTrigger = v })
		}
	})
}

func mcpPermCheck(c *ui.Context, key string, val *bool, label, tip string, disabled bool) bool {
	t := c.Theme()
	var changed bool
	ui.Row(c.Key(key + "-row")).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
		changed = ui.Checkbox(c.Key(key), val, "").Label(label).Disabled(disabled).Changed()
		labelTip(c, ui.Text(c, label).FontSize(px(t, fsBody)), label, px(t, fsBody), 400, tip)
	})
	return changed
}

func (a *app) mcpConnectAgentsCard(c *ui.Context) {
	t := c.Theme()
	k := tok(t)

	ui.Row(c).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
		sectionTitle(c, i18n.T("Connect agents"))
		ui.Spacer(c)
		if iconButton(c.Key("integ-mcp-setup-refresh"), icons.Refresh, i18n.T("Refresh")).Clicked() {
			a.loadMCPSetup()
		}
	})

	ctlPath := ctlbridge.CtlPath(a.resourcesDir)
	cmd := agentsetup.DefaultCommand(ctlPath)

	card(c).Padding(px(t, sp2), px(t, sp3)).Gap(px(t, sp2)).Children(func() {
		for i, cl := range a.mcpClients {
			if i > 0 {
				ui.Divider(c)
			}
			status := i18n.T("Not detected")
			statusCol := k.Text3
			if cl.Configured {
				status = i18n.T("Configured")
				statusCol = k.Success
			} else if cl.Installed {
				status = i18n.T("Detected")
				statusCol = k.Text2
			}
			ui.Row(c.Key("integ-mcp-cl-" + cl.ID)).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
					truncText(c, cl.Name).FontSize(px(t, fsBody))
					truncText(c, status).FontSize(px(t, fsMicro)).TextColor(statusCol)
				})
				addLabel := i18n.T("Add")
				if cl.Configured {
					addLabel = i18n.T("Update")
				}
				if ui.Button(c.Key("integ-mcp-add-"+cl.ID), addLabel).Shrink(0).Clicked() {
					a.openMCPConfigPreview(cl.ID, cl.Name, cmd)
				}
				if ui.Button(c.Key("integ-mcp-copy-"+cl.ID), i18n.T("Copy")).Shrink(0).Clicked() {
					c.Services().WriteClipboard(mcpConfigSnippet(cl.ID, cmd))
					a.toast(i18n.T("Copied"))
				}
			})
		}

		if len(a.mcpClients) > 0 {
			ui.Divider(c)
		}
		a.mcpSkillRow(c)
	})
}

func (a *app) mcpSkillRow(c *ui.Context) {
	t := c.Theme()
	k := tok(t)

	anyInstalled, allCurrent := false, true
	var targets []agentsetup.SkillTarget
	for _, st := range a.mcpSkills {
		if st.Found || st.Installed {
			targets = append(targets, st)
		}
		if st.Installed {
			anyInstalled = true
			if !st.Current {
				allCurrent = false
			}
		} else if st.Found {
			allCurrent = false
		}
	}
	status := i18n.T("Not installed")
	statusCol := k.Text3
	btnLabel := i18n.T("Install")
	if anyInstalled && allCurrent {
		status = i18n.T("Up to date")
		statusCol = k.Success
		btnLabel = i18n.T("Reinstall")
	} else if anyInstalled {
		status = i18n.T("Update available")
		statusCol = k.Warning
		btnLabel = i18n.T("Update")
	}

	ui.Row(c.Key("integ-mcp-skill")).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
			labelTip(c, ui.Text(c, i18n.T("VoxtMini skill")).FontSize(px(t, fsBody)),
				i18n.T("VoxtMini skill"), px(t, fsBody), 400,
				i18n.T("Installs SKILL.md so agents know how to use VoxtMini's tools cleanly."))
			truncText(c, status).FontSize(px(t, fsMicro)).TextColor(statusCol)
		})
		if ui.Button(c.Key("integ-mcp-skill-install"), btnLabel).Shrink(0).Clicked() {
			a.installMCPSkills(targets)
		}
	})
}

func (a *app) mcpKnownAgentsCard(c *ui.Context) {
	t := c.Theme()
	k := tok(t)

	sectionTitle(c, i18n.T("Connected agents"))
	liveSet := make(map[string]mcp.Client, len(a.mcpLive))
	for _, l := range a.mcpLive {
		liveSet[l.Key] = l
	}

	type item struct {
		key    string
		name   string
		live   bool
		grants string
	}
	seen := map[string]bool{}
	var items []item
	for _, cl := range a.prefs.MCP.Clients {
		seen[cl.Key] = true
		name := cl.App
		if name == "" {
			name = cl.Name
		}
		if name == "" {
			name = filepath.Base(cl.Path)
		}
		var gs []string
		if cl.AlwaysManage {
			gs = append(gs, i18n.T("Manage"))
		}
		if cl.AlwaysModels {
			gs = append(gs, i18n.T("Models"))
		}
		if cl.AlwaysTrigger {
			gs = append(gs, i18n.T("Trigger"))
		}
		_, live := liveSet[cl.Key]
		items = append(items, item{
			key:    cl.Key,
			name:   name,
			live:   live,
			grants: strings.Join(gs, " · "),
		})
	}
	for _, l := range a.mcpLive {
		if seen[l.Key] {
			continue
		}
		items = append(items, item{key: l.Key, name: l.Display(), live: true})
	}

	if len(items) == 0 {
		ui.Text(c, i18n.T("No agents connected yet.")).FontSize(px(t, fsCaption)).TextColor(k.Text2)
		return
	}
	card(c).Padding(px(t, sp2), px(t, sp3)).Gap(px(t, sp2)).Children(func() {
		for i, it := range items {
			if i > 0 {
				ui.Divider(c)
			}
			ui.Row(c.Key(fmt.Sprintf("integ-mcp-ag-%d", i))).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
				if it.live {
					statusDot(c, k.Success)
				} else {
					statusDot(c, k.Text3)
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
					truncText(c, it.name).FontSize(px(t, fsBody))
					sub := i18n.T("Connected")
					if !it.live {
						sub = i18n.T("Allowed")
					}
					if it.grants != "" {
						sub += " · " + it.grants
					}
					truncText(c, sub).FontSize(px(t, fsMicro)).TextColor(k.Text2)
				})
				if ui.Button(c.Key(fmt.Sprintf("integ-mcp-ask-%d", i)), i18n.T("Ask again")).Shrink(0).Clicked() {
					a.askAgainMCPClient(it.key)
					a.toast(i18n.T("Will ask again"))
				}
				if ui.Button(c.Key(fmt.Sprintf("integ-mcp-forget-%d", i)), i18n.T("Forget")).Shrink(0).Clicked() {
					a.forgetMCPClient(it.key)
				}
			})
		}
	})
}

func (a *app) mcpAuditCard(c *ui.Context) {
	t := c.Theme()
	k := tok(t)

	ui.Row(c).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
		sectionTitle(c, i18n.T("Records"))
		ui.Spacer(c)
		if iconButton(c.Key("integ-mcp-audit-refresh"), icons.Refresh, i18n.T("Refresh")).Clicked() {
			a.loadMCPAudit()
		}
		if ui.Button(c.Key("integ-mcp-audit-clear"), i18n.T("Clear")).Disabled(len(a.mcpAuditRows) == 0).Shrink(0).Clicked() {
			a.clearMCPAudit()
		}
	})

	if len(a.mcpAuditRows) == 0 {
		ui.Text(c, i18n.T("No records yet.")).FontSize(px(t, fsCaption)).TextColor(k.Text2)
		return
	}

	searchField(c, "integ-mcp-audit-filter", &a.mcpAuditFilt, i18n.T("Filter by agent, tool or result"), 0)

	q := strings.ToLower(strings.TrimSpace(a.mcpAuditFilt))
	var shown []store.MCPCall
	for _, r := range a.mcpAuditRows {
		if q != "" &&
			!strings.Contains(strings.ToLower(r.ClientName), q) &&
			!strings.Contains(strings.ToLower(r.Tool), q) &&
			!strings.Contains(strings.ToLower(r.Result), q) {
			continue
		}
		shown = append(shown, r)
		if len(shown) >= 20 {
			break
		}
	}

	card(c).Padding(px(t, sp2), px(t, sp3)).Gap(px(t, sp1)).Children(func() {
		if len(shown) == 0 {
			ui.Text(c, i18n.T("No matching records.")).FontSize(px(t, fsCaption)).TextColor(k.Text2)
			return
		}
		for i, r := range shown {
			if i > 0 {
				ui.Divider(c)
			}
			resCol := k.Success
			if r.Result != "ok" {
				resCol = k.Danger
			}
			ui.Row(c.Key(fmt.Sprintf("integ-mcp-rec-%d", i))).AlignItems(ui.Center).Gap(px(t, sp2)).Children(func() {
				ui.Text(c, r.At.Format("15:04:05")).FontSize(px(t, fsMicro)).TextColor(k.Text3).Shrink(0)
				truncText(c, r.ClientName).FontSize(px(t, fsCaption)).Width(px(t, 110)).Shrink(0)
				truncText(c, r.Tool).FontSize(px(t, fsCaption)).Grow(1).MinWidth(0)
				ui.Text(c, r.Result).FontSize(px(t, fsMicro)).TextColor(resCol).Shrink(0)
				ui.Text(c, fmt.Sprintf("%d ms", r.DurationMS)).FontSize(px(t, fsMicro)).TextColor(k.Text3).Shrink(0)
			})
		}
	})
}

func (a *app) mcpEnableConfirmDialog(c *ui.Context) {
	if !a.mcpEnableAsk {
		return
	}
	k := tok(c.Theme())
	dialogFrame(c, "integ-mcp-confirm", &a.mcpEnableAsk, i18n.T("Enable MCP server?"), "", 440, func() {
		ui.Text(c, i18n.T("Data read by connected AI agents is sent to their own model providers. VoxtMini only listens on a local Unix socket and never opens a network port.")).TextColor(k.Text2)
	}, func() {
		ui.Spacer(c)
		if ui.Button(c.Key("integ-mcp-confirm-cancel"), i18n.T("Cancel")).Clicked() {
			a.mcpEnableAsk = false
		}
		if primaryButton(c.Key("integ-mcp-confirm-ok"), i18n.T("Enable")).Clicked() {
			a.mcpEnableAsk = false
			a.updateSettings(func(s *settings.Settings) {
				s.MCP.Acknowledged = true
				s.MCP.Enabled = true
			})
		}
	})
}

func (a *app) mcpPreviewDialog(c *ui.Context) {
	if !a.mcpPrevOpen {
		return
	}
	t := c.Theme()
	k := tok(t)
	title := i18n.T("Configure %s", a.mcpPrevTitle)
	dialogFrame(c, "integ-mcp-preview", &a.mcpPrevOpen, title, a.mcpPrev.Path, 500, func() {
		if a.mcpPrevErr != "" {
			ui.Text(c, a.mcpPrevErr).TextColor(k.Danger).FontSize(px(t, fsCaption))
			return
		}
		if !a.mcpPrev.Changes {
			ui.Text(c, i18n.T("Already up to date.")).TextColor(k.Text2)
			return
		}
		ui.Text(c, i18n.T("A backup will be saved next to the existing file.")).FontSize(px(t, fsCaption)).TextColor(k.Text2)
		ui.Scroll(c.Key("integ-mcp-diff-scroll")).MaxHeight(px(t, 220)).
			Padding(px(t, sp2)).Radius(px(t, radiusControl)).Background(k.Subtle).Border(1, k.Line).
			Children(func() {
				ui.Text(c, a.mcpPrev.Diff).FontSize(px(t, fsMicro)).Selectable()
			})
	}, func() {
		ui.Spacer(c)
		if ui.Button(c.Key("integ-mcp-prev-cancel"), i18n.T("Cancel")).Clicked() {
			a.mcpPrevOpen = false
		}
		if a.mcpPrevErr == "" && a.mcpPrev.Changes {
			if primaryButton(c.Key("integ-mcp-prev-apply"), i18n.T("Apply")).Clicked() {
				clientID := a.mcpPrevClient
				cmd := agentsetup.DefaultCommand(ctlbridge.CtlPath(a.resourcesDir))
				a.mcpPrevOpen = false
				a.applyMCPConfig(clientID, cmd)
			}
		}
	})
}

func (a *app) openMCPConfigPreview(clientID, name string, cmd agentsetup.Command) {
	a.mcpPrevClient = clientID
	a.mcpPrevTitle = name
	a.mcpPrevErr = ""
	a.mcpPrev = agentsetup.PreviewResult{}
	a.async(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			a.post(func() {
				a.mcpPrevErr = err.Error()
				a.mcpPrevOpen = true
			})
			return
		}
		prev, err := agentsetup.Preview(home, clientID, cmd)
		a.post(func() {
			a.mcpPrev = prev
			if err != nil {
				a.mcpPrevErr = err.Error()
			}
			a.mcpPrevOpen = true
		})
	})
}

func (a *app) applyMCPConfig(clientID string, cmd agentsetup.Command) {
	a.async(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			a.post(func() { a.toast(err.Error()) })
			return
		}
		_, err = agentsetup.Write(home, clientID, cmd)
		a.post(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			a.toast(i18n.T("Configuration updated"))
			a.loadMCPSetup()
		})
	})
}

func (a *app) installMCPSkills(targets []agentsetup.SkillTarget) {
	ver := a.version
	a.async(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			a.post(func() { a.toast(err.Error()) })
			return
		}
		dirs := make([]string, 0, len(targets))
		for _, t := range targets {
			dirs = append(dirs, t.Dir)
		}
		if len(dirs) == 0 {
			dirs = []string{filepath.Join(home, ".agents", "skills", agentsetup.ServerName)}
		}
		skill := mcp.Skill(ver)
		for _, dir := range dirs {
			if err := agentsetup.InstallSkill(skill, dir); err != nil {
				a.post(func() { a.toast(err.Error()) })
				return
			}
		}
		a.post(func() {
			a.toast(i18n.T("Skill installed"))
			a.loadMCPSetup()
		})
	})
}

func mcpConfigSnippet(clientID string, cmd agentsetup.Command) string {
	if clientID == agentsetup.Codex {
		return fmt.Sprintf("[mcp_servers.voxtmini]\ncommand = %q\nargs = [%q]\n", cmd.Path, "mcp")
	}
	return fmt.Sprintf("{\n  \"mcpServers\": {\n    \"voxtmini\": {\n      \"command\": %q,\n      \"args\": [\"mcp\"]\n    }\n  }\n}\n", cmd.Path)
}
