package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/agentsetup"
	"github.com/hehehai/VoxtMini/internal/ctlbridge"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

func TestIntegrationsSettingsAndStateFile(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.dataHome = t.TempDir()
	a.openIntegrations()
	t.Cleanup(a.closeIntegrations)

	a.navigate("/settings/integrations")
	frames(tt)

	// MCP tab (default): first-time enable opens privacy confirmation dialog.
	if !has(tt, "Enable MCP server") {
		t.Fatalf("MCP tab missing toggle: %q", firstTexts(tt))
	}
	clickSwitchBeside(t, tt, "Enable MCP server", func() bool { return a.mcpEnableAsk })
	if !has(tt, "Enable MCP server?") {
		t.Fatalf("expected privacy confirmation dialog: %q", firstTexts(tt))
	}
	click(t, tt, "Enable")
	frames(tt)
	if !a.settings.Get().MCP.Enabled || !a.settings.Get().MCP.Acknowledged {
		t.Fatalf("MCP not enabled after confirmation: %+v", a.settings.Get().MCP)
	}
	if a.mcpSrv == nil || !a.mcpSrv.Serving() {
		t.Fatal("MCP server did not start listening")
	}
	if st := ctlbridge.ReadState(a.dataHome); !st.MCP || st.Trigger {
		t.Fatalf("run/mcp.json after MCP on: %+v", st)
	}

	// Switch to External trigger tab.
	click(t, tt, "External trigger")
	if !has(tt, "Allow external apps to trigger") || !has(tt, "Quick reference") {
		t.Fatalf("Trigger tab missing controls: %q", firstTexts(tt))
	}
	clickSwitchBeside(t, tt, "Allow external apps to trigger", func() bool { return a.settings.Get().Trigger.Enabled })
	if st := ctlbridge.ReadState(a.dataHome); !st.MCP || !st.Trigger {
		t.Fatalf("run/mcp.json after Trigger on: %+v", st)
	}
	if a.ctrlSrv == nil {
		t.Fatal("control socket server did not start when Trigger enabled")
	}
	if _, err := os.Stat(a.ctrlSrv.Path()); err != nil {
		t.Fatalf("control socket file missing: %v", err)
	}

	// Sub-toggle: Allow URL triggers.
	clickSwitchBeside(t, tt, "Allow URL triggers", func() bool { return !a.settings.Get().Trigger.AllowURLOn() })

	// Scroll down to verify quick reference and microphone list (loaded from helper).
	tt.Scroll(windowWidth/2, windowHeight/2, 0, 300)
	frames(tt)
	if !has(tt, "BuiltInMicrophoneDevice") || !has(tt, "AirPodsPro:input") {
		t.Fatalf("microphone list missing devices: %q", tt.Texts())
	}
	click(t, tt, "Copy UID")
	if got := tt.Clipboard(); got != "BuiltInMicrophoneDevice" {
		t.Fatalf("clipboard after Copy UID = %q, want BuiltInMicrophoneDevice", got)
	}

	// Scroll back up and disable Trigger -> socket server stops cleanly.
	tt.Scroll(windowWidth/2, windowHeight/2, 0, -300)
	frames(tt)
	clickSwitchBeside(t, tt, "Allow external apps to trigger", func() bool { return !a.settings.Get().Trigger.Enabled })
	if a.ctrlSrv != nil {
		t.Fatal("control socket server still running after Trigger disabled")
	}
	if st := ctlbridge.ReadState(a.dataHome); !st.MCP || st.Trigger {
		t.Fatalf("run/mcp.json after Trigger off: %+v", st)
	}
}

func clickSwitchBeside(t *testing.T, tt *ui.Tester, title string, done func() bool) {
	t.Helper()
	frames(tt)
	r, ok := tt.Find(title)
	if !ok {
		t.Fatalf("no %q: %q", title, firstTexts(tt))
	}
	dw, _ := settingsDialogSize(ui.LightTheme(), windowWidth, windowHeight)
	right := (windowWidth + dw) / 2
	for x := right - 16; x > right-120; x -= 4 {
		tt.ClickAt(x, r.Y+r.H/2)
		frames(tt)
		if done() {
			return
		}
	}
	t.Fatalf("no switch beside %q at %+v", title, r)
}

