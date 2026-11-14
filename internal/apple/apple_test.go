package apple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/helper"
	"github.com/hehehai/VoxtMini/internal/helper/helpertest"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
)

// The test binary is also the fake helper (TestFakeHelperProcess).
func TestMain(m *testing.M) {
	if helpertest.Enabled() {
		helpertest.Main()
	}
	os.Exit(m.Run())
}

func TestLocale(t *testing.T) {
	for in, want := range map[string]string{
		"en": "en-US", "en-GB": "en-US", "zh-Hans": "zh-CN", "zh": "zh-CN", "zh_CN": "zh-CN", "ja": "ja-JP", "ja-JP": "ja-JP",
		"zh-Hant": "", "zh-TW": "", "fr": "", "": "",
	} {
		if got := Locale(in); got != want {
			t.Errorf("Locale(%q) = %q, want %q", in, got, want)
		}
	}
}

// downloaded is local models with one speech model and one language
// model installed (or none).
type downloaded struct{ asr, llm bool }

func (d downloaded) Installed(k local.Kind) []local.Model {
	if k == local.ASR && d.asr {
		return []local.Model{{Kind: k, ID: "asr.fast", Name: "Fast"}}
	}
	if k == local.LLM && d.llm {
		return []local.Model{{Kind: k, ID: "llm.fast", Name: "Fast"}}
	}
	return nil
}

func (d downloaded) ASR(_ context.Context, id string) (asr.Provider, local.Model, error) {
	if m, ok := local.Choose(d.Installed(local.ASR), id); ok {
		return nil, m, nil
	}
	return nil, local.Model{}, local.NotInstalledError(local.ASR, id)
}

func (d downloaded) LLM(_ context.Context, id string) (local.Chat, local.Model, error) {
	if m, ok := local.Choose(d.Installed(local.LLM), id); ok {
		return nil, m, nil
	}
	return nil, local.Model{}, local.NotInstalledError(local.LLM, id)
}

func ids(list []local.Model) []string {
	var out []string
	for _, m := range list {
		out = append(out, m.ID)
	}
	return out
}

func TestServicesList(t *testing.T) {
	sim := helpertest.NewAppleSim()
	s := New(Config{Local: downloaded{asr: true}, Helper: sim})
	if got := ids(s.Installed(local.ASR)); !slices.Equal(got, []string{"asr.fast"}) {
		t.Fatalf("before Refresh: %v", got)
	}
	if _, err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := ids(s.Installed(local.ASR)); !slices.Equal(got, []string{"asr.fast", SpeechID}) {
		t.Fatalf("speech: %v", got)
	}
	if got := ids(s.Installed(local.LLM)); !slices.Equal(got, []string{IntelligenceID}) {
		t.Fatalf("language: %v", got)
	}
	// Automatic: the downloaded model first, Apple's without one.
	if _, m, _ := s.ASR(context.Background(), ""); m.ID != "asr.fast" {
		t.Errorf("automatic speech %q", m.ID)
	}
	if c, m, err := s.LLM(context.Background(), ""); err != nil || m.ID != IntelligenceID || c == nil {
		t.Errorf("automatic language %q %v", m.ID, err)
	}
	if p, m, err := s.ASR(context.Background(), SpeechID); err != nil || m.ID != SpeechID || !p.Caps().Stream || !p.Caps().Hints || p.Caps().File {
		t.Errorf("chosen Apple Speech: %v %v %+v", m, err, p)
	}

	// Apple Intelligence turned off, no speech language: neither listed.
	sim.Set(func(a *helpertest.AppleSim) {
		a.Intelligence = proto.AppleReasonNotEnabled
		a.Locales = map[string]string{"en-US": proto.LocaleAvailable}
	})
	st, _ := s.Refresh(context.Background())
	if len(s.apples(local.ASR))+len(s.apples(local.LLM)) != 0 || st.IntelligenceReason() != proto.AppleReasonNotEnabled {
		t.Fatalf("listed %v %v; reason %q", s.apples(local.ASR), s.apples(local.LLM), st.IntelligenceReason())
	}
	if _, _, err := s.LLM(context.Background(), IntelligenceID); !errors.Is(err, local.ErrNotInstalled) || !errors.Is(err, ErrUnavailable) {
		t.Errorf("chosen while unavailable: %v", err)
	}

	// An older helper: none, and nothing is kept.
	old := New(Config{Local: downloaded{}, Helper: callerFunc(func() error { return proto.Errorf(proto.CodeMethodNotFound, "Unknown method") })})
	old.SetStatus(Status{Known: true, AppleStatus: sim.Status(proto.AppleStatusParams{Locales: Locales})})
	if st, err := old.Refresh(context.Background()); err != nil || st.Known || len(old.Installed(local.LLM)) != 0 {
		t.Errorf("older helper: %+v %v", st, err)
	}
	// A helper that is down keeps what was known.
	down := New(Config{Helper: callerFunc(func() error { return fmt.Errorf("%w: exited", core.ErrHelper) })})
	down.SetStatus(Status{Known: true})
	if st, err := down.Refresh(context.Background()); err == nil || !st.Known {
		t.Errorf("helper down: %+v %v", st, err)
	}
}

