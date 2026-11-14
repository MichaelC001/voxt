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
)

// NewBackend builds the HTTP backend of a remote service (remote models,
// docs/remote-models.md): its preset's transport, or the Responses API
// when cfg.API asks for it, over client (the family's proxy) with the key
// from secrets. Web search is
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
	}
	return nil, newErr(CodeConfig, "provider", p.ID, "field", "transport")
}

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
