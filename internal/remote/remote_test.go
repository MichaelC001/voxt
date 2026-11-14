package remote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// memRing is a Keyring in memory.
type memRing struct {
	mu   sync.Mutex
	keys map[string]string
	err  error
}

func (r *memRing) Get(acc string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.keys[acc], r.err
}

func (r *memRing) Set(acc, _, v string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v == "" {
		delete(r.keys, acc)
	} else {
		r.keys[acc] = v
	}
	return nil
}

// oneLocal has one local model of each kind.
type oneLocal struct{ opened []string }

func (o *oneLocal) Installed(k local.Kind) []local.Model {
	return []local.Model{{Kind: k, ID: string(k) + ".fast", Name: "Fast"}}
}

func (o *oneLocal) ASR(_ context.Context, id string) (asr.Provider, local.Model, error) {
	o.opened = append(o.opened, "asr:"+id)
	return nil, o.Installed(local.ASR)[0], nil
}

func (o *oneLocal) LLM(_ context.Context, id string) (local.Chat, local.Model, error) {
	o.opened = append(o.opened, "llm:"+id)
	return nil, o.Installed(local.LLM)[0], nil
}

// TestServices: the ready remote services follow the local models,
// open with their key and parameters over loopback servers, and a remote
// id that is not ready opens the local model.
func TestServices(t *testing.T) {
	var gotKey, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/v1/audio/transcriptions":
			_ = r.ParseMultipartForm(1 << 20)
			gotModel = r.FormValue("model")
			io.WriteString(w, `{"text":"hello"}`)
		default:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"model":"m","choices":[{"message":{"content":"answer"},"finish_reason":"stop"}]}`)
		}
	}))
	defer srv.Close()
	s := settings.Defaults()
	s.Providers.ASR = []settings.Provider{{ID: "openai", Enabled: true, Endpoint: srv.URL + "/v1", Model: "whisper-1"}}
	s.Providers.LLM = []settings.Provider{{ID: "deepseek", Enabled: true, Endpoint: srv.URL + "/v1"}, {ID: "openai", Enabled: false}}
	ring := &memRing{keys: map[string]string{"asr.openai": "sk-asr-123456", "llm.deepseek": "sk-llm-123456", "llm.openai": "x"}}
	keys := NewKeys(ring)
	lm := &oneLocal{}
	svc := New(Config{Local: lm, Settings: func() settings.Settings { return s }, Keys: keys})

	// Before the keys are read, nothing remote is ready.
	if l := svc.Installed(local.LLM); len(l) != 1 {
		t.Fatalf("before keys: %v", l)
	}
	if err := keys.Load([]string{"asr.openai", "llm.deepseek", "llm.openai"}); err != nil {
		t.Fatal(err)
	}
	l := svc.Installed(local.LLM)
	if len(l) != 2 || l[1].ID != "remote:deepseek" || !l[1].Remote || l[1].Name != "DeepSeek" || l[1].Detail != "deepseek-flash" {
		t.Fatalf("installed %+v", l)
	}
	if m, _ := local.Choose(l, ""); m.Remote {
		t.Error("automatic chose a remote service")
	}
	if m, _ := keys.Mask("llm.deepseek"); m != "sk-…3456" {
		t.Errorf("mask %q", m)
	}

	p, m, err := svc.ASR(context.Background(), "remote:openai")
	if err != nil || m.ID != "remote:openai" {
		t.Fatalf("asr %v %+v", err, m)
	}
	r, err := asr.Recognize(context.Background(), p, asr.Silence(100*time.Millisecond), "en", nil, "", asr.Feed{})
	if err != nil || r.Text != "hello" || gotKey != "Bearer sk-asr-123456" || gotModel != "whisper-1" {
		t.Errorf("recognize %+v %v %q %q", r, err, gotKey, gotModel)
	}
	c, _, err := svc.LLM(context.Background(), "remote:deepseek")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "hi"}}})
	if err != nil || resp.Text != "answer" || gotKey != "Bearer sk-llm-123456" {
		t.Errorf("chat %+v %v", resp, err)
	}

	// Not ready (off), and a local id: the local models.
	if _, _, err := svc.LLM(context.Background(), "remote:openai"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ASR(context.Background(), "asr.fast"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"llm:", "asr:asr.fast"}; len(lm.opened) != 2 || lm.opened[0] != want[0] || lm.opened[1] != want[1] {
		t.Errorf("local opens %v", lm.opened)
	}

	// A key removed: no longer ready.
	if err := keys.Set("llm.deepseek", "VoxtMini DeepSeek", ""); err != nil || keys.Has("llm.deepseek") {
		t.Errorf("delete %v", err)
	}
	// A Keychain that fails: the open fails, never another service.
	ring.err = errors.New("locked")
	if _, _, err := svc.ASR(context.Background(), "remote:openai"); err == nil {
		t.Error("an unreadable key opened")
	}
}

func TestMask(t *testing.T) {
	if Mask("abc") != "•••" || Mask("sk-1234567890") != "sk-…7890" {
		t.Error(Mask("abc"), Mask("sk-1234567890"))
	}
}
