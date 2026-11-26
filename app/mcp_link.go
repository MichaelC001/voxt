package main

// MCP server lifecycle, SQLite audit adapter, self-test, and agent/skill
// detection for Settings → Integrations → MCP.

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/hehehai/VoxtMini/internal/agentsetup"
	"github.com/hehehai/VoxtMini/internal/ctlbridge"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/mcp"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

const mcpAuditLimit = 100

// mcpState holds MCP server handles and Settings → Integrations → MCP view state.
type mcpState struct {
	mcpSrv          *mcp.Server
	mcpExtraBridges []string // extra allowed bridge paths (tests)
	mcpInspector    mcp.Inspector
	mcpPrompter     mcp.Prompter // overrides default UI prompter (tests)

	mcpAsks      []*mcpAsk
	mcpLive      []mcp.Client
	mcpAuditRows []store.MCPCall
	mcpAuditFilt string

	mcpEnableAsk bool // first-time privacy confirmation dialog

	mcpTestBusy bool
	mcpTestText string
	mcpTestErr  bool

	mcpClients []agentsetup.ClientStatus
	mcpSkills  []agentsetup.SkillTarget

	mcpPrevOpen   bool
	mcpPrevClient string
	mcpPrevTitle  string
	mcpPrev       agentsetup.PreviewResult
	mcpPrevErr    string
}

// openMCP creates the MCP server instance; syncMCP starts or stops listening.
func (a *app) openMCP() {
	if a.mcpSrv != nil {
		return
	}
	var bridges []string
	if exe, err := os.Executable(); err == nil && exe != "" {
		bridges = append(bridges, exe)
	}
	if ctl := ctlbridge.CtlPath(a.resourcesDir); ctl != "" {
		bridges = append(bridges, ctl)
	}
	bridges = append(bridges, a.mcpExtraBridges...)

	prompter := a.mcpPrompter
	if prompter == nil {
		prompter = mcpPrompter{a: a}
	}
	a.mcpSrv = mcp.New(mcp.Options{
		Host:      mcpHost{a: a},
		Prompter:  prompter,
		Inspector: a.mcpInspector,
		Audit:     mcpAudit{a: a},
		Version:   a.version,
		Bridges:   bridges,
	})
}

// syncMCP starts/stops run/mcp.sock and rescopes live sessions according to
// a.prefs.MCP.
func (a *app) syncMCP() {
	if a.mcpSrv == nil {
		return
	}
	home, cache := a.dataHome, a.cacheDir
	on := a.prefs.MCP.Enabled
	if on && home != "" {
		if !a.mcpSrv.Serving() {
			path := ctlbridge.MCPSocketPath(home, cache)
			if err := a.mcpSrv.Listen(path); err != nil {
				a.logTrigger("warning", "starting MCP socket: "+err.Error())
			}
		}
		a.mcpSrv.Rescope()
	} else if a.mcpSrv.Serving() {
		a.failMCPAsks()
		srv := a.mcpSrv
		a.async(func() { _ = srv.Close() })
		a.mcpLive = nil
	}
}

// closeMCP stops the MCP server and cancels pending confirmation dialogs.
func (a *app) closeMCP() {
	a.failMCPAsks()
	if a.mcpSrv != nil {
		_ = a.mcpSrv.Close()
		a.mcpSrv = nil
	}
}

// refreshMCPStatus updates connected clients and audit log snapshots.
func (a *app) refreshMCPStatus() {
	if a.mcpSrv != nil {
		a.mcpLive = a.mcpSrv.Live()
	} else {
		a.mcpLive = nil
	}
	a.loadMCPAudit()
}

func (a *app) loadMCPAudit() {
	if a.lib == nil || a.lib.store == nil {
		return
	}
	st := a.lib.store.MCPCalls()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		rows, err := st.List(ctx, store.MCPCallQuery{Limit: mcpAuditLimit})
		if err != nil {
			return
		}
		a.post(func() { a.mcpAuditRows = rows })
	})
}

func (a *app) clearMCPAudit() {
	if a.lib == nil || a.lib.store == nil {
		return
	}
	st := a.lib.store.MCPCalls()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := st.Clear(ctx); err != nil {
			a.post(func() { a.toast(i18n.T("Could not clear records: %v", err)) })
			return
		}
		a.post(func() {
			a.mcpAuditRows = nil
			a.toast(i18n.T("Records cleared"))
		})
	})
}

func (a *app) loadMCPSetup() {
	ver := a.version
	a.async(func() {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return
		}
		clients := agentsetup.DetectClients(home)
		skills := agentsetup.SkillTargets(home, mcp.Skill(ver))
		a.post(func() {
			a.mcpClients = clients
			a.mcpSkills = skills
		})
	})
}

func (a *app) runMCPSelfTest() {
	if a.mcpTestBusy {
		return
	}
	home, cache, ver := a.dataHome, a.cacheDir, a.version
	if home == "" {
		a.mcpTestErr = true
		a.mcpTestText = i18n.T("MCP server is not running")
		return
	}
	a.mcpTestBusy = true
	a.mcpTestErr = false
	a.mcpTestText = ""
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		path := ctlbridge.MCPSocketPath(home, cache)
		res, err := mcp.SelfTest(ctx, path, ver)
		a.post(func() {
			a.mcpTestBusy = false
			if err != nil {
				a.mcpTestErr = true
				a.mcpTestText = err.Error()
				return
			}
			a.mcpTestErr = false
			a.mcpTestText = i18n.T("✓ %d tools, %d ms", res.Tools, res.Took.Milliseconds())
		})
	})
}

func (a *app) askAgainMCPClient(key string) {
	if a.mcpSrv != nil {
		a.mcpSrv.AskAgain(key)
	}
	a.updateSettings(func(s *settings.Settings) {
		for i := range s.MCP.Clients {
			if s.MCP.Clients[i].Key == key {
				s.MCP.Clients[i].AlwaysManage = false
				s.MCP.Clients[i].AlwaysModels = false
				s.MCP.Clients[i].AlwaysTrigger = false
			}
		}
	})
}

func (a *app) forgetMCPClient(key string) {
	if a.mcpSrv != nil {
		a.mcpSrv.Forget(key)
	}
	a.updateSettings(func(s *settings.Settings) {
		s.MCP.Clients = slices.DeleteFunc(s.MCP.Clients, func(c settings.MCPClient) bool { return c.Key == key })
	})
	a.refreshMCPStatus()
}

// mcpAudit adapts store.MCPCalls to mcp.Audit.
type mcpAudit struct{ a *app }

func (m mcpAudit) Record(ctx context.Context, rec mcp.CallRecord) {
	if m.a.lib == nil || m.a.lib.store == nil {
		return
	}
	_ = m.a.lib.store.MCPCalls().Record(ctx, store.MCPCall{
		At:         rec.At,
		ClientKey:  rec.ClientKey,
		ClientName: rec.ClientName,
		Tool:       rec.Tool,
		Scope:      rec.Scope,
		Count:      rec.Count,
		Result:     rec.Result,
		DurationMS: rec.DurationMS,
		Bytes:      rec.Bytes,
	})
}