// callerFunc fails every call with fn's error.
type callerFunc func() error

func (f callerFunc) Call(context.Context, string, string, any, func(proto.Message)) (json.RawMessage, error) {
	return nil, f()
}

func TestInstall(t *testing.T) {
	sim := helpertest.NewAppleSim()
	s := New(Config{Helper: sim})
	var mu sync.Mutex
	var got []float64
	if err := s.Install(context.Background(), []string{"ja-JP"}, func(f float64) { mu.Lock(); got = append(got, f); mu.Unlock() }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []float64{0.5, 1}) || s.Status().LocaleStatus("ja-JP") != proto.LocaleInstalled {
		t.Errorf("progress %v, ja-JP %s", got, s.Status().LocaleStatus("ja-JP"))
	}
}

// openSpeech is Apple Speech over sim.
func openSpeech(t *testing.T, c Caller, lang string, hints []string) (asr.Stream, error) {
	t.Helper()
	return (&Speech{h: c}).Open(context.Background(), asr.StreamRequest{Lang: lang, Hints: hints})
}

func TestSpeechStream(t *testing.T) {
	sim := helpertest.NewAppleSim()
	var hints []string
	for i := range 150 {
		hints = append(hints, fmt.Sprintf("term%d", i))
	}
	st, err := openSpeech(t, sim, "zh-Hans", hints)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	pcm := make([]byte, 3*asr.SampleRate) // 1.5 s: two pieces
	if err := st.Send(pcm); err != nil {
		t.Fatal(err)
	}
	if err := st.Send(make([]byte, 3200)); err != nil {
		t.Fatal(err)
	}
	if err := st.Finish(); err != nil {
		t.Fatal(err)
	}
	var partials []string
	var final asr.Event
	for ev := range st.Events() {
		switch ev.Kind {
		case asr.EventPartial:
			partials = append(partials, ev.Text)
		default:
			final = ev
		}
	}
	if final.Kind != asr.EventFinal || final.Text != "Hello from Apple Speech." || final.Result.Lang != "zh" || final.Result.Duration != 1600*time.Millisecond {
		t.Fatalf("final %+v", final)
	}
	if !slices.Equal(partials, []string{"Hello", "Hello from", "Hello from Apple"}) || st.(asr.Salvager).Salvage() != "Hello from Apple" {
		t.Errorf("partials %v", partials)
	}
	p := sim.Speeches[0]
	if p.Locale != "zh-CN" || len(p.Hints) != MaxHints || p.Hints[0] != "term0" {
		t.Errorf("params %s, %d hints", p.Locale, len(p.Hints))
	}
}

func TestSpeechRecognize(t *testing.T) {
	sim := helpertest.NewAppleSim()
	res, err := asr.Recognize(context.Background(), &Speech{h: sim}, make([]byte, 6400), "en", nil, "", asr.Feed{})
	if err != nil || res.Text != "Hello from Apple Speech." {
		t.Fatalf("%+v %v", res, err)
	}
}

