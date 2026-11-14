package apple

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
)

// Config is what Services needs.
type Config struct {
	// Local are the downloaded local models (listed first).
	Local local.Services
	// Helper reaches the helper; nil: Apple's models are never there.
	Helper Caller
	// Name says a model's Name (uitext.ModelName); nil: its id.
	Name func(id string) string
}

// Services are the local models and Apple's. Goroutine-safe.
type Services struct {
	cfg Config
	mu  sync.Mutex
	st  Status
}

var _ local.Services = (*Services)(nil)

// New is the services of cfg; nothing of Apple's is listed before the
// first Refresh.
func New(cfg Config) *Services {
	if cfg.Local == nil {
		cfg.Local = local.NotInstalled{}
	}
	if cfg.Name == nil {
		cfg.Name = func(id string) string { return id }
	}
	return &Services{cfg: cfg}
}

// Local is the local models it wraps.
func (s *Services) Local() local.Services { return s.cfg.Local }

// Status is the last status Refresh read (memory only).
func (s *Services) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}

// SetStatus replaces it (Refresh; tests).
func (s *Services) SetStatus(st Status) {
	s.mu.Lock()
	s.st = st
	s.mu.Unlock()
}

// statusTimeout bounds apple.status.
const statusTimeout = 10 * time.Second

// Refresh asks the helper for Apple's models now. A helper without the
// method (an older one) means none; another failure keeps what was known.
func (s *Services) Refresh(ctx context.Context) (Status, error) {
	if s.cfg.Helper == nil {
		return Status{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	raw, err := s.cfg.Helper.Call(ctx, "", proto.MethodAppleStatus, proto.AppleStatusParams{Locales: Locales}, nil)
	var st Status
	switch {
	case errors.Is(err, &proto.Error{Code: proto.CodeMethodNotFound}):
		st = Status{}
	case err != nil:
		return s.Status(), err
	default:
		if err := jsonUnmarshal(raw, &st.AppleStatus); err != nil {
			return s.Status(), err
		}
		st.Known = true
	}
	s.SetStatus(st)
	return st, nil
}

// Install asks Apple to download the speech locales (proto.LocaleAvailable
// ones), reporting its progress (0…1, on the helper's reader goroutine:
// it must not block); the status is read again after it.
func (s *Services) Install(ctx context.Context, locales []string, progress func(float64)) error {
	if s.cfg.Helper == nil || len(locales) == 0 {
		return nil
	}
	_, err := s.cfg.Helper.Call(ctx, jobID(), proto.MethodAppleInstall, proto.AppleInstallParams{Locales: locales}, func(m proto.Message) {
		var p proto.AppleProgressParams
		if m.Method == proto.MethodAppleProgress && m.DecodeParams(&p) == nil && progress != nil {
			progress(p.Fraction)
		}
	})
	if _, rerr := s.Refresh(context.WithoutCancel(ctx)); err == nil {
		err = rerr
	}
	return err
}

// apples are Apple's models of kind this Mac has now.
func (s *Services) apples(kind local.Kind) []local.Model {
	st := s.Status()
	switch {
	case kind == local.ASR && st.SpeechReady():
		return []local.Model{{Kind: local.ASR, ID: SpeechID, Name: s.cfg.Name(SpeechID)}}
	case kind == local.LLM && st.IntelligenceReady():
		return []local.Model{{Kind: local.LLM, ID: IntelligenceID, Name: s.cfg.Name(IntelligenceID)}}
	}
	return nil
}

// Installed lists the downloaded local models, then Apple's. Memory only.
func (s *Services) Installed(kind local.Kind) []local.Model {
	return slices.Concat(s.cfg.Local.Installed(kind), s.apples(kind))
}

// ASR opens speech model id: Apple Speech when id names it, or when it is
// the automatic choice (no downloaded model); else a local model.
func (s *Services) ASR(ctx context.Context, id string) (asr.Provider, local.Model, error) {
	m, ok := local.Choose(s.Installed(local.ASR), id)
	if !ok {
		if id == SpeechID {
			return nil, local.Model{}, fmt.Errorf("%w: %w", local.NotInstalledError(local.ASR, id), ErrUnavailable)
		}
		return s.cfg.Local.ASR(ctx, id)
	}
	if m.ID != SpeechID {
		return s.cfg.Local.ASR(ctx, m.ID)
	}
	return &Speech{h: s.cfg.Helper}, m, nil
}

// LLM opens language model id, as ASR.
func (s *Services) LLM(ctx context.Context, id string) (local.Chat, local.Model, error) {
	m, ok := local.Choose(s.Installed(local.LLM), id)
	if !ok {
		if id == IntelligenceID {
			return nil, local.Model{}, fmt.Errorf("%w: %w", local.NotInstalledError(local.LLM, id), ErrUnavailable)
		}
		return s.cfg.Local.LLM(ctx, id)
	}
	if m.ID != IntelligenceID {
		return s.cfg.Local.LLM(ctx, m.ID)
	}
	return &llm.Client{Backend: &Chat{h: s.cfg.Helper}, Retry: llm.RetryPolicy{MaxAttempts: 1}}, m, nil
}
