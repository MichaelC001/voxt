package uitext

// The words of Apple's on-device models (internal/apple): their names,
// their state on this Mac and their failures.

import (
	"errors"

	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/i18n"
)

// appleName is the name of Apple's model id ("" for another id).
func appleName(id string) string {
	switch id {
	case apple.SpeechID:
		return i18n.T("Apple Speech")
	case apple.IntelligenceID:
		return i18n.T("Apple Intelligence")
	}
	return ""
}

// appleAbout says what Apple's model id is.
func appleAbout(id string) string {
	switch id {
	case apple.SpeechID:
		return i18n.T("macOS's own speech recognition, on this Mac only. Nothing to download but a language; less accurate than the recommended models, and it knows one language per recording.")
	case apple.IntelligenceID:
		return i18n.T("macOS's own language model, on this Mac. Nothing to download; it cleans up, translates and rewrites short texts.")
	}
	return ""
}

// AppleLanguages are the languages VoxtMini uses Apple's models for.
func AppleLanguages() string { return i18n.T("English, Chinese (Simplified) and Japanese") }

// AppleLocaleName is a speech locale's language.
func AppleLocaleName(locale string) string {
	switch apple.Language(locale) {
	case "en":
		return i18n.T("English")
	case "zh":
		return i18n.T("Chinese (Simplified)")
	case "ja":
		return i18n.T("Japanese")
	}
	return locale
}

// AppleLocaleState says a speech locale's state (proto.Locale*), and
// whether it is ready.
func AppleLocaleState(status string) (string, bool) {
	switch status {
	case proto.LocaleInstalled:
		return i18n.T("Ready"), true
	case proto.LocaleAvailable:
		return i18n.T("Not downloaded"), false
	case proto.LocaleDownloading:
		return i18n.T("Downloading…"), false
	}
	return i18n.T("Not supported on this Mac"), false
}

// AppleReasonText says why an Apple model is not on this Mac
// (proto.AppleReason*).
func AppleReasonText(reason string) string {
	switch reason {
	case proto.AppleReasonNotEnabled:
		return i18n.T("Turn on Apple Intelligence in System Settings to use it.")
	case proto.AppleReasonDeviceNotEligible:
		return i18n.T("This Mac (or its region) cannot run Apple Intelligence.")
	case proto.AppleReasonModelNotReady:
		return i18n.T("Apple Intelligence is still preparing its model. Try again shortly.")
	case proto.AppleReasonUnsupportedOS:
		return i18n.T("Needs a newer macOS.")
	}
	return i18n.T("Not available on this Mac now.")
}

// AppleLanguagesReady is the toast after Apple downloaded languages.
func AppleLanguagesReady() string { return i18n.T("Apple Speech's languages are ready.") }

// appleErrorText says what went wrong with one of Apple's models ("" for
// another error).
func appleErrorText(err error) string {
	switch {
	case errors.Is(err, apple.ErrPermission):
		return i18n.T("VoxtMini may not use speech recognition: allow it in Settings → Permissions.")
	case errors.Is(err, apple.ErrLanguage):
		return i18n.T("Apple Speech does not have this language on this Mac: download it on the Models page, or choose another speech model.")
	case errors.Is(err, apple.ErrRefused):
		return i18n.T("Apple Intelligence refused to answer this text.")
	case errors.Is(err, apple.ErrUnavailable):
		return i18n.T("The Apple model is not available on this Mac now.")
	}
	return ""
}
