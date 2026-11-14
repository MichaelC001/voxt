package llm

import (
	"context"
	"net/http"
)

// Transports: how a backend talks to its service (Preset.Transport,
// BackendInfo.Transport).
const (
	TransportOpenAI    = "openai"    // OpenAI-compatible chat completions
	TransportAnthropic = "anthropic" // the Anthropic Messages API
	TransportResponses = "responses" // the OpenAI Responses API (ProviderConfig.API "responses")
	TransportCodex     = "codex"     // the Responses API of a ChatGPT account (Codex's backend)
)

// NewBackend builds the HTTP backend of a remote service (remote models,
// docs/remote-models.md): its preset's transport, or the Responses API
// when cfg.API asks for it, over client (the family's proxy) with the key
// from secrets (cfg.Auth's headers for a ChatGPT account). Web search is
// on when cfg.WebSearch is and the service supports it on that transport
// (SearchSupported); it then applies to SkillRewrite requests only.
func NewBackend(cfg ProviderConfig, client *http.Client, secrets Secrets) (Backend, error) {
	p, err := cfg.Resolve()
	if err != nil {
		return nil, err
	}
	search := cfg.WebSearch && SearchSupported(p, "")
	switch p.Transport {
	case TransportOpenAI:
		b := NewOpenAI(p, client, secrets)
		b.search = search
		return b, nil
	case TransportAnthropic:
		b := NewAnthropic(p, client, secrets)
		b.search = search
		return b, nil
	case TransportResponses:
		b := NewResponses(p, client, secrets)
		b.search = search
		return b, nil
	case TransportCodex:
		b := NewCodex(p, client, cfg.Auth)
		b.search = search
		return b, nil
	}
	return nil, newErr(CodeConfig, "provider", p.ID, "field", "transport")
}

// HeaderSource supplies a request's auth headers when a service signs in
// with an account instead of a key (Preset.Auth "oauth": a ChatGPT
// account for Codex). The app's sign-in makes one; Headers may refresh a
// token and block (it is called off the UI thread, per request). Its
// error becomes CodeSignIn.
type HeaderSource interface {
	Headers(ctx context.Context) (map[string]string, error)
}

// HeaderFunc adapts a function to HeaderSource.
type HeaderFunc func(ctx context.Context) (map[string]string, error)

func (f HeaderFunc) Headers(ctx context.Context) (map[string]string, error) { return f(ctx) }

// ModelInfo is one entry of a remote service's model list.
type ModelInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"` // a display name, when the service gives one
	OwnedBy       string `json:"ownedBy,omitempty"`
	ContextLength int    `json:"contextLength,omitempty"`
}

// ModelLister is a backend that lists its service's models (every remote
// backend NewBackend returns).
type ModelLister interface {
	Models(ctx context.Context) ([]ModelInfo, error)
}