// TestSpeechTranscribe: a whole recording (a history entry's retry).
func TestSpeechTranscribe(t *testing.T) {
	s := &Speech{h: helpertest.NewAppleSim()}
	for _, req := range []asr.FileRequest{
		{Audio: make([]byte, 6400), Format: "pcm", Lang: "ja-JP"},
		{Audio: asr.WAV(make([]byte, 6400), asr.SampleRate), Format: "wav", Lang: "en"},
	} {
		if req.Lang == "ja-JP" {
			s.h.(*helpertest.AppleSim).Set(func(a *helpertest.AppleSim) { a.Locales["ja-JP"] = proto.LocaleInstalled })
		}
		if res, err := s.Transcribe(context.Background(), req); err != nil || res.Text != "Hello from Apple Speech." {
			t.Errorf("%s: %+v %v", req.Format, res, err)
		}
	}
	if _, err := s.Transcribe(context.Background(), asr.FileRequest{Audio: asr.WAV(make([]byte, 6400), 48000), Format: "wav", Lang: "en"}); asr.KindOf(err) != asr.KindAudio {
		t.Errorf("48 kHz: %v", err)
	}
}

func TestSpeechFailures(t *testing.T) {
	sim := helpertest.NewAppleSim()
	if _, err := openSpeech(t, sim, "fr", nil); !errors.Is(err, ErrLanguage) || asr.KindOf(err) != asr.KindConfig {
		t.Errorf("French: %v", err)
	}
	if _, err := openSpeech(t, sim, "ja", nil); !errors.Is(err, ErrLanguage) { // available, not installed
		t.Errorf("Japanese not downloaded: %v", err)
	}
	sim.Set(func(a *helpertest.AppleSim) { a.SpeechPerm = proto.PermDenied })
	if _, err := openSpeech(t, sim, "en", nil); !errors.Is(err, ErrPermission) {
		t.Errorf("denied: %v", err)
	}
	sim.Set(func(a *helpertest.AppleSim) { a.SpeechPerm = proto.PermNotDetermined })
	if st, err := openSpeech(t, sim, "en", nil); err != nil { // the first use asks, and the user allows
		t.Errorf("not asked yet: %v", err)
	} else {
		st.Close()
	}
	sim.Set(func(a *helpertest.AppleSim) { a.Engine = "" })
	if _, err := openSpeech(t, sim, "en", nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("no engine: %v", err)
	}
	// Close abandons the job: no final event.
	sim = helpertest.NewAppleSim()
	st, err := openSpeech(t, sim, "en", nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Send(make([]byte, 3200))
	st.Close()
	for ev := range st.Events() {
		if ev.Kind == asr.EventFinal {
			t.Errorf("final after Close: %+v", ev)
		}
	}
	if err := st.Send(make([]byte, 3200)); asr.KindOf(err) != asr.KindCancelled {
		t.Errorf("send after Close: %v", err)
	}
}

func TestChat(t *testing.T) {
	sim := helpertest.NewAppleSim()
	c := &Chat{h: sim}
	var deltas []string
	res, err := c.Chat(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.System, Content: "Clean up."}, {Role: llm.User, Content: "um hello 你好"}},
		Skill:    llm.SkillRewrite, OnDelta: func(s string) { deltas = append(deltas, s) },
	})
	if err != nil || res.Text != "Apple: um hello 你好" || strings.Join(deltas, "") != res.Text || len(deltas) != 2 || res.Provider != Provider {
		t.Fatalf("%+v %v %q", res, err, deltas)
	}
	p := sim.Chats[0]
	if len(p.Messages) != 2 || p.Messages[0].Role != "system" || *p.Temperature != 0.5 || p.MaxTokens != MaxAnswer {
		t.Errorf("params %+v", p)
	}
	c.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "x"}}, Temperature: llm.F64(0), MaxTokens: 100})
	if p := sim.Chats[1]; *p.Temperature != 0 || p.MaxTokens != 100 {
		t.Errorf("params %+v", p)
	}
	// The user's settings win over the request's (Services.SetParams).
	user := Params{Temperature: llm.F64(0.9), MaxTokens: 300}
	tuned := &Chat{h: sim, params: func() Params { return user }}
	tuned.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "x"}}, Temperature: llm.F64(0), MaxTokens: 100})
	if p := sim.Chats[2]; *p.Temperature != 0.9 || p.MaxTokens != 300 {
		t.Errorf("user's params %+v", p)
	}
	sim.Chats = sim.Chats[:2]
	// Too long for the window: refused before anything is sent.
	long := strings.Repeat("字", Window)
	if _, err := c.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: long}}}); codeOf(err) != llm.CodeContextLength || len(sim.Chats) != 2 {
		t.Errorf("long: %v", err)
	}
	sim.Set(func(a *helpertest.AppleSim) { a.ChatErr = proto.Errorf(proto.CodeRefused, "no") })
	if _, err := c.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "x"}}}); !errors.Is(err, ErrRefused) || codeOf(err) != llm.CodeFiltered {
		t.Errorf("refused: %v", err)
	}
	sim.Set(func(a *helpertest.AppleSim) { a.ChatErr = proto.Errorf(proto.CodeContextLength, "too long") })
	if _, err := c.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "x"}}}); codeOf(err) != llm.CodeContextLength {
		t.Errorf("context: %v", err)
	}
	sim.Set(func(a *helpertest.AppleSim) { a.ChatErr, a.Intelligence = nil, proto.AppleReasonModelNotReady })
	if _, err := c.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "x"}}}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("unavailable: %v", err)
	}
}

