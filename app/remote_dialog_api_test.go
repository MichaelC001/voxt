package main

import (
	"testing"

	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/providers"
)

// TestChatAPISearch: OpenAI offers the Responses API in Advanced, and
// web search only once it is chosen; DeepSeek offers neither; Anthropic
// searches over its own API.
func TestChatAPISearch(t *testing.T) {
	ring := &memKeyring{keys: map[string]string{}}
	a, tt := remoteApp(t, ring)
	a.navigate(remotePath)
	a.mdTab = 1
	a.openProviderDialog(providers.LLM, "openai")
	a.rmDlg.advanced = true
	frames(tt)
	if !has(tt, "Chat Completions") || has(tt, "Search the web when rewriting") {
		t.Fatalf("chat: %q", firstTexts(tt))
	}
	a.rmDlg.api = llm.APIResponses // the select's "Responses"
	frames(tt)
	if a.rmDlg.api != llm.APIResponses || !has(tt, "Search the web when rewriting") {
		t.Fatalf("responses: %q %q", a.rmDlg.api, firstTexts(tt))
	}
	a.rmDlg.search = true // the switch
	click(t, tt, "Key for OpenAI")
	tt.Type("sk-openai-0000")
	click(t, tt, "Save")
	p, _ := providers.Setup(a.lib.settings.Get(), providers.LLM, "openai")
	if p.API != llm.APIResponses || !p.WebSearch {
		t.Fatalf("saved %+v (%q)", p, a.rmDlg.err)
	}

	a.openProviderDialog(providers.LLM, "deepseek")
	a.rmDlg.advanced = true
	frames(tt)
	if has(tt, "Chat Completions") || has(tt, "Search the web when rewriting") {
		t.Errorf("deepseek: %q", firstTexts(tt))
	}
	a.openProviderDialog(providers.LLM, "anthropic")
	a.rmDlg.advanced = true
	frames(tt)
	if has(tt, "Chat Completions") || !has(tt, "Search the web when rewriting") {
		t.Errorf("anthropic: %q", firstTexts(tt))
	}
}

// TestPlainOverNetwork: the dialog's "not encrypted" note is for plain
// endpoints to another machine only.
func TestPlainOverNetwork(t *testing.T) {
	for ep, want := range map[string]bool{
		"": false, "https://api.deepseek.com/v1": false, "wss://relay.example/x": false,
		"http://localhost:11434/v1": false, "http://127.0.0.1:1234/v1": false, "ws://[::1]:9/x": false,
		"http://100.88.1.2:3425/v1": true, "HTTP://gpubox.local:8000/v1": true, "ws://192.168.1.20/x": true,
	} {
		if got := plainOverNetwork(ep); got != want {
			t.Errorf("%q: %v", ep, got)
		}
	}
}
