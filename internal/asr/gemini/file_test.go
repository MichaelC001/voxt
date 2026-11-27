package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hehehai/VoxtMini/internal/asr"
)

func TestUseLive(t *testing.T) {
	cases := []struct {
		model, endpoint string
		want            bool
	}{
		{"", "", true},
		{DefaultModel, "", true},
		{"GEMINI-2.0-FLASH-LIVE-001", "", true},
		{"gemini-4-hydrogen", "", false},
		{"gemini-4-neon", "", false},
		{"gemini-4-carbon", "https://generativelanguage.googleapis.com", false},
		{"gemini-4-argon", "https://proxy.example.com/v1beta", false},
		{"gemini-4-hydrogen", "http://100.88.1.2:8080/v1beta", false},
		{"gemini-4-hydrogen", "wss://relay.example.com/live", true},
		{"gemini-4-hydrogen", "ws://127.0.0.1:8080", true},
		{"gemini-4-hydrogen", "https://relay.example.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent", true},
		{DefaultModel, "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-transcribe-live:generateContent", false},
		{"", "https://example.com?key=old", true},
	}
	for _, c := range cases {
		if got := UseLive(c.model, c.endpoint); got != c.want {
			t.Errorf("UseLive(%q, %q) = %v, want %v", c.model, c.endpoint, got, c.want)
		}
	}
}