func TestBudget(t *testing.T) {
	for _, c := range []struct{ prompt, want, got int }{{600, 0, MaxAnswer}, {600, 300, 300}, {3000, 0, 1096}, {4090, 0, 0}} {
		got, ok := Budget(c.prompt, c.want)
		if got != c.got || ok != (c.got > 0) {
			t.Errorf("Budget(%d, %d) = %d %v", c.prompt, c.want, got, ok)
		}
	}
}

// TestFakeHelperProcess: the protocol through the real supervisor and the
// fake helper process (pipes, job.cancel, notifications).
func TestFakeHelperProcess(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sup := helper.New(helper.Config{Bin: self, AppVersion: "test", Bus: core.NewBus(), Env: helpertest.Env("apple"),
		Log: func(core.LogEntry) {}, Logf: t.Logf, HelloTimeout: 5 * time.Second, CancelGrace: time.Second})
	defer sup.Stop()
	sup.Start(context.Background())
	for deadline := time.Now().Add(5 * time.Second); sup.Status().State != helper.Ready; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the fake helper did not start")
		}
	}
	s := New(Config{Local: downloaded{}, Helper: sup})
	if _, err := s.Refresh(context.Background()); err != nil || !s.Status().SpeechReady() || !s.Status().IntelligenceReady() {
		t.Fatalf("status %+v %v", s.Status(), err)
	}
	p, _, err := s.ASR(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var partials []string
	res, err := asr.Recognize(context.Background(), p, make([]byte, 6400), "en", []string{"VoxtMini"}, "", asr.Feed{OnPartial: func(s string) { partials = append(partials, s) }})
	if err != nil || res.Text != "Hello from Apple Speech." || len(partials) == 0 {
		t.Fatalf("%+v %v %v", res, err, partials)
	}
	// A cancelled recognition ends its job.
	st, err := p.(asr.Streamer).Open(context.Background(), asr.StreamRequest{Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	st.Send(make([]byte, 3200))
	st.Close()
	chat, _, err := s.LLM(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var deltas []string
	r, err := chat.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "hi"}}, OnDelta: func(s string) { deltas = append(deltas, s) }})
	if err != nil || r.Text != "Apple: hi" || strings.Join(deltas, "") != "Apple: hi" {
		t.Fatalf("%+v %v %v", r, err, deltas)
	}
}

func codeOf(err error) string {
	var le *llm.Error
	if errors.As(err, &le) {
		return le.Code
	}
	return ""
}
