package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBothProtocols verifies both 2026-07-28 (SDK client) and 2025-11-25
// (initialize) handshakes, plus {"app":...,"instance":...,"v":1} and {"bridge":1}.
func TestBothProtocols(t *testing.T) {
	r := newRig(t, Policy{Enabled: true}, true)
	cs := r.connect("new-client")
	if v := cs.InitializeResult().ProtocolVersion; v != "2026-07-28" {
		t.Errorf("SDK client protocol = %s, want 2026-07-28", v)
	}
	if out, isErr := callTool(t, cs, "voxtmini_overview", nil); isErr || out["permissions"] == nil {
		t.Fatalf("overview failed: %v", out)
	}

	conn, err := net.Dial("unix", r.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(`{"bridge":1}` + "\n"))
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"codex-mcp-client","version":"0.1"}}}` + "\n"))
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, `"protocolVersion":"2025-11-25"`) || !strings.Contains(line, `"instructions"`) {
		t.Fatalf("2025-11-25 initialize response: %s", line)
	}
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"voxtmini_overview","arguments":{}}}` + "\n"))
	line, err = br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, `\"permissions\"`) || strings.Contains(line, `"isError":true`) {
		t.Fatalf("2025-11-25 call response: %s", line)
	}

	names := map[string]bool{}
	for _, e := range r.audit.Entries() {
		names[e.ClientName] = true
	}
	if !names["ChatGPT (new-client)"] || !names["ChatGPT (codex-mcp-client)"] {
		t.Errorf("expected both clients in audit, got %v", names)
	}
}

func TestRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*rig)
		want string
	}{
		{"mcp off", func(r *rig) { r.host.mu.Lock(); r.host.policy.Enabled = false; r.host.mu.Unlock() }, "turned off"},
		{"not ready", func(r *rig) { r.host.mu.Lock(); r.host.ready = errors.New("starting up"); r.host.mu.Unlock() }, "starting up"},
		{"not bridge", func(r *rig) { r.ins.mu.Lock(); r.ins.peer.Exe = "/usr/bin/python3"; r.ins.mu.Unlock() }, "bridge"},
		{"other user", func(r *rig) { r.ins.mu.Lock(); r.ins.peer.UID = os.Getuid() + 1; r.ins.mu.Unlock() }, "another user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, Policy{Enabled: true}, true)
			tc.set(r)
			conn, err := net.Dial("unix", r.path)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_, _ = conn.Write([]byte(BridgeHelloLine + "\n" + `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}` + "\n"))
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			line, _ := bufio.NewReader(conn).ReadString('\n')
			var resp struct {
				ID    int `json:"id"`
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal([]byte(line), &resp)
			if resp.ID != 7 || !strings.Contains(resp.Error.Message, tc.want) {
				t.Fatalf("got %q, want error containing %q", line, tc.want)
			}
		})
	}
}

func TestGateConnectAndCoalesce(t *testing.T) {
	r := newRig(t, Policy{Enabled: true}, false)
	gateCh := make(chan struct{})
	r.prompter.fn = func(ctx context.Context, q Question) Answer {
		<-gateCh
		return AllowAlways
	}

	cs1 := r.connect("bridge-1")
	cs2 := r.connect("bridge-2")
	var wg sync.WaitGroup
	errs := make([]bool, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = callTool(t, cs1, "voxtmini_overview", nil)
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = callTool(t, cs2, "voxtmini_overview", nil)
	}()
	time.Sleep(50 * time.Millisecond)
	close(gateCh)
	wg.Wait()

	if errs[0] || errs[1] {
		t.Fatalf("expected both coalesced calls to succeed, got errs=%v", errs)
	}
	if got := r.prompter.count(); got != 1 {
		t.Errorf("expected 1 connect prompt for 2 concurrent bridges, got %d", got)
	}
}

