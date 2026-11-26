package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/ctlbridge"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/mcp"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

type fakeMCPInspector struct {
	peer   mcp.Proc
	parent mcp.Proc
	team   string
}

func (f fakeMCPInspector) Peer(net.Conn) (mcp.Proc, error) { return f.peer, nil }
func (f fakeMCPInspector) Proc(int) (mcp.Proc, error)      { return f.parent, nil }
func (f fakeMCPInspector) Team(string) string              { return f.team }

type allowAllPrompter struct {
	asks []mcp.Question
	ans  mcp.Answer
}

func (p *allowAllPrompter) Ask(_ context.Context, q mcp.Question) mcp.Answer {
	p.asks = append(p.asks, q)
	if p.ans != 0 {
		return p.ans
	}
	return mcp.AllowAlways
}

func TestMCPSelfTestEndToEndViaCtlBridge(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.dataHome = t.TempDir()
	a.openIntegrations()
	t.Cleanup(a.closeIntegrations)

	// Disabled first -> ctlbridge.SelfTest reports ErrMCPOff.
	if _, err := ctlbridge.SelfTest(ctlbridge.BridgeOptions{Home: a.dataHome}); err == nil {
		t.Fatal("expected error when MCP is disabled")
	}

	// Enable MCP -> run/mcp.sock starts listening and run/mcp.json has mcp=true.
	a.updateSettings(func(s *settings.Settings) {
		s.MCP.Enabled = true
		s.MCP.Acknowledged = true
	})
	if !a.mcpSrv.Serving() {
		t.Fatal("MCP server not serving after enable")
	}

	// 1. ctlbridge.SelfTest (what `voxtmini-ctl mcp --self-test` executes).
	summary, err := ctlbridge.SelfTest(ctlbridge.BridgeOptions{Home: a.dataHome})
	if err != nil {
		t.Fatalf("ctlbridge.SelfTest: %v", err)
	}
	if !strings.HasPrefix(summary, "ok:") || !strings.Contains(summary, "tools") {
		t.Fatalf("unexpected SelfTest summary: %q", summary)
	}

	// 2. mcp.SelfTest (what Settings "Test connection" executes: also calls voxtmini_overview).
	sock := ctlbridge.MCPSocketPath(a.dataHome, a.cacheDir)
	res, err := mcp.SelfTest(context.Background(), sock, "test")
	if err != nil {
		t.Fatalf("mcp.SelfTest: %v", err)
	}
	if res.Tools < 6 {
		t.Fatalf("expected >= 6 read tools, got %d", res.Tools)
	}
}

