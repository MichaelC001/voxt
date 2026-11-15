package mimo

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
)

func TestTranscribe(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":" 你好，世界。 "}}]}`)
	}))
	defer srv.Close()
	c, err := New(asr.Config{Endpoint: srv.URL + "/v1", Key: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Transcribe(context.Background(), asr.FileRequest{Audio: asr.Silence(100 * time.Millisecond), Format: "pcm", Lang: "zh-Hans"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "你好，世界。" || r.Model != DefaultModel {
		t.Errorf("result %+v", r)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("auth %q", auth)
	}
	if got["model"] != DefaultModel || got["asr_options"].(map[string]any)["language"] != "zh" {
		t.Errorf("body %v", got)
	}
	msgs := got["messages"].([]any)
	part := msgs[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if part["type"] != "input_audio" || !strings.HasPrefix(part["input_audio"].(map[string]any)["data"].(string), "data:audio/wav;base64,UklGR") {
		t.Errorf("audio part %v", part)
	}
}

func TestErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer srv.Close()
	c, _ := New(asr.Config{Endpoint: srv.URL, Key: "sk-bad"})
	_, err := c.Transcribe(context.Background(), asr.FileRequest{Audio: asr.Silence(10 * time.Millisecond), Format: "pcm"})
	if asr.KindOf(err) != asr.KindAuth {
		t.Errorf("err %v, want auth", err)
	}
	if _, err := New(asr.Config{}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("no key: %v", err)
	}
	if _, err := New(asr.Config{Key: "k", Endpoint: "http://203.0.113.5/v1"}); err == nil {
		t.Error("plain http to a public address accepted")
	}
}

func TestLanguageAndEndpoint(t *testing.T) {
	for in, want := range map[string]string{"": "auto", "zh": "zh", "zh-Hans": "zh", "EN": "en", "ja": "auto"} {
		if got := Language(in); got != want {
			t.Errorf("Language(%q) = %q, want %q", in, got, want)
		}
	}
	if Endpoint("") != DefaultEndpoint || Endpoint("https://x/v1/") != "https://x/v1/chat/completions" || Endpoint("https://x/v1/chat/completions") != "https://x/v1/chat/completions" {
		t.Error("Endpoint")
	}
}
