package mcp

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeInspector struct {
	mu     sync.Mutex
	peer   Proc
	parent Proc
	team   string
}

func chatGPTInspector() *fakeInspector {
	return &fakeInspector{
		peer:   Proc{PID: 2001, PPID: 1001, UID: os.Getuid(), Exe: "/Applications/VoxtMini.app/Contents/Helpers/voxtmini-ctl"},
		parent: Proc{PID: 1001, PPID: 1, UID: os.Getuid(), Exe: "/Applications/ChatGPT.app/Contents/MacOS/ChatGPT"},
		team:   "TEAM123456",
	}
}

func (f *fakeInspector) Peer(net.Conn) (Proc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peer, nil
}

func (f *fakeInspector) Proc(pid int) (Proc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if pid == f.parent.PID {
		return f.parent, nil
	}
	return Proc{}, ErrNotFound
}

func (f *fakeInspector) Team(string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.team
}

type fakePrompter struct {
	mu      sync.Mutex
	fn      func(ctx context.Context, q Question) Answer
	asked   []Question
	onAsked chan Question
}

func (p *fakePrompter) Ask(ctx context.Context, q Question) Answer {
	p.mu.Lock()
	p.asked = append(p.asked, q)
	fn := p.fn
	ch := p.onAsked
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- q:
		default:
		}
	}
	if fn != nil {
		return fn(ctx, q)
	}
	return AllowAlways
}

func (p *fakePrompter) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.asked)
}

type fakeHost struct {
	mu      sync.Mutex
	policy  Policy
	ready   error
	changed int
	busy    bool

	history    map[string]HistoryDetail
	terms      map[string]Term
	categories map[string]Category
	groups     map[string]AppGroup
	models     []ModelInfo
	mics       []Microphone
	status     SessionStatus
}

func newFakeHost(p Policy) *fakeHost {
	return &fakeHost{
		policy: p,
		history: map[string]HistoryDetail{
			"h1": {
				ID:        "h1",
				CreatedAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
				Kind:      "transcribe",
				Status:    "done",
				AppName:   "Notes",
				RawText:   "hello vox mini",
				Result:    "Hello VoxtMini",
				ASRModel:  "qwen3-asr-0.6b",
				Corrections: []Correction{
					{From: "vox mini", To: "VoxtMini"},
				},
				Audio: &AudioMeta{HasAudio: true, DurationMS: 2100, Path: "/tmp/h1.wav"},
			},
		},
		terms: map[string]Term{
			"t1": {ID: "t1", Spelling: "VoxtMini", Hints: []string{"vox mini"}, Category: "Product", Enabled: true, Uses: 5},
		},
		categories: map[string]Category{
			"c1": {ID: "c1", Name: "Product"},
		},
		groups: map[string]AppGroup{
			"g1": {ID: "g1", Name: "Coding", Enabled: true, Apps: []string{"com.microsoft.VSCode"}, Prompt: "Keep code identifiers exact.", AfterKey: "Return", Features: []string{"transcribe", "rewrite"}},
		},
		models: []ModelInfo{
			{ID: "local-asr", Name: "Qwen3-ASR 0.6B", Kind: "speech", Source: "local", Installed: true, Ready: true, Features: []string{"transcribe"}, Default: true},
			{ID: "local-llm", Name: "Qwen3.5 4B", Kind: "language", Source: "local", Installed: true, Ready: true, Features: []string{"translate", "rewrite"}, Default: true},
			{ID: "remote-asr", Name: "OpenAI Whisper", Kind: "speech", Source: "remote", Installed: true, Ready: true, Features: []string{"transcribe"}},
			{ID: "remote-llm", Name: "GPT-5", Kind: "language", Source: "remote", Installed: true, Ready: true, Features: []string{"translate", "rewrite"}},
		},
		mics: []Microphone{
			{UID: "BuiltInMic", Name: "MacBook Air Microphone", Transport: "builtin", IsDefault: true, Priority: 0},
			{UID: "USB-Mic-1", Name: "Shure MV7", Transport: "usb", IsDefault: false, Priority: 1},
		},
		status: SessionStatus{State: "idle"},
	}
}

func (h *fakeHost) Policy() Policy {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.policy
}

func (h *fakeHost) Ready() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ready
}

func (h *fakeHost) Changed() {
	h.mu.Lock()
	h.changed++
	h.mu.Unlock()
}

