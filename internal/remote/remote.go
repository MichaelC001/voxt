// Package remote puts the remote speech and language services the user
// set up (settings.Providers, internal/providers) beside the local models,
// behind the same seam: Services is a local.Services that lists the
// local models, then each ready remote service, and opens either
// (docs/remote-models.md). Sessions and pages keep asking local.Services.
//
// A remote service is never chosen automatically: its local.Model is
// Remote, which local.Choose skips for "" (automatic), so audio and text
// leave the Mac only for a feature whose model the user picked as a
// remote one (settings.RemoteID).
//
// Installed reads memory only (the settings' snapshot and the keys
// already read, Keys); ASR and LLM may block (the Keychain, the network):
// off the main thread.
//
// Imports: asr, llm, local, providers, settings.
package remote

import (
	"context"
	"net/http"
	"slices"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// Config is what Services needs.
type Config struct {
	// Local are the local models (the first in every list).
	Local local.Services
	// Settings is the settings' current snapshot (memory only).
	Settings func() settings.Settings
	// Keys are the services' keys (the Keychain, read once).
	Keys *Keys
	// Proxy is how requests reach the network now: the family's
	// network.proxy rule (nil, or a nil rule: the environment's). Called
	// for each open, off the main thread.
	Proxy func() asr.ProxyFunc
	// HTTP replaces the clients built on the proxy (tests).
	HTTP *http.Client
	// SignIn is the request headers of a service that signs in with an
	// account (providers.Preset.SignIn: ChatGPT for Codex), by its
	// Keychain account; nil: such services cannot be opened.
	SignIn func(account string) llm.HeaderSource
}

// Services is the local models and the ready remote services.
// Goroutine-safe.
type Services struct {
	cfg Config
}

var _ local.Services = (*Services)(nil)

// New is the services of cfg.
func New(cfg Config) *Services {
	if cfg.Local == nil {
		cfg.Local = local.NotInstalled{}
	}
	if cfg.Keys == nil {
		cfg.Keys = NewKeys(nil)
	}
	return &Services{cfg: cfg}
}

// Local is the local models it wraps.
func (s *Services) Local() local.Services { return s.cfg.Local }

// Keys are its keys.
func (s *Services) Keys() *Keys { return s.cfg.Keys }

func (s *Services) env(key string) providers.Env {
	var proxy asr.ProxyFunc
	if s.cfg.Proxy != nil {
		proxy = s.cfg.Proxy()
	}
	return providers.Env{Key: key, Proxy: proxy, HTTP: s.cfg.HTTP}
}

// kindOf maps local's kinds to providers'.
func kindOf(k local.Kind) providers.Kind {
	if k == local.ASR {
		return providers.ASR
	}
	return providers.LLM
}

// Ready are kind's ready remote services as models, in the Remote models
// page's order. Memory only.
func (s *Services) Ready(kind local.Kind) []local.Model {
	if s.cfg.Settings == nil {
		return nil
	}
	pk := kindOf(kind)
	var out []local.Model
	for _, p := range providers.Ready(s.cfg.Settings(), pk, s.cfg.Keys.Has) {
		out = append(out, Model(kind, p))
	}
	return out
}

// Model is a remote service as a feature chooses it.
func Model(kind local.Kind, p settings.Provider) local.Model {
	pk := kindOf(kind)
	return local.Model{Kind: kind, ID: settings.RemoteID(p.ID), Name: providers.Name(pk, p), Detail: providers.Model(pk, p), Remote: true}
}

// Installed lists the local models, then the ready remote services.
func (s *Services) Installed(kind local.Kind) []local.Model {
	return slices.Concat(s.cfg.Local.Installed(kind), s.Ready(kind))
}

// setup is the ready setup a remote id names.
func (s *Services) setup(kind local.Kind, id string) (settings.Provider, bool) {
	pid, ok := settings.IsRemote(id)
	if !ok || s.cfg.Settings == nil {
		return settings.Provider{}, false
	}
	return providers.Lookup(s.cfg.Settings(), kindOf(kind), pid, s.cfg.Keys.Has)
}

// ASR opens speech model id: a ready remote service, else a local model
// (a remote id that is not ready, its key or setup gone, is automatic:
// the recommended local model).
func (s *Services) ASR(ctx context.Context, id string) (asr.Provider, local.Model, error) {
	if _, remote := settings.IsRemote(id); !remote {
		return s.cfg.Local.ASR(ctx, id)
	}
	p, ok := s.setup(local.ASR, id)
	if !ok {
		return s.cfg.Local.ASR(ctx, "")
	}
	key, err := s.cfg.Keys.Get(providers.Account(providers.ASR, p.ID))
	if err != nil {
		return nil, local.Model{}, err
	}
	c, err := providers.NewASR(p, s.env(key))
	if err != nil {
		return nil, local.Model{}, err
	}
	return c, Model(local.ASR, p), nil
}

// LLM opens language model id, as ASR.
func (s *Services) LLM(ctx context.Context, id string) (local.Chat, local.Model, error) {
	if _, remote := settings.IsRemote(id); !remote {
		return s.cfg.Local.LLM(ctx, id)
	}
	p, ok := s.setup(local.LLM, id)
	if !ok {
		return s.cfg.Local.LLM(ctx, "")
	}
	acc := providers.Account(providers.LLM, p.ID)
	var env providers.Env
	if pr, _ := providers.PresetOf(providers.LLM, p); pr.SignIn {
		env = s.env("") // the sign-in's headers, not a key
		if s.cfg.SignIn != nil {
			env.Auth = s.cfg.SignIn(acc)
		}
	} else {
		key, err := s.cfg.Keys.Get(acc)
		if err != nil {
			return nil, local.Model{}, err
		}
		env = s.env(key)
	}
	b, err := providers.NewLLM(p, env)
	if err != nil {
		return nil, local.Model{}, err
	}
	return chat{c: &llm.Client{Backend: b, Retry: llm.RetryPolicy{MaxAttempts: 2}}, p: p}, Model(local.LLM, p), nil
}

// chat is a remote language model with its setup's parameters.
type chat struct {
	c *llm.Client
	p settings.Provider
}

func (c chat) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	providers.ChatParams(c.p, &req)
	return c.c.Chat(ctx, req)
}
