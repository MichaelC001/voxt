package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/remote"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

// memKeyring is the Keychain of the app's tests: never the real one,
// also on a Mac.
type memKeyring struct {
	mu   sync.Mutex
	keys map[string]string
}

func (m *memKeyring) Get(acc string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keys[acc], nil
}

func (m *memKeyring) Set(acc, _, v string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v == "" {
		delete(m.keys, acc)
	} else {
		m.keys[acc] = v
	}
	return nil
}

func init() {
	newKeyring = func() remote.Keyring { return &memKeyring{keys: map[string]string{}} }
}

// remoteApp is a test app whose local models are the fakes (readyLocal)
// under the remote services, as openModels wraps them.
func remoteApp(t *testing.T, ring *memKeyring) (*app, *ui.Tester) {
	t.Helper()
	a, _, tt := newTestApp(t)
	tt.SetSize(windowWidth, 2400)
	a.rkeys = remote.NewKeys(ring)
	a.local = remote.New(remote.Config{Local: readyLocal(), Settings: a.lib.settings.Get, Keys: a.rkeys})
	return a, tt
}

// TestRemotePage: the services of both kinds, set up in the dialog with
// a key and advanced parameters, then chosen on a feature's page.
func TestRemotePage(t *testing.T) {
	ring := &memKeyring{keys: map[string]string{}}
	a, tt := remoteApp(t, ring)
	a.navigate(remotePath) // the Models page, its Remote filter
	if a.router.Path() != modelsPath || a.mdFilter != filterRemote || a.mdTab != 0 {
		t.Fatalf("/remote: %s, filter %d, tab %d", a.router.Path(), a.mdFilter, a.mdTab)
	}
	for _, s := range []string{"Models", "Doubao (Volcengine)", "Alibaba Cloud Model Studio", "OpenAI Transcribe",
		"StepFun", "Z.ai GLM-ASR", "Xiaomi MiMo", "Streaming", "Upload", "Remote services"} {
		if !has(tt, s) {
			t.Errorf("no %q in %q", s, firstTexts(tt))
		}
	}
	a.mdTab = 1
	frames(tt)
	for _, s := range []string{"DeepSeek", "Anthropic", "Ollama", "LM Studio", "oMLX", "Custom (OpenAI-compatible)"} {
		if !has(tt, s) {
			t.Errorf("no %q in %q", s, firstTexts(tt))
		}
	}

	// Set DeepSeek up: its key, a model, the advanced parameters.
	a.openProviderDialog(providers.LLM, "deepseek")
	frames(tt)
	if !has(tt, "When a feature uses it, recognised and selected text is sent to DeepSeek.") {
		t.Errorf("dialog %q", firstTexts(tt))
	}
	click(t, tt, "Key for DeepSeek")
	tt.Type("sk-test-123456")
	frames(tt)
	if a.rmDlg.key != "sk-test-123456" {
		t.Fatalf("key typed %q", a.rmDlg.key)
	}
	a.rmDlg.temp, a.rmDlg.body = "3", ""
	click(t, tt, "Save")
	if !strings.Contains(a.rmDlg.err, "Temperature") {
		t.Fatalf("a temperature of 3 saved: %q", a.rmDlg.err)
	}
	a.rmDlg.temp, a.rmDlg.maxTok, a.rmDlg.body = "0.4", "900", `{"x"`
	click(t, tt, "Save")
	if !strings.Contains(a.rmDlg.err, "JSON object") {
		t.Fatalf("bad JSON saved: %q", a.rmDlg.err)
	}
	a.rmDlg.body = `{"thinking":{"type":"enabled"}}`
	click(t, tt, "Save")
	if a.rmDlg.open || ring.keys["llm.deepseek"] != "sk-test-123456" {
		t.Fatalf("not saved: %+v %v", a.rmDlg, ring.keys)
	}
	p, ok := providers.Setup(a.lib.settings.Get(), providers.LLM, "deepseek")
	if !ok || !p.Enabled || p.Temperature == nil || *p.Temperature != 0.4 || p.MaxTokens != 900 || p.ExtraBody == "" {
		t.Fatalf("setup %+v", p)
	}
	frames(tt)
	if !has(tt, "Remote services you set up") || !has(tt, "sk-…3456") || !has(tt, "Test DeepSeek") {
		t.Errorf("card %q", firstTexts(tt))
	}

	// Automatic stays local; the remote service is offered and chosen.
	if m, ok := a.featureModel(local.LLM, hotkey.Transcribe); !ok || m.Remote {
		t.Errorf("automatic %+v", m)
	}
	a.navigate("/transcribe")
	click(t, tt, "Language model for Transcribe")
	click(t, tt, "DeepSeek · deepseek-flash (remote)")
	if id := a.lib.settings.Get().Features.Transcribe.LLM; id != "remote:deepseek" {
		t.Fatalf("LLM %q", id)
	}
	if !has(tt, "Recognised and selected text is sent to DeepSeek · deepseek-flash (remote).") {
		t.Errorf("no notice: %q", firstTexts(tt))
	}
	if m, ok := a.featureModel(local.LLM, hotkey.Transcribe); !ok || m.ID != "remote:deepseek" {
		t.Errorf("chosen %+v", m)
	}

	// Its key removed: no longer offered, the feature is back on the
	// local model.
	a.navigate(remotePath)
	a.openProviderDialog(providers.LLM, "deepseek")
	frames(tt)
	click(t, tt, "Remove the saved key")
	click(t, tt, "Save")
	if _, ok := ring.keys["llm.deepseek"]; ok {
		t.Fatal("key kept")
	}
	if m, _ := a.featureModel(local.LLM, hotkey.Transcribe); m.Remote {
		t.Errorf("still remote: %+v", m)
	}
}