func (h *fakeHost) Overview(context.Context) (Overview, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ms := make([]ModelSummary, 0, len(h.models))
	for _, m := range h.models {
		ms = append(ms, ModelSummary{ID: m.ID, Name: m.Name, Kind: m.Kind, Source: m.Source, Ready: m.Ready})
	}
	return Overview{
		AppVersion: "0.2.0",
		Session:    h.status,
		Counts: OverviewCounts{
			History:    len(h.history),
			Terms:      len(h.terms),
			Categories: len(h.categories),
			AppGroups:  len(h.groups),
		},
		Models:      ms,
		Microphones: h.mics,
	}, nil
}

func (h *fakeHost) ListHistory(_ context.Context, req HistoryQuery) (HistoryPage, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var items []HistoryItem
	for _, e := range h.history {
		if req.Kind != "" && e.Kind != req.Kind {
			continue
		}
		items = append(items, HistoryItem{
			ID:        e.ID,
			CreatedAt: e.CreatedAt,
			Kind:      e.Kind,
			Status:    e.Status,
			AppName:   e.AppName,
			Preview:   e.Result,
			HasAudio:  e.Audio != nil && e.Audio.HasAudio,
		})
	}
	return HistoryPage{Entries: items}, nil
}

func (h *fakeHost) GetHistory(_ context.Context, id string) (HistoryDetail, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := h.history[id]
	if !ok {
		return HistoryDetail{}, ErrNotFound
	}
	return e, nil
}

func (h *fakeHost) ListTerms(_ context.Context, _, _ string) ([]Term, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Term, 0, len(h.terms))
	for _, tm := range h.terms {
		out = append(out, tm)
	}
	return out, nil
}

func (h *fakeHost) ListCategories(context.Context) ([]Category, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Category, 0, len(h.categories))
	for _, c := range h.categories {
		out = append(out, c)
	}
	return out, nil
}

func (h *fakeHost) ListAppGroups(context.Context) ([]AppGroup, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]AppGroup, 0, len(h.groups))
	for _, g := range h.groups {
		out = append(out, g)
	}
	return out, nil
}

func (h *fakeHost) ListModels(_ context.Context, kind string) ([]ModelInfo, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []ModelInfo
	for _, m := range h.models {
		if kind == "" || kind == "all" || m.Kind == kind {
			out = append(out, m)
		}
	}
	return out, nil
}

func (h *fakeHost) ListMicrophones(context.Context) ([]Microphone, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mics, nil
}

func (h *fakeHost) UpsertTerms(_ context.Context, in []TermInput) ([]Term, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Term, 0, len(in))
	for _, item := range in {
		id := item.ID
		if id == "" {
			id = "t-new-" + item.Spelling
		}
		en := true
		if item.Enabled != nil {
			en = *item.Enabled
		}
		tm := Term{ID: id, Spelling: item.Spelling, Hints: item.Hints, Category: item.Category, Group: item.Group, Enabled: en}
		h.terms[id] = tm
		out = append(out, tm)
	}
	return out, nil
}

func (h *fakeHost) DeleteTerm(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.terms[id]; !ok {
		return ErrNotFound
	}
	delete(h.terms, id)
	return nil
}

func (h *fakeHost) UpsertCategory(_ context.Context, in CategoryInput) (Category, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := in.ID
	if id == "" {
		id = "c-new-" + in.Name
	}
	cat := Category{ID: id, Name: in.Name}
	h.categories[id] = cat
	return cat, nil
}

func (h *fakeHost) UpsertAppGroup(_ context.Context, in AppGroupInput) (AppGroup, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := in.ID
	if id == "" {
		id = "g-new-" + in.Name
	}
	en := true
	if in.Enabled != nil {
		en = *in.Enabled
	}
	g := AppGroup{ID: id, Name: in.Name, Enabled: en, Apps: in.Apps, Domains: in.Domains, Prompt: in.Prompt, AfterKey: in.AfterKey, Features: in.Features}
	h.groups[id] = g
	return g, nil
}

func (h *fakeHost) DeleteAppGroup(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.groups[id]; !ok {
		return ErrNotFound
	}
	delete(h.groups, id)
	return nil
}

func (h *fakeHost) DeleteHistory(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.history[id]; !ok {
		return ErrNotFound
	}
	delete(h.history, id)
	return nil
}

func (h *fakeHost) TranscribeAudio(_ context.Context, req TranscribeRequest) (TranscribeResult, error) {
	model := req.ModelID
	source := "local"
	if model == "" {
		model = "local-asr"
	}
	if model == "remote-asr" {
		if !req.AllowRemote {
			return TranscribeResult{}, ErrRemoteOff
		}
		source = "remote"
	}
	return TranscribeResult{Text: "transcribed audio text", Model: model, Source: source, LatencyMS: 42}, nil
}

