package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestSpeechParams: a setup's language and dictionary hints reach the
// upload (an OpenAI-compatible server on loopback).
func TestSpeechParams(t *testing.T) {
	var form map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		form = map[string]string{"language": r.FormValue("language"), "prompt": r.FormValue("prompt")}
		io.WriteString(w, `{"text":"ok"}`)
	}))
	defer srv.Close()
	run := func(p settings.Provider) map[string]string {
		t.Helper()
		p.ID, p.Enabled, p.Endpoint = "openai", true, srv.URL+"/v1"
		c, err := NewASR(p, Env{Key: "k"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := asr.Recognize(context.Background(), c, asr.Silence(100*time.Millisecond), "zh", []string{"VoxtMini"}, "", asr.Feed{}); err != nil {
			t.Fatal(err)
		}
		return form
	}
	if f := run(settings.Provider{}); f["language"] != "zh" || !strings.Contains(f["prompt"], "VoxtMini") {
		t.Errorf("defaults %v", f)
	}
	if f := run(settings.Provider{Language: "en", NoHints: true}); f["language"] != "en" || strings.Contains(f["prompt"], "VoxtMini") {
		t.Errorf("en, no hints %v", f)
	}
	if f := run(settings.Provider{Language: LanguageAuto}); f["language"] != "" {
		t.Errorf("auto %v", f)
	}
}

// TestChatParams: temperature, top-p, the output budget and the extra
// body reach a chat request.
func TestChatParams(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	temp, top := 0.3, 0.9
	p := settings.Provider{ID: "deepseek", Enabled: true, Endpoint: srv.URL + "/v1", Model: "deepseek-chat",
		Temperature: &temp, TopP: &top, MaxTokens: 777, ExtraBody: `{"thinking":{"type":"enabled"},"x":1}`}
	b, err := NewLLM(p, Env{Key: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	req := llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "hi"}}, Temperature: llm.F64(0)}
	ChatParams(p, &req)
	if _, err := b.Chat(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	th, _ := body["thinking"].(map[string]any)
	if body["model"] != "deepseek-chat" || body["temperature"] != 0.3 || body["top_p"] != 0.9 || body["max_tokens"] != 777.0 || th["type"] != "enabled" || body["x"] != 1.0 {
		t.Errorf("body %v", body)
	}
	if _, err := ExtraBody("[1]"); err != ErrExtraBody {
		t.Errorf("a list: %v", err)
	}
	if m, err := ExtraBody(" "); m != nil || err != nil {
		t.Errorf("empty: %v %v", m, err)
	}
}

// TestLLMConfigAPI: the API and web search reach the llm configuration,
// and the dialog's switch follows the API chosen.
func TestLLMConfigAPI(t *testing.T) {
	p := settings.Provider{ID: "openai", API: llm.APIResponses, WebSearch: true}
	if c := LLMConfig(p); c.API != llm.APIResponses || !c.WebSearch {
		t.Errorf("config %+v", c)
	}
	if !SearchSupported(p) || SearchSupported(settings.Provider{ID: "openai"}) {
		t.Error("openai: search only over the Responses API")
	}
	if !SearchSupported(settings.Provider{ID: "anthropic"}) || !SearchSupported(settings.Provider{ID: "codex"}) || SearchSupported(settings.Provider{ID: "deepseek"}) {
		t.Error("search support")
	}
	if err := CheckEndpoint(LLM, settings.Provider{ID: "deepseek", API: llm.APIResponses}); err == nil {
		t.Error("DeepSeek has no Responses API")
	}
	if pr, _ := LookupPreset(LLM, "codex"); !pr.SignIn || pr.KeyOptional {
		t.Errorf("codex %+v", pr)
	}
	if pr, _ := LookupPreset(LLM, "openai"); !pr.Responses {
		t.Errorf("openai %+v", pr)
	}
}