// TestRemoteSession: a session's resolver opens the chosen remote
// services (fake servers on loopback) with their keys, and the history
// names them.
func TestRemoteSession(t *testing.T) {
	var mu sync.Mutex
	var asrAuth, chatBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/audio/transcriptions") {
			asrAuth = r.Header.Get("Authorization")
			io.WriteString(w, `{"text":"hello from the cloud"}`)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		chatBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"model":"deepseek-chat","choices":[{"message":{"content":"Hello from the cloud."},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	ring := &memKeyring{keys: map[string]string{"asr.openai": "sk-asr", "llm.deepseek": "sk-llm"}}
	a, _ := remoteApp(t, ring)
	temp := 0.2
	a.updateSettings(func(s *settings.Settings) {
		s.Providers.ASR = []settings.Provider{{ID: "openai", Enabled: true, Endpoint: srv.URL + "/v1", Language: "en"}}
		s.Providers.LLM = []settings.Provider{{ID: "deepseek", Enabled: true, Endpoint: srv.URL + "/v1", Model: "deepseek-chat", Temperature: &temp}}
		s.Features.Transcribe.ASR = settings.RemoteID("openai")
		s.Features.Transcribe.LLM = settings.RemoteID("deepseek")
		s.Features.Transcribe.Enhance = true
	})
	if err := a.rkeys.Load([]string{"asr.openai", "llm.deepseek"}); err != nil {
		t.Fatal(err)
	}
	svc, err := resolver(a.local)(context.Background(), store.KindTranscribe, a.lib.settings.Get())
	if err != nil {
		t.Fatal(err)
	}
	if svc.ASRInfo.ID != "remote:openai" || svc.LLMInfo.ID != "remote:deepseek" || svc.LLMInfo.Model != "deepseek-chat" || svc.VAD != nil {
		t.Fatalf("services %+v %+v", svc.ASRInfo, svc.LLMInfo)
	}
	r, err := asr.Recognize(context.Background(), svc.ASR, asr.Silence(200*time.Millisecond), "zh", nil, "", asr.Feed{})
	if err != nil || r.Text != "hello from the cloud" || asrAuth != "Bearer sk-asr" {
		t.Fatalf("recognize %+v %v %q", r, err, asrAuth)
	}
	resp, err := svc.LLM.Chat(context.Background(), llm.Request{Skill: llm.SkillTranscribeEnhance, Messages: []llm.Message{{Role: llm.User, Content: "hello"}}})
	if err != nil || resp.Text != "Hello from the cloud." {
		t.Fatalf("chat %+v %v", resp, err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(chatBody), &body); err != nil || body["model"] != "deepseek-chat" || body["temperature"] != 0.2 {
		t.Errorf("chat body %s", chatBody)
	}

	// Automatic never sends anything away: the local fakes.
	a.updateSettings(func(s *settings.Settings) { s.Features.Transcribe.ASR, s.Features.Transcribe.LLM = "", "" })
	svc, err = resolver(a.local)(context.Background(), store.KindTranscribe, a.lib.settings.Get())
	if err != nil || svc.ASRInfo.ID != fakeASRModel.ID || svc.LLMInfo.ID != fakeLLMModel.ID {
		t.Errorf("automatic %+v %+v %v", svc.ASRInfo, svc.LLMInfo, err)
	}
}

// remoteShots are the Remote models page's snapshots: services set up,
// the dialog with its advanced section, and a feature using one.
func remoteShots(a *app, save func(string)) {
	ring := &memKeyring{keys: map[string]string{"llm.deepseek": "sk-1234567890abcd", "asr.doubao": "tok-1234567890"}}
	a.rkeys = remote.NewKeys(ring)
	old := a.local
	a.local = remote.New(remote.Config{Local: old, Settings: a.lib.settings.Get, Keys: a.rkeys})
	a.updateSettings(func(s *settings.Settings) {
		s.Providers.ASR = []settings.Provider{{ID: "doubao", Enabled: true, AppID: "app-1"}, {ID: "openai", Enabled: true}}
		s.Providers.LLM = []settings.Provider{{ID: "deepseek", Enabled: true, Model: "deepseek-chat"}}
	})
	_ = a.rkeys.Load([]string{"llm.deepseek", "asr.doubao", "asr.openai"})
	a.navigate(remotePath)
	a.mdTab = 0
	save("remote-speech")
	a.mdTab = 1
	save("remote-language")
	a.mdTab, a.mdFilter = 0, filterAll
	a.openProviderDialog(providers.LLM, "deepseek")
	a.rmDlg.advanced, a.rmDlg.temp = true, "0.3"
	save("remote-dialog")
	a.rmDlg = providerDialog{}
	a.openProviderDialog(providers.ASR, "doubao")
	a.rmDlg.advanced = true
	save("remote-dialog-speech")
	a.rmDlg = providerDialog{}
	a.openProviderDialog(providers.LLM, "codex") // codex_signin_view.go
	save("remote-dialog-codex")
	a.rmDlg = providerDialog{}
	a.openProviderDialog(providers.LLM, "openai") // remote_dialog_api.go
	a.rmDlg.advanced, a.rmDlg.api, a.rmDlg.search = true, llm.APIResponses, true
	save("remote-dialog-search")
	a.rmDlg = providerDialog{}
	familyOfferShot(a, save) // family_keys_test.go
	a.updateSettings(func(s *settings.Settings) { s.Features.Translate.LLM = settings.RemoteID("deepseek") })
	a.navigate("/translate")
	save("translate-remote")
	a.updateSettings(func(s *settings.Settings) {
		s.Providers = settings.Providers{}
		s.Features.Translate.LLM = ""
	})
	a.local = old
}