func TestGateConfirmLeases(t *testing.T) {
	p := Policy{
		Enabled:          true,
		AllowManage:      true,
		AllowLocalModels: true,
		Clients: map[string]ClientGrant{
			"/Applications/ChatGPT.app|TEAM123456": {Allowed: true},
		},
	}
	r := newRig(t, p, false)
	cs := r.connect("codex")

	// 1. Deny -> user_declined.
	r.prompter.fn = func(context.Context, Question) Answer { return Deny }
	out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "AI"})
	if !isErr || out["error"] != "user_declined" {
		t.Fatalf("expected user_declined, got %v (isErr=%v)", out, isErr)
	}

	// 2. AllowOnce -> prompts again on next call.
	r.prompter.fn = func(context.Context, Question) Answer { return AllowOnce }
	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "AI"}); isErr {
		t.Fatalf("AllowOnce failed: %v", out)
	}
	before := r.prompter.count()
	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "ML"}); isErr {
		t.Fatalf("second AllowOnce failed: %v", out)
	}
	if r.prompter.count() != before+1 {
		t.Errorf("AllowOnce should prompt on next call")
	}

	// 3. AllowMinute -> skips prompt within 1 minute, prompts after expiry.
	r.prompter.fn = func(context.Context, Question) Answer { return AllowMinute }
	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "Dev"}); isErr {
		t.Fatalf("AllowMinute failed: %v", out)
	}
	afterMin := r.prompter.count()
	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "Ops"}); isErr {
		t.Fatalf("within minute failed: %v", out)
	}
	if r.prompter.count() != afterMin {
		t.Errorf("within 1 minute should not prompt again")
	}
	r.advance(2 * time.Minute)
	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "QA"}); isErr {
		t.Fatalf("after minute failed: %v", out)
	}
	if r.prompter.count() != afterMin+1 {
		t.Errorf("after 1 minute should prompt again")
	}

	// 4. AllowAlways -> skips prompt until AskAgain; delete_history still prompts (EveryTime).
	r.advance(2 * time.Minute)
	r.prompter.fn = func(context.Context, Question) Answer { return AllowAlways }
	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "Sys"}); isErr {
		t.Fatalf("AllowAlways failed: %v", out)
	}
	afterAlways := r.prompter.count()
	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "Net"}); isErr {
		t.Fatalf("second call under AllowAlways failed: %v", out)
	}
	if r.prompter.count() != afterAlways {
		t.Errorf("AllowAlways should skip prompt")
	}
	if out, isErr := callTool(t, cs, "delete_history", map[string]any{"id": "h1"}); isErr {
		t.Fatalf("delete_history failed: %v", out)
	}
	if r.prompter.count() != afterAlways+1 {
		t.Errorf("delete_history must always prompt (EveryTime)")
	}

	r.srv.AskAgain("/Applications/ChatGPT.app|TEAM123456")
	if out, isErr := callTool(t, cs, "upsert_category", map[string]any{"name": "Reset"}); isErr {
		t.Fatalf("after AskAgain failed: %v", out)
	}
	if r.prompter.count() != afterAlways+2 {
		t.Errorf("AskAgain should clear AllowAlways lease")
	}
}

func TestRateLimiter(t *testing.T) {
	r := newRig(t, Policy{Enabled: true}, true)
	cs := r.connect("burst-client")
	var gotLimited bool
	for i := range 80 {
		out, isErr := callTool(t, cs, "list_categories", nil)
		if isErr && out["error"] == "rate_limited" {
			gotLimited = true
			if ra, _ := out["retryAfter"].(float64); ra < 1 {
				t.Errorf("call %d: retryAfter = %v, want >= 1", i, out["retryAfter"])
			}
			break
		}
	}
	if !gotLimited {
		t.Fatal("expected rate_limited after exceeding burst")
	}
}

func TestRescope(t *testing.T) {
	r := newRig(t, Policy{Enabled: true}, true)
	cs := r.connect("rescope-client")

	list1, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list1.Tools) != 8 {
		t.Fatalf("read-only tools = %d, want 8", len(list1.Tools))
	}

	r.host.mu.Lock()
	r.host.policy.AllowManage = true
	r.host.policy.AllowLocalModels = true
	r.host.policy.AllowTrigger = true
	r.host.mu.Unlock()
	r.srv.Rescope()

	list2, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2.Tools) != 20 {
		t.Fatalf("all scopes enabled tools = %d, want 20", len(list2.Tools))
	}
}

