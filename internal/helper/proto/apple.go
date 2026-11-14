package proto

// ---- apple.* ----
//
// Apple's on-device models, which macOS brings (docs/apple-models.md):
// speech recognition (SpeechAnalyzer on macOS 26, SFSpeechRecognizer
// on-device before it) and Apple Intelligence (FoundationModels, macOS
// 26). Go picks the locale, the hints, the token budget and the
// temperature; the helper runs them.

// AppleStatusParams names the locales whose state the app wants
// ("en-US", "zh-CN", "ja-JP").
type AppleStatusParams struct {
	Locales []string `json:"locales"`
}

// AppleStatus answers apple.status.
type AppleStatus struct {
	Speech       AppleSpeechStatus       `json:"speech"`
	Intelligence AppleIntelligenceStatus `json:"intelligence"`
}

// AppleSpeechStatus is speech recognition on this Mac. Engine is
// AppleEngine*, "" when there is none (Reason says why).
type AppleSpeechStatus struct {
	Engine  string        `json:"engine,omitempty"`
	Reason  string        `json:"reason,omitempty"` // AppleReason*
	Locales []AppleLocale `json:"locales"`
}

// AppleLocale is one locale's state for speech recognition.
type AppleLocale struct {
	Locale string `json:"locale"`
	Status string `json:"status"` // LocaleInstalled | LocaleAvailable | LocaleDownloading | LocaleUnsupported
}

// AppleIntelligenceStatus is Apple Intelligence on this Mac.
type AppleIntelligenceStatus struct {
	Status    string   `json:"status"`              // IntelligenceAvailable | IntelligenceUnavailable
	Reason    string   `json:"reason,omitempty"`    // AppleReason*, when unavailable
	Languages []string `json:"languages,omitempty"` // the base codes it supports: "en", "zh", "ja"…
}

// Speech engines.
const (
	AppleEngineAnalyzer   = "analyzer"   // SpeechAnalyzer + SpeechTranscriber (macOS 26)
	AppleEngineRecognizer = "recognizer" // SFSpeechRecognizer, on-device only
)

// Locale states.
const (
	LocaleInstalled   = "installed"   // ready on this Mac
	LocaleAvailable   = "available"   // Apple can download it (apple.install)
	LocaleDownloading = "downloading" // being downloaded
	LocaleUnsupported = "unsupported" // not on this Mac (or not on-device)
)

// Apple Intelligence states and the reasons it is not there.
const (
	IntelligenceAvailable   = "available"
	IntelligenceUnavailable = "unavailable"

	AppleReasonNotEnabled        = "notEnabled"        // Apple Intelligence is off in System Settings
	AppleReasonDeviceNotEligible = "deviceNotEligible" // this Mac (or region) cannot run it
	AppleReasonModelNotReady     = "modelNotReady"     // the model is still downloading or preparing
	AppleReasonUnsupportedOS     = "unsupportedOS"     // macOS too old, or a helper built without its SDK
	AppleReasonOther             = "other"
)

// AppleInstallParams asks Apple to download speech locales; a job.
type AppleInstallParams struct {
	Locales []string `json:"locales"`
}

// AppleInstallResult is the locales' states after the download.
type AppleInstallResult struct {
	Locales []AppleLocale `json:"locales"`
}

// AppleProgressParams is an apple.install job's progress (0…1).
type AppleProgressParams struct {
	Job      string  `json:"job"`
	Fraction float64 `json:"fraction"`
}

// AppleSpeechParams starts recognising speech (a job): the audio comes
// in apple.speech.audio requests, apple.speech.finish ends it.
type AppleSpeechParams struct {
	Locale string   `json:"locale"`          // "en-US", "zh-CN", "ja-JP"
	Hints  []string `json:"hints,omitempty"` // contextual strings: the dictionary's terms
}

// AppleSpeechResult is the final text ("" when no speech was heard).
type AppleSpeechResult struct {
	Text    string `json:"text"`
	Locale  string `json:"locale"`  // the locale the recogniser used
	Engine  string `json:"engine"`  // AppleEngine*
	AudioMs int    `json:"audioMs"` // the audio it got
}

// AppleSpeechStartedParams: the recogniser runs and takes audio.
type AppleSpeechStartedParams struct {
	Job    string `json:"job"`
	Locale string `json:"locale"`
	Engine string `json:"engine"`
}

// ApplePartialParams is the text so far (the whole hypothesis).
type ApplePartialParams struct {
	Job  string `json:"job"`
	Text string `json:"text"`
}

// AppleAudioParams is a piece of an apple.speech job's audio: base64 of
// mono PCM16 LE at 16 kHz.
type AppleAudioParams struct {
	Job  string `json:"job"`
	Data []byte `json:"data"`
}

// AppleJobParams names an apple.speech job (apple.speech.finish).
type AppleJobParams struct {
	Job string `json:"job"`
}

// AppleChatParams is one answer of Apple Intelligence (a job): the
// system message is the instructions, the others the conversation, the
// last user message the prompt.
type AppleChatParams struct {
	Messages    []AppleMessage `json:"messages"`
	MaxTokens   int            `json:"maxTokens,omitempty"`   // 0: the model's own limit
	Temperature *float64       `json:"temperature,omitempty"` // nil: the model's default; 0: greedy
}

// AppleMessage is a chat turn: role "system", "user" or "assistant".
type AppleMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AppleChatResult is the whole answer.
type AppleChatResult struct {
	Text   string `json:"text"`
	Finish string `json:"finish"` // "stop" | "length"
}

// AppleDeltaParams is the next piece of an apple.chat answer.
type AppleDeltaParams struct {
	Job  string `json:"job"`
	Text string `json:"text"`
}

// Capabilities in unsupported's data.
const (
	CapabilitySpeech       = "speech"
	CapabilityIntelligence = "intelligence"
	CapabilityLocale       = "locale"
)