func TestTriggerURLDoesNotNavigateOrActivate(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.dataHome = t.TempDir()
	a.openIntegrations()
	t.Cleanup(a.closeIntegrations)

	a.navigate("/home")
	frames(tt)
	beforePath := a.shownPath()

	// Disabled: ignored with toast, no navigation.
	a.openURL("voxtmini://trigger/transcribe?action=toggle&device=usb-mic-uid")
	if a.shownPath() != beforePath || a.settingsOpen {
		t.Fatalf("trigger URL changed route: %s settingsOpen=%v", a.shownPath(), a.settingsOpen)
	}
	if len(a.toasts) == 0 {
		t.Fatal("expected toast when trigger disabled")
	}

	// Enabled but AllowURL=false: ignored with toast.
	a.toasts = nil
	off := false
	a.updateSettings(func(s *settings.Settings) {
		s.Trigger.Enabled = true
		s.Trigger.AllowURL = &off
	})
	a.openURL("voxtmini://trigger/transcribe?action=toggle")
	if a.shownPath() != beforePath || len(a.toasts) == 0 {
		t.Fatalf("URL disabled: path=%s toasts=%v", a.shownPath(), a.toasts)
	}
}

func TestIntegrationsSnapshots(t *testing.T) {
	out := os.Getenv("VOXTMINI_INTEG_SHOTS")
	if out == "" {
		t.Skip("set VOXTMINI_INTEG_SHOTS=DIR to write Integrations screenshots")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "zh-Hans"} {
		a, _, tt := newTestApp(t)
		a.dataHome = t.TempDir()
		a.openIntegrations()
		t.Cleanup(a.closeIntegrations)
		tt.SetScale(2)
		if lang != "en" {
			a.updateSettings(func(s *settings.Settings) { s.General.Language = lang })
		}
		a.updateSettings(func(s *settings.Settings) {
			s.MCP.Enabled = true
			s.MCP.Acknowledged = true
			s.MCP.AllowManage = true
			s.MCP.AllowLocalModels = true
			s.MCP.Clients = []settings.MCPClient{
				{Key: "/Applications/Codex.app|TEAM123", App: "Codex", Path: "/Applications/Codex.app", AlwaysManage: true},
			}
			s.Trigger.Enabled = true
		})
		a.navigate("/settings/integrations")
		a.mcpTestText = i18n.T("✓ %d tools, %d ms", 16, 4)
		a.mcpClients = []agentsetup.ClientStatus{
			{ID: agentsetup.Codex, Name: "Codex / ChatGPT", Installed: true, Configured: true},
			{ID: agentsetup.ClaudeCode, Name: "Claude Code", Installed: true, Configured: false},
			{ID: agentsetup.ClaudeDesktop, Name: "Claude Desktop", Installed: false, Configured: false},
			{ID: agentsetup.Cursor, Name: "Cursor", Installed: true, Configured: false},
			{ID: agentsetup.WorkBuddy, Name: "WorkBuddy", Installed: false, Configured: false},
		}
		a.mcpSkills = []agentsetup.SkillTarget{
			{ID: "agents", Name: "Codex", Found: true, Installed: true, Current: true},
		}
		now := time.Date(2026, 11, 25, 14, 30, 0, 0, time.Local)
		a.mcpAuditRows = []store.MCPCall{
			{ID: 2, At: now, ClientName: "Codex", Tool: "upsert_terms", Scope: "manage", Count: 2, Result: "ok", DurationMS: 9},
			{ID: 1, At: now.Add(-time.Minute), ClientName: "Codex", Tool: "voxtmini_overview", Scope: "read", Count: 1, Result: "ok", DurationMS: 4},
		}
		save := func(name string) {
			t.Helper()
			frames(tt)
			f, err := os.Create(filepath.Join(out, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
		a.integTab = 0
		save("integrations-mcp-top-" + lang)
		tt.Scroll(windowWidth/2, windowHeight/2, 0, 500)
		save("integrations-mcp-bottom-" + lang)
		tt.Scroll(windowWidth/2, windowHeight/2, 0, -500)
		a.integTab = 1
		save("integrations-trigger-" + lang)
	}
}