func TestNewProvider(t *testing.T) {
	p, err := NewProvider(asr.Config{Key: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(*Client); !ok || !p.Caps().Stream || p.Caps().File {
		t.Errorf("default provider: %T %+v", p, p.Caps())
	}
	p, err = NewProvider(asr.Config{Key: "k", Model: "gemini-4-hydrogen"})
	if err != nil {
		t.Fatal(err)
	}
	fc, ok := p.(*FileClient)
	if !ok || !fc.Caps().File || fc.Caps().Stream || !fc.Caps().Hints || !fc.Caps().Prompt {
		t.Errorf("file provider: %T %+v", p, p.Caps())
	}
	if fc.Model() != "gemini-4-hydrogen" {
		t.Errorf("model = %q", fc.Model())
	}
	// Explicit wss:// routes a non-live model to Live.
	p, err = NewProvider(asr.Config{Key: "k", Model: "gemini-4-hydrogen", Endpoint: "wss://relay.example.com/live"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(*Client); !ok {
		t.Errorf("wss override: %T", p)
	}
}

func TestFileEndpoint(t *testing.T) {
	cases := []struct {
		raw, model, want string
	}{
		{"", "gemini-4-hydrogen", "https://generativelanguage.googleapis.com/v1beta/models/gemini-4-hydrogen:generateContent"},
		{"", "models/gemini-4-neon", "https://generativelanguage.googleapis.com/v1beta/models/gemini-4-neon:generateContent"},
		{"https://generativelanguage.googleapis.com", "gemini-4-hydrogen", "https://generativelanguage.googleapis.com/v1beta/models/gemini-4-hydrogen:generateContent"},
		{"https://generativelanguage.googleapis.com/", "gemini-4-hydrogen", "https://generativelanguage.googleapis.com/v1beta/models/gemini-4-hydrogen:generateContent"},
		{"https://generativelanguage.googleapis.com/v1beta", "gemini-4-hydrogen", "https://generativelanguage.googleapis.com/v1beta/models/gemini-4-hydrogen:generateContent"},
		{"https://generativelanguage.googleapis.com/v1/", "gemini-4-hydrogen", "https://generativelanguage.googleapis.com/v1/models/gemini-4-hydrogen:generateContent"},
		{"https://generativelanguage.googleapis.com/v1beta/models", "gemini-4-hydrogen", "https://generativelanguage.googleapis.com/v1beta/models/gemini-4-hydrogen:generateContent"},
		{"https://proxy.example.com/gemini", "gemini-4-hydrogen", "https://proxy.example.com/gemini/v1beta/models/gemini-4-hydrogen:generateContent"},
		{"https://proxy.example.com/v1beta/models/custom:generateContent?key=secret&x=1", "gemini-4-hydrogen", "https://proxy.example.com/v1beta/models/custom:generateContent?x=1"},
		{"http://127.0.0.1:8080/v1beta?key=secret", "gemini-4-hydrogen", "http://127.0.0.1:8080/v1beta/models/gemini-4-hydrogen:generateContent"},
	}
	for _, c := range cases {
		if got := FileEndpoint(c.raw, c.model); got != c.want {
			t.Errorf("FileEndpoint(%q, %q) = %q, want %q", c.raw, c.model, got, c.want)
		}
	}
}

func TestNewFileValidation(t *testing.T) {
	if _, err := NewFile(asr.Config{Model: "gemini-4-hydrogen"}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("no key: %v", err)
	}
	if _, err := NewFile(asr.Config{Key: "k", Model: "gemini-4-hydrogen", Endpoint: "http://203.0.113.5/v1beta"}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("public plain http: %v", err)
	}
	if _, err := NewFile(asr.Config{Key: "k", Model: "gemini-4-hydrogen", Endpoint: "wss://example.com/live"}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("wss scheme for file client: %v", err)
	}
	for _, ep := range []string{"", "https://proxy.example.com/v1beta", "http://127.0.0.1:8080", "http://100.88.1.2:8080/v1beta"} {
		if _, err := NewFile(asr.Config{Key: "k", Model: "gemini-4-hydrogen", Endpoint: ep}); err != nil {
			t.Errorf("NewFile(%q): %v", ep, err)
		}
	}
}

func TestFileTranscribe(t *testing.T) {
	var (
		gotPath  string
		gotQuery string
		gotKey   string
		gotAuth  string
		gotBody  generateRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("x-goog-api-key")
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [
						{"text": "internal reasoning", "thought": true},
						{"text": "Hello "},
						{"text": "VoxtMini 世界"}
					]
				},
				"finishReason": "STOP"
			}]
		}`))
	}))
	defer srv.Close()

	c, err := NewFile(asr.Config{
		Endpoint: srv.URL + "/v1beta?key=should-strip",
		Model:    "gemini-4-hydrogen",
		Key:      testKey,
		Prompt:   "software engineering notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]byte, 3200) // 100ms PCM16
	res, err := c.Transcribe(context.Background(), asr.FileRequest{
		Audio:      pcm,
		Format:     "pcm",
		SampleRate: 16000,
		Lang:       "zh-Hans",
		Others:     []string{"en"},
		Hints:      []string{"VoxtMini", "MyGo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Hello VoxtMini 世界" || res.Model != "gemini-4-hydrogen" || res.Latency <= 0 {
		t.Errorf("result = %+v", res)
	}
	if gotPath != "/v1beta/models/gemini-4-hydrogen:generateContent" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "" || gotKey != testKey || gotAuth != "" {
		t.Errorf("query=%q key=%q auth=%q", gotQuery, gotKey, gotAuth)
	}
	if gotBody.SystemInstruction == nil || len(gotBody.SystemInstruction.Parts) != 1 ||
		!strings.Contains(gotBody.SystemInstruction.Parts[0].Text, "Never answer questions") {
		t.Errorf("systemInstruction = %+v", gotBody.SystemInstruction)
	}
	if gotBody.GenerationConfig.Temperature != 0 || gotBody.GenerationConfig.ThinkingConfig == nil ||
		gotBody.GenerationConfig.ThinkingConfig.ThinkingBudget != 0 {
		t.Errorf("generationConfig = %+v", gotBody.GenerationConfig)
	}
	if len(gotBody.Contents) != 1 || len(gotBody.Contents[0].Parts) != 2 {
		t.Fatalf("contents = %+v", gotBody.Contents)
	}
	in := gotBody.Contents[0].Parts[0].InlineData
	if in == nil || in.MimeType != "audio/wav" {
		t.Fatalf("inlineData = %+v", in)
	}
	wav, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		t.Fatal(err)
	}
	decodedPCM, rate, ch, err := asr.ReadWAV(wav)
	if err != nil || len(decodedPCM) != len(pcm) || rate != 16000 || ch != 1 {
		t.Errorf("WAV: len=%d rate=%d ch=%d err=%v", len(decodedPCM), rate, ch, err)
	}
	promptText := gotBody.Contents[0].Parts[1].Text
	for _, want := range []string{"zh", "en", "VoxtMini, MyGo", "software engineering notes"} {
		if !strings.Contains(promptText, want) {
			t.Errorf("prompt missing %q: %s", want, promptText)
		}
	}
}

func TestFileThinkingFallback(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		var body generateRequest
		json.NewDecoder(r.Body).Decode(&body)
		if n == 1 {
			if body.GenerationConfig.ThinkingConfig == nil {
				t.Error("first call should send thinkingConfig")
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"code":400,"message":"Budget 0 is invalid. This model only works in thinking mode.","status":"INVALID_ARGUMENT"}}`))
			return
		}
		if body.GenerationConfig.ThinkingConfig != nil {
			t.Error("second call should omit thinkingConfig")
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hmm","thought":true},{"text":"transcribed ok"}]},"finishReason":"STOP"}]}`))
	}))
	defer srv.Close()

	c, err := NewFile(asr.Config{Endpoint: srv.URL, Model: "gemini-4-neon", Key: testKey})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Transcribe(context.Background(), asr.FileRequest{Audio: asr.WAV(make([]byte, 320), 16000), Format: "wav"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "transcribed ok" || calls.Load() != 2 {
		t.Errorf("res=%+v calls=%d", res, calls.Load())
	}
}

func TestFileSilenceAndBlocks(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr asr.Kind
	}{
		{"empty candidates", 200, `{"candidates":[]}`, "", ""},
		{"empty object", 200, `{}`, "", ""},
		{"empty text part", 200, `{"candidates":[{"content":{"parts":[{"text":"   "}]},"finishReason":"STOP"}]}`, "", ""},
		{"no parts", 200, `{"candidates":[{"content":{},"finishReason":"STOP"}]}`, "", ""},
		{"400 no speech", 400, `{"error":{"code":400,"message":"no speech detected","status":"INVALID_ARGUMENT"}}`, "", ""},
		{"prompt blocked", 200, `{"promptFeedback":{"blockReason":"SAFETY"}}`, "", asr.KindProtocol},
		{"candidate safety", 200, `{"candidates":[{"content":{},"finishReason":"SAFETY"}]}`, "", asr.KindProtocol},
		{"bad json", 200, `not json`, "", asr.KindProtocol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c, err := NewFile(asr.Config{Endpoint: srv.URL, Model: "gemini-4-hydrogen", Key: testKey})
			if err != nil {
				t.Fatal(err)
			}
			res, err := c.Transcribe(context.Background(), asr.FileRequest{Audio: make([]byte, 320), Format: "pcm"})
			if tc.wantErr != "" {
				if asr.KindOf(err) != tc.wantErr {
					t.Errorf("err = %v, want kind %s", err, tc.wantErr)
				}
				return
			}
			if err != nil || res.Text != tc.want {
				t.Errorf("res=%+v err=%v", res, err)
			}
		})
	}
}