func TestToolsListSize(t *testing.T) {
	r := newRig(t, Policy{
		Enabled:           true,
		AllowManage:       true,
		AllowLocalModels:  true,
		AllowRemoteModels: true,
		AllowTrigger:      true,
	}, true)
	cs := r.connect("size-check")
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	const maxBytes = 15 * 1024
	if len(b) > maxBytes {
		t.Errorf("tools/list is %d bytes, want <= %d bytes", len(b), maxBytes)
	}
}

func TestInstructionsSize(t *testing.T) {
	if len(instructions) > 1536 {
		t.Errorf("instructions is %d bytes, want <= 1536", len(instructions))
	}
	first512 := instructions
	if len(first512) > 512 {
		first512 = first512[:512]
	}
	for _, must := range []string{"VoxtMini", "voxtmini_overview", "remote models"} {
		if !strings.Contains(first512, must) {
			t.Errorf("first 512 chars of instructions missing %q", must)
		}
	}
}

func TestSelfTest(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "run", "mcp.sock")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	srv := New(Options{
		Host:    newFakeHost(Policy{Enabled: true}),
		Bridges: []string{exe},
		Version: "0.2.0",
	})
	if err := srv.Listen(sock); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	res, err := SelfTest(context.Background(), sock, "0.2.0")
	if err != nil {
		t.Fatalf("SelfTest failed: %v", err)
	}
	if res.Tools != 8 || res.Protocol == "" {
		t.Errorf("unexpected SelfTestResult: %+v", res)
	}
}

type memDocs struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (d *memDocs) Get(_ context.Context, key string) ([]byte, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b, ok := d.m[key]
	return b, ok, nil
}

func (d *memDocs) Put(_ context.Context, key string, doc []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.m == nil {
		d.m = map[string][]byte{}
	}
	d.m[key] = append([]byte(nil), doc...)
	return nil
}

func TestAuditNoSecrets(t *testing.T) {
	docs := &memDocs{}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	a := OpenDocAudit(docs, func() time.Time { return now })
	a.Record(context.Background(), CallRecord{
		At:         now,
		ClientKey:  "k1",
		ClientName: "Codex",
		Tool:       "translate_text",
		Scope:      "models",
		Count:      1,
		Result:     "ok",
		DurationMS: 12,
		Bytes:      128,
	})
	a.Flush()

	b, ok, _ := docs.Get(context.Background(), auditDocKey)
	if !ok || len(b) == 0 {
		t.Fatal("audit not persisted")
	}
	reloaded := OpenDocAudit(docs, func() time.Time { return now })
	today, total, last := reloaded.Stats("k1")
	if today != 1 || total != 1 || last.Tool != "translate_text" {
		t.Errorf("unexpected stats: today=%d total=%d last=%+v", today, total, last)
	}
	reloaded.Clear()
	if len(reloaded.Entries()) != 0 {
		t.Errorf("expected empty after Clear")
	}
}

func TestServerLifecycleAndForget(t *testing.T) {
	r := newRig(t, Policy{Enabled: true}, true)
	if !r.srv.Serving() {
		t.Fatal("expected Serving() == true")
	}
	cs := r.connect("live-client")
	if out, isErr := callTool(t, cs, "voxtmini_overview", nil); isErr || out["appVersion"] != "0.2.0" {
		t.Fatalf("overview: %v", out)
	}
	live := r.srv.Live()
	if len(live) != 1 || live[0].Key != "/Applications/ChatGPT.app|TEAM123456" {
		t.Fatalf("Live() = %+v", live)
	}
	r.srv.Forget(live[0].Key)
	if err := r.srv.Close(); err != nil {
		t.Fatal(err)
	}
	if r.srv.Serving() {
		t.Fatal("expected Serving() == false after Close")
	}
}
