package gemini

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/asr"
)

func TestEndpoint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", DefaultEndpoint},
		{"  ", DefaultEndpoint},
		{"https://generativelanguage.googleapis.com", DefaultEndpoint},
		{"https://generativelanguage.googleapis.com/", DefaultEndpoint},
		{"http://127.0.0.1:8080", "ws://127.0.0.1:8080" + defaultPath},
		{"wss://relay.example.com/live?key=abc", "wss://relay.example.com/live"},
		{"wss://relay.example.com/live?key=abc&x=1", "wss://relay.example.com/live?x=1"},
		{"wss://relay.example.com/live?x=1", "wss://relay.example.com/live?x=1"},
	}
	for _, c := range cases {
		if got := Endpoint(c.in); got != c.want {
			t.Errorf("Endpoint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNew(t *testing.T) {
	if _, err := New(asr.Config{}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("no key: %v", err)
	}
	if _, err := New(asr.Config{Key: "k", Endpoint: "ws://203.0.113.5/x"}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("plain ws to a public address: %v", err)
	}
	c, err := New(asr.Config{Key: " k ", Endpoint: "https://example.com?key=old"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Model() != DefaultModel || c.Endpoint() != "wss://example.com"+defaultPath {
		t.Errorf("model %q endpoint %q", c.Model(), c.Endpoint())
	}
	if got := c.connectURL(); got != "wss://example.com"+defaultPath+"?key=k" {
		t.Errorf("connectURL = %q", got)
	}
	if caps := c.Caps(); !caps.Stream || !caps.Hints || caps.File || caps.Prompt {
		t.Errorf("caps %+v", caps)
	}
}

func TestSetupMessage(t *testing.T) {
	got := string(setupMessage("gemini-x", asr.StreamRequest{Lang: "zh-Hans", Others: []string{"en", "ZH", "ja_JP", ""},
		Hints: []string{" Voxt ", "Voxt", "", "MyGo"}}))
	want := `{"setup":{"model":"models/gemini-x","generationConfig":{"responseModalities":["TEXT"]},` +
		`"inputAudioTranscription":{"languageCodes":["zh","en","ja"],"mode":"SMART","customVocabulary":["Voxt","MyGo"]}}}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	// No language: detect (an empty list, not a missing one); no hints: no vocabulary.
	got = string(setupMessage("models/m", asr.StreamRequest{Others: []string{"en"}}))
	want = `{"setup":{"model":"models/m","generationConfig":{"responseModalities":["TEXT"]},` +
		`"inputAudioTranscription":{"languageCodes":[],"mode":"SMART"}}}`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := string(setupMessage(" ", asr.StreamRequest{})); !json.Valid([]byte(got)) || !strings.Contains(got, `"models/`+DefaultModel+`"`) {
		t.Errorf("default model: %s", got)
	}
}

func TestVocabularyLimit(t *testing.T) {
	var hints []string
	for i := range 1200 {
		hints = append(hints, fmt.Sprint("w", i))
	}
	if v := vocabulary(hints); len(v) != maxVocabulary || v[999] != "w999" {
		t.Errorf("len %d", len(v))
	}
}

func TestAudioMessages(t *testing.T) {
	if got := string(audioMessage([]byte{1, 2, 3}, 16000)); got != `{"realtimeInput":{"audio":{"data":"AQID","mimeType":"audio/pcm;rate=16000"}}}` {
		t.Errorf("audio: %s", got)
	}
	if got := string(endMessage()); got != `{"realtimeInput":{"audioStreamEnd":true}}` {
		t.Errorf("end: %s", got)
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want message
	}{
		{`{"setupComplete":{}}`, message{setupComplete: true}},
		{`{"setup_complete":{}}`, message{setupComplete: true}},
		{`{"serverContent":{"inputTranscription":{"text":" hello "}}}`, message{final: "hello"}},
		{`{"server_content":{"input_transcription":{"text":"你好"},"turn_complete":true}}`, message{final: "你好", turnComplete: true}},
		{`{"serverContent":{"interimInputTranscription":{"text":"hel"}}}`, message{interim: "hel"}},
		{`{"server_content":{"interim_input_transcription":{"text":"hel"}}}`, message{interim: "hel"}},
		{`{"serverContent":{"turnComplete":true}}`, message{turnComplete: true}},
		{`{"goAway":{"timeLeft":"5s"}}`, message{goAway: true}},
		{`{"go_away":{}}`, message{goAway: true}},
		{`{"usageMetadata":{}}`, message{}},
	}
	for _, c := range cases {
		got, ok := parse([]byte(c.in))
		if !ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("parse(%s) = %+v %v, want %+v", c.in, got, ok, c.want)
		}
	}
	if _, ok := parse([]byte(`not json`)); ok {
		t.Error("not JSON parsed")
	}
	m, _ := parse([]byte(`{"error":{"code":403,"message":"denied","status":"PERMISSION_DENIED"}}`))
	if m.err == nil || m.err.Kind != asr.KindAuth || m.err.Status != 403 || m.err.Detail != "denied" {
		t.Errorf("error packet: %+v", m.err)
	}
}

func TestServiceError(t *testing.T) {
	cases := []struct {
		code      int
		status    string
		msg       string
		kind      asr.Kind
		retryable bool
	}{
		{401, "UNAUTHENTICATED", "no", asr.KindAuth, false},
		{0, "PERMISSION_DENIED", "", asr.KindAuth, false},
		{400, "INVALID_ARGUMENT", "API key not valid. Please pass a valid API key.", asr.KindAuth, false},
		{429, "RESOURCE_EXHAUSTED", "You exceeded your current quota", asr.KindQuota, false},
		{0, "RESOURCE_EXHAUSTED", "rate limit exceeded", asr.KindQuota, true},
		{404, "NOT_FOUND", "models/x is not found for API version v1beta", asr.KindConfig, false},
		{400, "INVALID_ARGUMENT", "models/x is not supported for bidiGenerateContent", asr.KindConfig, false},
		{400, "INVALID_ARGUMENT", "Request contains an invalid argument.", asr.KindProtocol, false},
		{400, "FAILED_PRECONDITION", "User location is not supported for the API use.", asr.KindConfig, false},
		{500, "INTERNAL", "oops", asr.KindServer, true},
		{0, "UNAVAILABLE", "", asr.KindServer, true},
		{0, "", "", asr.KindProtocol, false},
	}
	for _, c := range cases {
		e := serviceError(c.code, c.status, c.msg)
		if e.Kind != c.kind || e.Retryable != c.retryable || e.Provider != ID || e.Detail == "" {
			t.Errorf("serviceError(%d, %q, %q) = %+v", c.code, c.status, c.msg, e)
		}
	}
	if e := packetError(json.RawMessage(`"boom"`)); e.Detail != "boom" || e.Kind != asr.KindProtocol {
		t.Errorf("string error: %+v", e)
	}
}

func TestJoin(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"hello", "world"}, "hello world"},
		{[]string{" 你好", "世界 "}, "你好世界"},
		{[]string{"我用", "Voxt", "写字"}, "我用Voxt写字"},
		{[]string{"Hello,", "", "  ", "こんにちは", "。", "ok"}, "Hello,こんにちは。ok"},
		{[]string{"안녕", "Kim"}, "안녕Kim"},
		{[]string{"a.", "b"}, "a. b"},
	}
	for _, c := range cases {
		if got := join(c.in); got != c.want {
			t.Errorf("join(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