func TestMCPWiredToolsAndSQLiteAudit(t *testing.T) {
	a, _, _ := newTestApp(t)
	a.dataHome = t.TempDir()
	a.local = &fakeLocal{
		ASRP: &fakeTranscriptions{text: "繁體測試 Voxt Mini"},
		Chat: fakeChatFunc(func(_ context.Context, _ llm.Request) (llm.Response, error) {
			return llm.Response{Text: "繁體翻譯結果"}, nil
		}),
	}

	const bridgePath = "/fake/VoxtMini.app/Contents/Resources/ctl/voxtmini-ctl"
	a.mcpExtraBridges = []string{bridgePath}
	a.mcpInspector = fakeMCPInspector{
		peer:   mcp.Proc{PID: 99999, PPID: 88888, UID: os.Getuid(), Exe: bridgePath},
		parent: mcp.Proc{PID: 88888, PPID: 1, UID: os.Getuid(), Exe: "/Applications/Codex.app/Contents/MacOS/Codex"},
		team:   "TEAMTEST",
	}
	prompter := &allowAllPrompter{ans: mcp.AllowAlways}
	a.mcpPrompter = prompter
	a.openIntegrations()
	t.Cleanup(a.closeIntegrations)

	a.updateSettings(func(s *settings.Settings) {
		s.MCP.Enabled = true
		s.MCP.Acknowledged = true
		s.MCP.AllowManage = true
		s.MCP.AllowLocalModels = true
		s.MCP.AllowTrigger = true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sock := ctlbridge.MCPSocketPath(a.dataHome, a.cacheDir)
	nc, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial mcp.sock: %v", err)
	}
	defer nc.Close()
	if _, err := nc.Write([]byte(ctlbridge.Hello + "\n")); err != nil {
		t.Fatal(err)
	}

	client := sdk.NewClient(&sdk.Implementation{Name: "codex-test", Version: "1.0"}, nil)
	sess, err := client.Connect(ctx, &sdk.IOTransport{Reader: nc, Writer: nc}, nil)
	if err != nil {
		t.Fatalf("sdk connect: %v", err)
	}
	defer sess.Close()

	// All 4 scopes enabled -> 20 tools (8 read + 6 manage + 4 models + 2 trigger).
	toolsRes, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(toolsRes.Tools) != 20 {
		t.Fatalf("got %d tools, want 20", len(toolsRes.Tools))
	}

	// Upsert a dictionary term and read it back.
	upRes, err := sess.CallTool(ctx, &sdk.CallToolParams{
		Name: "upsert_terms",
		Arguments: map[string]any{
			"terms": []map[string]any{
				{"spelling": "VoxtMini", "hints": []string{"Voxt Mini"}},
			},
		},
	})
	if err != nil || upRes.IsError {
		t.Fatalf("upsert_terms: err=%v res=%+v", err, upRes)
	}

	// Upsert an app group and list it back.
	grpRes, err := sess.CallTool(ctx, &sdk.CallToolParams{
		Name: "upsert_app_group",
		Arguments: map[string]any{
			"name":    "Editors",
			"apps":    []string{"com.apple.TextEdit"},
			"domains": []string{"github.com"},
			"prompt":  "Keep concise.",
		},
	})
	if err != nil || grpRes.IsError {
		t.Fatalf("upsert_app_group: err=%v res=%+v", err, grpRes)
	}

	// Transcribe a local WAV file in temp dir: should apply dict alias + Simplified Chinese.
	wavPath := filepath.Join(t.TempDir(), "sample.wav")
	if err := os.WriteFile(wavPath, asr.WAV(make([]byte, 3200), asr.SampleRate), 0o600); err != nil {
		t.Fatal(err)
	}
	trRes, err := sess.CallTool(ctx, &sdk.CallToolParams{
		Name:      "transcribe_audio",
		Arguments: map[string]any{"audio_path": wavPath},
	})
	if err != nil || trRes.IsError {
		t.Fatalf("transcribe_audio: err=%v res=%+v", err, trRes)
	}
	var trOut mcp.TranscribeResult
	if err := json.Unmarshal([]byte(trRes.Content[0].(*sdk.TextContent).Text), &trOut); err != nil {
		t.Fatal(err)
	}
	if trOut.Text != "繁体测试 VoxtMini" {
		t.Fatalf("transcribe_audio text = %q, want %q", trOut.Text, "繁体测试 VoxtMini")
	}

	// Translate text: Simplified Chinese normalization applied.
	tlRes, err := sess.CallTool(ctx, &sdk.CallToolParams{
		Name:      "translate_text",
		Arguments: map[string]any{"text": "hello", "target_language": "zh-Hans"},
	})
	if err != nil || tlRes.IsError {
		t.Fatalf("translate_text: err=%v res=%+v", err, tlRes)
	}
	var tlOut mcp.TranslateResult
	_ = json.Unmarshal([]byte(tlRes.Content[0].(*sdk.TextContent).Text), &tlOut)
	if tlOut.Text != "繁体翻译结果" {
		t.Fatalf("translate_text text = %q, want %q", tlOut.Text, "繁体翻译结果")
	}

	// Remote model blocked when AllowRemoteModels is false.
	remRes, err := sess.CallTool(ctx, &sdk.CallToolParams{
		Name:      "chat_model",
		Arguments: map[string]any{"prompt": "hi", "model_id": settings.RemoteID("openai-1")},
	})
	if err != nil || !remRes.IsError {
		t.Fatalf("expected remote_off error, got err=%v res=%+v", err, remRes)
	}

	// Audit log recorded in SQLite mcp_calls table.
	rows, err := a.lib.store.MCPCalls().List(ctx, store.MCPCallQuery{})
	if err != nil {
		t.Fatalf("MCPCalls.List: %v", err)
	}
	if len(rows) < 5 {
		t.Fatalf("expected >= 5 audit rows in SQLite, got %d", len(rows))
	}

	// Dynamic Rescope: turning off AllowManage removes manage tools immediately.
	a.updateSettings(func(s *settings.Settings) { s.MCP.AllowManage = false })
	toolsRes2, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools after rescope: %v", err)
	}
	if len(toolsRes2.Tools) != 14 {
		t.Fatalf("got %d tools after disabling AllowManage, want 14", len(toolsRes2.Tools))
	}
}

func TestMCPAskDialogConnectAndManage(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/home")
	frames(tt)

	// Connect prompt -> Always allow persists client in settings.MCP.Clients.
	ansCh := make(chan mcp.Answer, 1)
	cl := mcp.Client{Key: "/Applications/Codex.app|TEAM1", App: "Codex", Path: "/Applications/Codex.app", Team: "TEAM1"}
	ask := &mcpAsk{q: mcp.Question{Kind: mcp.AskConnect, Client: cl}, answer: ansCh, open: true}
	a.mcpAsks = []*mcpAsk{ask}
	frames(tt)
	if !has(tt, "Always allow") {
		t.Fatalf("connect dialog missing Always allow: %q", firstTexts(tt))
	}
	click(t, tt, "Always allow")
	if got := <-ansCh; got != mcp.AllowAlways {
		t.Fatalf("connect answer = %v, want AllowAlways", got)
	}
	if len(a.settings.Get().MCP.Clients) != 1 || a.settings.Get().MCP.Clients[0].Key != cl.Key {
		t.Fatalf("client not persisted: %+v", a.settings.Get().MCP.Clients)
	}

	// Manage prompt -> Allow 1 min returns AllowMinute.
	ansCh2 := make(chan mcp.Answer, 1)
	ask2 := &mcpAsk{
		q:      mcp.Question{Kind: mcp.AskManage, Client: cl, Tool: "upsert_terms", Summary: "Add 2 terms"},
		answer: ansCh2,
		open:   true,
	}
	a.mcpAsks = []*mcpAsk{ask2}
	frames(tt)
	if !has(tt, "Allow 1 min") || !has(tt, "Allow once") {
		t.Fatalf("manage dialog missing buttons: %q", firstTexts(tt))
	}
	click(t, tt, "Allow 1 min")
	if got := <-ansCh2; got != mcp.AllowMinute {
		t.Fatalf("manage answer = %v, want AllowMinute", got)
	}
}

type fakeChatFunc func(ctx context.Context, req llm.Request) (llm.Response, error)

func (f fakeChatFunc) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	return f(ctx, req)
}
