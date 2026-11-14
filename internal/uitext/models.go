package uitext

// The words of a local model's failure (the notices of a session and the
// Try areas): what went wrong and its detail, for the ⓘ.

import (
	"errors"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
)

// ModelErrorText says what went wrong with a local model, and its detail
// (the error itself, for the ⓘ).
func ModelErrorText(err error) (msg, more string) {
	if err == nil {
		return "", ""
	}
	more = err.Error()
	if s := appleErrorText(err); s != "" { // Apple's models: permission, language, availability
		return s, more
	}
	if remoteFailure(err) { // a remote service's key, quota or model
		return ProviderErrorText(err)
	}
	if errors.Is(err, local.ErrNotInstalled) {
		return i18n.T("The model is not installed."), more
	}
	if errors.Is(err, local.ErrNoRuntime) {
		return i18n.T("This version of VoxtMini cannot run the model yet."), more
	}
	var ae *asr.Error
	if errors.As(err, &ae) {
		switch ae.Kind {
		case asr.KindNetwork, asr.KindServer:
			return i18n.T("The speech model stopped or did not answer. Try again in a moment."), more
		case asr.KindTimeout:
			return i18n.T("The speech model took too long."), more
		case asr.KindAudio:
			return i18n.T("The speech model did not accept the audio."), more
		case asr.KindCancelled:
			return i18n.T("Cancelled."), more
		case asr.KindConfig:
			return i18n.T("The speech model is not set up correctly: check it on the Models page."), more
		}
		return i18n.T("The speech model answered something VoxtMini does not understand."), more
	}
	var le *llm.Error
	if errors.As(err, &le) {
		switch le.Code {
		case llm.CodeTimeout:
			return i18n.T("The language model did not answer in time."), more
		case llm.CodeNetwork, llm.CodeServer, llm.CodeOverloaded, llm.CodeRateLimited:
			return i18n.T("The language model stopped or did not answer. Try again in a moment."), more
		case llm.CodeContextLength:
			return i18n.T("The text is too long for the language model."), more
		case llm.CodeCancelled:
			return i18n.T("Cancelled."), more
		}
		return i18n.T("The language model answered something VoxtMini does not understand."), more
	}
	return i18n.T("The model stopped or did not answer. Try again in a moment."), more
}

// remoteFailure says whether err is one only a remote service gives (a
// key refused or missing, a quota, an unknown model): the Remote models
// page's words say it better than a local model's.
func remoteFailure(err error) bool {
	var ae *asr.Error
	if errors.As(err, &ae) {
		return ae.Kind == asr.KindAuth || ae.Kind == asr.KindQuota
	}
	var le *llm.Error
	if errors.As(err, &le) {
		return le.Code == llm.CodeNoKey || le.Code == llm.CodeAuth || le.Code == llm.CodeModelNotFound
	}
	return false
}