func (h *fakeHost) TranslateText(_ context.Context, req TranslateRequest) (TranslateResult, error) {
	model := req.ModelID
	source := "local"
	if model == "" {
		model = "local-llm"
	}
	if model == "remote-llm" {
		if !req.AllowRemote {
			return TranslateResult{}, ErrRemoteOff
		}
		source = "remote"
	}
	return TranslateResult{Text: "translated:" + req.Text, TargetLanguage: req.TargetLanguage, Model: model, Source: source, LatencyMS: 18}, nil
}

func (h *fakeHost) RewriteText(_ context.Context, req RewriteRequest) (RewriteResult, error) {
	model := req.ModelID
	source := "local"
	if model == "" {
		model = "local-llm"
	}
	return RewriteResult{Text: "rewritten:" + req.Instruction, Model: model, Source: source, LatencyMS: 20}, nil
}

func (h *fakeHost) ChatModel(_ context.Context, req ChatRequest) (ChatResult, error) {
	model := req.ModelID
	source := "local"
	if model == "" {
		model = "local-llm"
	}
	return ChatResult{Text: "reply:" + req.Prompt, Model: model, Source: source, LatencyMS: 25}, nil
}

func (h *fakeHost) TriggerSession(_ context.Context, req TriggerRequest) (TriggerResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.busy && req.Action == "start" {
		return TriggerResult{}, ErrBusy
	}
	h.status = SessionStatus{State: "recording", Feature: req.Feature, Microphone: req.Device}
	return TriggerResult{State: "recording", Feature: req.Feature, Microphone: req.Device}, nil
}

func (h *fakeHost) SessionStatus(context.Context) (SessionStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status, nil
}

type rig struct {
	t        *testing.T
	srv      *Server
	host     *fakeHost
	prompter *fakePrompter
	audit    *DocAudit
	ins      *fakeInspector
	path     string
	audioDir string
	mu       sync.Mutex
	now      time.Time
}

func newRig(t *testing.T, p Policy, preApprove bool) *rig {
	t.Helper()
	ins := chatGPTInspector()
	if preApprove {
		if p.Clients == nil {
			p.Clients = map[string]ClientGrant{}
		}
		p.Clients["/Applications/ChatGPT.app|TEAM123456"] = ClientGrant{
			Allowed:       true,
			AlwaysManage:  true,
			AlwaysModels:  true,
			AlwaysTrigger: true,
		}
	}
	dir := t.TempDir()
	audioDir := filepath.Join(dir, "audio")
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	r := &rig{
		t:        t,
		host:     newFakeHost(p),
		prompter: &fakePrompter{},
		ins:      ins,
		path:     filepath.Join(dir, "run", "mcp.sock"),
		audioDir: audioDir,
		now:      now,
	}
	clock := func() time.Time {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.now
	}
	r.audit = OpenDocAudit(nil, clock)
	r.srv = New(Options{
		Host:              r.host,
		Prompter:          r.prompter,
		Inspector:         ins,
		Audit:             r.audit,
		Version:           "0.2.0",
		Bridges:           []string{"/Applications/VoxtMini.app/Contents/Helpers/voxtmini-ctl"},
		AllowedAudioRoots: []string{audioDir},
		Now:               clock,
		AskTimeout:        2 * time.Second,
	})
	if err := r.srv.Listen(r.path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.srv.Close() })
	return r
}

func (r *rig) advance(d time.Duration) {
	r.mu.Lock()
	r.now = r.now.Add(d)
	r.mu.Unlock()
}

func (r *rig) connect(clientName string) *sdk.ClientSession {
	r.t.Helper()
	conn, err := net.Dial("unix", r.path)
	if err != nil {
		r.t.Fatal(err)
	}
	if _, err := conn.Write([]byte(`{"app":"voxtmini","instance":"test","v":1}` + "\n")); err != nil {
		conn.Close()
		r.t.Fatal(err)
	}
	cl := sdk.NewClient(&sdk.Implementation{Name: clientName, Version: "1.0"}, nil)
	sess, err := cl.Connect(context.Background(), &sdk.IOTransport{Reader: conn, Writer: conn}, nil)
	if err != nil {
		conn.Close()
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func callTool(t *testing.T, sess *sdk.ClientSession, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	res, err := sess.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("CallTool(%s): empty content", name)
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("CallTool(%s): non-text content", name)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("CallTool(%s): invalid JSON %q: %v", name, tc.Text, err)
	}
	return out, res.IsError
}
