package remote

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestSignInService: a service that signs in with an account (Codex) is
// ready once its sign-in is saved, and opens with the sign-in's headers,
// not the saved item as a key; web search reaches the request of a
// rewrite only.
func TestSignInService(t *testing.T) {
	var auth, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
	}))
	defer srv.Close()
	s := settings.Defaults()
	s.Providers.LLM = []settings.Provider{{ID: "codex", Enabled: true, Endpoint: srv.URL + "/codex", WebSearch: true}}
	ring := &memRing{keys: map[string]string{}}
	keys := NewKeys(ring)
	var asked string
	svc := New(Config{Local: &oneLocal{}, Settings: func() settings.Settings { return s }, Keys: keys,
		SignIn: func(acc string) llm.HeaderSource {
			asked = acc
			return llm.HeaderFunc(func(context.Context) (map[string]string, error) {
				return map[string]string{"Authorization": "Bearer access-1"}, nil
			})
		}})
	if len(svc.Ready(local.LLM)) != 0 {
		t.Fatal("ready before signing in")
	}
	ring.keys["llm.codex"] = `{"tokens":"…"}`
	_ = keys.Load([]string{"llm.codex"})
	if r := svc.Ready(local.LLM); len(r) != 1 || r[0].ID != "remote:codex" {
		t.Fatalf("ready %+v", r)
	}
	c, _, err := svc.LLM(context.Background(), "remote:codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "x"}}, Skill: llm.SkillTranscribeEnhance}); err != nil {
		t.Fatal(err)
	}
	if asked != "llm.codex" || auth != "Bearer access-1" || strings.Contains(body, "web_search") {
		t.Fatalf("dictation: account %q auth %q body %s", asked, auth, body)
	}
	if _, err := c.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "x"}}, Skill: llm.SkillRewrite}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"web_search"`) {
		t.Fatalf("rewrite did not search: %s", body)
	}
}
