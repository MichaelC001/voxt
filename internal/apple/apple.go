// Package apple is Apple's on-device models as local models
// (docs/apple-models.md): Apple Speech (macOS's speech recognition,
// on this Mac only) and Apple Intelligence (macOS 26's language model).
// macOS brings them; nothing is downloaded but, on Apple's side, a
// speech language. Both run in the helper (its apple.* methods): Speech
// is an asr.Streamer that sends the session's audio there, Chat an
// llm.Backend over its streamed answers.
//
// Services wraps the downloaded local models (local.Services) and lists
// Apple's after them when this Mac has them, as the last apple.status
// said (Refresh): automatic (local.Choose) takes a downloaded model when
// there is one, else Apple's.
//
// Imports: asr, core, helper/proto, llm, local.
package apple

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/hehehai/VoxtMini/internal/helper/proto"
)

// The models' ids (settings.Feature's ASR and LLM).
const (
	SpeechID       = "apple.speech"
	IntelligenceID = "apple.intelligence"
)

// Provider is the speech runtime's name in asr errors.
const Provider = "apple"

// Is reports whether id is one of Apple's models.
func Is(id string) bool { return id == SpeechID || id == IntelligenceID }

// Caller is how this package reaches the helper (helper.Supervisor's
// Call).
type Caller interface {
	Call(ctx context.Context, id, method string, params any, notify func(proto.Message)) (json.RawMessage, error)
}

// The locales VoxtMini asks Apple's speech recognition for: one per
// language in scope (English, Chinese (Simplified), Japanese).
var Locales = []string{"en-US", "zh-CN", "ja-JP"}

// Locale is the speech locale of a language code ("en", "zh-Hans",
// "ja-JP"…), "" for a language Apple Speech is not used for.
func Locale(lang string) string {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(lang)), "-")
	base, _, _ = strings.Cut(base, "_")
	switch base {
	case "en":
		return "en-US"
	case "zh":
		if l := strings.ToLower(lang); strings.Contains(l, "hant") || strings.HasSuffix(l, "-tw") || strings.HasSuffix(l, "-hk") {
			return "" // Traditional Chinese is out of scope
		}
		return "zh-CN"
	case "ja":
		return "ja-JP"
	}
	return ""
}

// Language is the base language of a locale ("zh-CN": "zh").
func Language(locale string) string {
	base, _, _ := strings.Cut(locale, "-")
	return base
}

// Status is what the helper last said of Apple's models; Known is false
// before it said anything (no helper yet, or an older one).
type Status struct {
	proto.AppleStatus
	Known bool
}

// SpeechReady: speech recognition with at least one language installed.
func (s Status) SpeechReady() bool {
	if !s.Known || s.Speech.Engine == "" {
		return false
	}
	for _, l := range s.Speech.Locales {
		if l.Status == proto.LocaleInstalled {
			return true
		}
	}
	return false
}

// LocaleStatus is locale's state (proto.Locale*), unsupported when the
// helper did not list it.
func (s Status) LocaleStatus(locale string) string {
	for _, l := range s.Speech.Locales {
		if l.Locale == locale {
			return l.Status
		}
	}
	return proto.LocaleUnsupported
}

// IntelligenceReady: Apple Intelligence can answer.
func (s Status) IntelligenceReady() bool {
	return s.Known && s.Intelligence.Status == proto.IntelligenceAvailable
}

// IntelligenceReason is why Apple Intelligence cannot answer
// (proto.AppleReason*), "" when it can or nothing is known.
func (s Status) IntelligenceReason() string {
	if !s.Known || s.IntelligenceReady() {
		return ""
	}
	if s.Intelligence.Reason == "" {
		return proto.AppleReasonOther
	}
	return s.Intelligence.Reason
}

// Errors the UI words; the asr and llm errors of this package wrap them.
var (
	// ErrPermission: macOS refused Speech Recognition (Settings →
	// Permissions).
	ErrPermission = errors.New("apple: speech recognition is not allowed")
	// ErrLanguage: the language is not one Apple Speech is used for, or
	// not ready on this Mac (the Models page downloads it).
	ErrLanguage = errors.New("apple: the language is not available")
	// ErrUnavailable: the model is not on this Mac now (Apple
	// Intelligence turned off, an older macOS).
	ErrUnavailable = errors.New("apple: not available on this Mac")
	// ErrRefused: Apple Intelligence's guardrails refused the request.
	ErrRefused = errors.New("apple: refused by Apple Intelligence")
)

// capability is the capability of an unsupported error's data.
func capability(pe *proto.Error) string {
	var d struct {
		Capability string `json:"capability"`
	}
	json.Unmarshal(pe.Data, &d)
	return d.Capability
}
