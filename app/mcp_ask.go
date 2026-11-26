package main

// In-app confirmation modal for MCP agent approval and action gates
// (Allow once / Allow 1 min / Always allow / Deny).

import (
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/mcp"
	"github.com/hehehai/VoxtMini/internal/settings"
)

type mcpAsk struct {
	q      mcp.Question
	answer chan mcp.Answer
	open   bool
}

type mcpPrompter struct{ a *app }

func (p mcpPrompter) Ask(ctx context.Context, q mcp.Question) mcp.Answer {
	ask := &mcpAsk{q: q, answer: make(chan mcp.Answer, 1), open: true}
	p.a.post(func() {
		p.a.mcpAsks = append(p.a.mcpAsks, ask)
		if p.a.win != nil {
			p.a.win.Show()
			p.a.win.Focus()
		}
	})
	select {
	case ans := <-ask.answer:
		return ans
	case <-ctx.Done():
		p.a.post(func() { p.a.dropMCPAsk(ask) })
		return mcp.Unanswered
	}
}

func (a *app) dropMCPAsk(ask *mcpAsk) {
	a.mcpAsks = slices.DeleteFunc(a.mcpAsks, func(x *mcpAsk) bool { return x == ask })
}

func (a *app) answerMCPAsk(ask *mcpAsk, ans mcp.Answer) {
	cl := ask.q.Client
	if ask.q.Kind == mcp.AskConnect && ans != mcp.Deny && ans != mcp.Unanswered {
		a.updateSettings(func(s *settings.Settings) {
			s.MCP.Clients = slices.DeleteFunc(s.MCP.Clients, func(c settings.MCPClient) bool { return c.Key == cl.Key })
			s.MCP.Clients = append(s.MCP.Clients, settings.MCPClient{
				Key:     cl.Key,
				Name:    cl.Name,
				App:     cl.App,
				Path:    cl.Path,
				Team:    cl.Team,
				Allowed: time.Now(),
			})
		})
	}
	if ans == mcp.AllowAlways && ask.q.Kind != mcp.AskConnect {
		kind := ask.q.Kind
		a.updateSettings(func(s *settings.Settings) {
			found := false
			for i := range s.MCP.Clients {
				if s.MCP.Clients[i].Key == cl.Key {
					found = true
					setMCPGrant(&s.MCP.Clients[i], kind)
				}
			}
			if !found {
				mc := settings.MCPClient{
					Key:     cl.Key,
					Name:    cl.Name,
					App:     cl.App,
					Path:    cl.Path,
					Team:    cl.Team,
					Allowed: time.Now(),
				}
				setMCPGrant(&mc, kind)
				s.MCP.Clients = append(s.MCP.Clients, mc)
			}
		})
	}
	a.dropMCPAsk(ask)
	ask.answer <- ans
}

func setMCPGrant(c *settings.MCPClient, kind mcp.AskKind) {
	switch kind {
	case mcp.AskManage:
		c.AlwaysManage = true
	case mcp.AskModel:
		c.AlwaysModels = true
	case mcp.AskTrigger:
		c.AlwaysTrigger = true
	}
}

func (a *app) failMCPAsks() {
	for _, ask := range a.mcpAsks {
		ask.answer <- mcp.Unanswered
	}
	a.mcpAsks = nil
}

func (a *app) mcpAskDialog(c *ui.Context) {
	if len(a.mcpAsks) == 0 {
		return
	}
	ask := a.mcpAsks[0]
	t := c.Theme()
	k := tok(t)
	cl := ask.q.Client
	name := mcpAskName(cl)
	defer func() {
		if !ask.open && slices.Contains(a.mcpAsks, ask) {
			a.answerMCPAsk(ask, mcp.Deny)
		}
	}()

	if ask.q.Kind == mcp.AskConnect {
		dialogFrame(c, "mcp-ask", &ask.open, i18n.T("Allow “%s” to connect?", name), "", 460, func() {
			ui.Text(c, i18n.T("An AI agent in this program asks to use VoxtMini. Data it reads is sent to its own model provider.")).TextColor(k.Text2)
			mcpDetailRow(c, i18n.T("Program"), cl.Path)
			if cl.Name != "" {
				mcpDetailRow(c, i18n.T("Agent"), cl.Name)
			}
			team := cl.Team
			if team == "" {
				team = i18n.T("Not signed by a developer team")
			}
			mcpDetailRow(c, i18n.T("Team ID"), team)
		}, func() {
			ui.Spacer(c)
			if ui.Button(c.Key("mcp-ask-deny"), i18n.T("Deny")).Clicked() {
				a.answerMCPAsk(ask, mcp.Deny)
			}
			if primaryButton(c.Key("mcp-ask-allow"), i18n.T("Always allow")).Clicked() {
				a.answerMCPAsk(ask, mcp.AllowAlways)
			}
		})
		return
	}

	title := mcpActionTitle(ask.q.Kind, name)
	dialogFrame(c, "mcp-ask", &ask.open, title, "", 480, func() {
		if ask.q.Summary != "" {
			ui.Text(c, ask.q.Summary).FontWeight(500)
		}
		if ask.q.Tool != "" {
			mcpDetailRow(c, i18n.T("Tool"), ask.q.Tool)
		}
		mcpDetailRow(c, i18n.T("Agent"), cl.Display())
	}, func() {
		if ui.Button(c.Key("mcp-ask-deny"), i18n.T("Deny")).Clicked() {
			a.answerMCPAsk(ask, mcp.Deny)
		}
		ui.Spacer(c)
		if !ask.q.EveryTime {
			if ui.Button(c.Key("mcp-ask-min"), i18n.T("Allow 1 min")).Clicked() {
				a.answerMCPAsk(ask, mcp.AllowMinute)
			}
			if ui.Button(c.Key("mcp-ask-always"), i18n.T("Always allow")).Clicked() {
				a.answerMCPAsk(ask, mcp.AllowAlways)
			}
		}
		if primaryButton(c.Key("mcp-ask-once"), i18n.T("Allow once")).Clicked() {
			a.answerMCPAsk(ask, mcp.AllowOnce)
		}
	})
}

func mcpDetailRow(c *ui.Context, label, value string) {
	t := c.Theme()
	k := tok(t)
	ui.Row(c).Gap(px(t, sp2)).Children(func() {
		ui.Text(c, label).TextColor(k.Text3).Width(px(t, 88)).Shrink(0).FontSize(px(t, fsCaption))
		ui.Text(c, value).Selectable().Grow(1).MinWidth(0).FontSize(px(t, fsCaption))
	})
}

func mcpActionTitle(kind mcp.AskKind, name string) string {
	switch kind {
	case mcp.AskManage:
		return i18n.T("Allow “%s” to make changes?", name)
	case mcp.AskModel:
		return i18n.T("Allow “%s” to run a model?", name)
	case mcp.AskTrigger:
		return i18n.T("Allow “%s” to trigger a session?", name)
	}
	return i18n.T("Allow “%s”?", name)
}

func mcpAskName(cl mcp.Client) string {
	if cl.App != "" && isInstalledApp(cl.Path) {
		return cl.App
	}
	p := strings.TrimSuffix(cl.Path, "/")
	if p == "" {
		return cl.Display()
	}
	if i := strings.LastIndex(p, "/"); i > 0 {
		if j := strings.LastIndex(p[:i], "/"); j >= 0 {
			return p[j+1:]
		}
	}
	return p
}

func isInstalledApp(path string) bool {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+"/Applications/") {
		return true
	}
	return strings.HasPrefix(path, "/Applications/") || strings.HasPrefix(path, "/System/Applications/")
}