func TestFileErrorsAndRedaction(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		kind      asr.Kind
		retryable bool
	}{
		{"bad key 400", 400, `{"error":{"code":400,"message":"API key not valid: ` + testKey + `","status":"INVALID_ARGUMENT"}}`, asr.KindAuth, false},
		{"forbidden 403", 403, `{"error":{"code":403,"message":"denied","status":"PERMISSION_DENIED"}}`, asr.KindAuth, false},
		{"not found 404", 404, `{"error":{"code":404,"message":"models/bad is not found","status":"NOT_FOUND"}}`, asr.KindConfig, false},
		{"quota 429", 429, `{"error":{"code":429,"message":"quota exceeded","status":"RESOURCE_EXHAUSTED"}}`, asr.KindQuota, false},
		{"rate limit 429", 429, `{"error":{"code":429,"message":"rate limit exceeded","status":"RESOURCE_EXHAUSTED"}}`, asr.KindQuota, true},
		{"server 500", 500, `{"error":{"code":500,"message":"internal","status":"INTERNAL"}}`, asr.KindServer, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c, err := NewFile(asr.Config{Endpoint: srv.URL, Model: "gemini-4-hydrogen", Key: testKey})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Transcribe(context.Background(), asr.FileRequest{Audio: make([]byte, 320), Format: "pcm"})
			if asr.KindOf(err) != tc.kind {
				t.Errorf("kind = %s (%v), want %s", asr.KindOf(err), err, tc.kind)
			}
			var ae *asr.Error
			if err != nil && asAsr(err, &ae) && ae.Retryable != tc.retryable {
				t.Errorf("retryable = %v, want %v", ae.Retryable, tc.retryable)
			}
			if err != nil && strings.Contains(err.Error(), testKey) {
				t.Errorf("key leaked in error: %s", err.Error())
			}
		})
	}

	// Audio over MaxFileBytes fails before any HTTP request.
	c, err := NewFile(asr.Config{Endpoint: "https://example.com", Model: "gemini-4-hydrogen", Key: testKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Transcribe(context.Background(), asr.FileRequest{Audio: make([]byte, MaxFileBytes+1), Format: "wav"}); asr.KindOf(err) != asr.KindAudio {
		t.Errorf("oversized audio: %v", err)
	}

	// Cancelled context returns KindCancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Transcribe(ctx, asr.FileRequest{Audio: make([]byte, 320), Format: "pcm"}); asr.KindOf(err) != asr.KindCancelled {
		t.Errorf("cancelled: %v", err)
	}
}
