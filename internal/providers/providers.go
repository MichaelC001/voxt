// Package providers joins the user's setup of the remote speech and
// language services (settings.Providers) to the code that talks to them
// (internal/asr's services, internal/llm's transports): the presets the
// Remote models page offers, which setups are ready, building a client
// from a setup and its key, and the connection test. Taken from
// Auralink's, with Voxt's other services (GLM-ASR, Xiaomi MiMo ASR) and
// advanced parameters (docs/remote-models.md).
//
// It never reads the Keychain itself: callers pass keys in (read off the
// main thread with internal/secrets) and a hasKey function for the
// decisions that only need to know whether one is saved.
//
// Imports: asr (+ openai, doubao, aliyun, gemini, mimo), llm, settings.
package providers

import (
	"slices"

	"github.com/hehehai/VoxtMini/internal/asr/aliyun"
	"github.com/hehehai/VoxtMini/internal/asr/doubao"
	"github.com/hehehai/VoxtMini/internal/asr/gemini"
	"github.com/hehehai/VoxtMini/internal/asr/mimo"
	"github.com/hehehai/VoxtMini/internal/asr/openai"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// Kind is a list of services: speech recognition or language models.
type Kind string

const (
	ASR Kind = "asr"
	LLM Kind = "llm"
)

// Modes of an ASR preset.
const (
	ModeStream = "stream" // recognises while the user speaks
	ModeFile   = "file"   // uploads the recording when it stops
)

// Preset is a service the Remote models page offers, with its defaults.
type Preset struct {
	Kind Kind
	ID   string
	Name string // brand name, not translated
	// Mode: ASR ModeStream or ModeFile; LLM the transport ("openai",
	// "anthropic").
	Mode     string
	Endpoint string   // the default endpoint ("" for custom: the user's)
	Model    string   // the default model
	Models   []string // suggested models (any id may be typed)
	// AppID: the service has an App ID besides its key (Doubao's older
	// console: App ID + Access Token; its newer one gives one API key, so
	// the App ID may stay empty).
	AppID bool
	// Prompt: the service takes an ASR prompt. Hints: it takes the
	// dictionary's terms as hot words (else they go into the prompt, or
	// nowhere).
	Prompt, Hints bool
	// KeyOptional: it works without a key (a custom server).
	KeyOptional bool
	// SignIn: it signs in with an account instead of a key (Codex: a
	// ChatGPT account). Its sign-in is kept as its key (the Keychain
	// item of Account), so "has a key" is "signed in".
	SignIn bool
	// Responses: a language service that also offers the Responses API
	// (settings.Provider.API).
	Responses bool
	// Custom: the user names it and must give an endpoint.
	Custom bool
	// KeyURL is where to get a key, Docs the API's documentation.
	KeyURL, Docs string
}

// asrPresets are the speech services (Voxt's RemoteASRProvider), in the
// page's order: the streaming ones first, as they
// show text while you speak.
var asrPresets = []Preset{
	{Kind: ASR, ID: doubao.ID, Name: "Doubao (Volcengine)", Mode: ModeStream, Endpoint: doubao.DefaultEndpoint,
		Model: doubao.DefaultModel, Models: doubao.Models, AppID: true, Hints: true,
		KeyURL: "https://console.volcengine.com/speech/app", Docs: "https://www.volcengine.com/docs/6561/1354869"},
	{Kind: ASR, ID: aliyun.ID, Name: "Alibaba Cloud Model Studio", Mode: ModeStream, Endpoint: aliyun.InferenceEndpoint,
		Model: aliyun.DefaultModel, Models: aliyun.Models, Hints: true,
		KeyURL: "https://bailian.console.aliyun.com/?tab=model#/api-key", Docs: "https://help.aliyun.com/zh/model-studio/real-time-speech-recognition"},
	{Kind: ASR, ID: gemini.ID, Name: "Google Gemini", Mode: ModeStream, Endpoint: gemini.DefaultEndpoint,
		Model: gemini.DefaultModel, Models: gemini.Models, Hints: true,
		KeyURL: "https://aistudio.google.com/apikey", Docs: "https://ai.google.dev/gemini-api/docs/live"},
	{Kind: ASR, ID: openai.ID, Name: "OpenAI Transcribe", Mode: ModeFile, Endpoint: openai.DefaultEndpoint,
		Model: openai.DefaultModel, Models: openai.Models, Prompt: true,
		KeyURL: "https://platform.openai.com/api-keys", Docs: "https://platform.openai.com/docs/api-reference/audio/createTranscription"},
	// StepFun's and Z.ai's transcriptions are OpenAI-compatible
	// (multipart, Bearer): the openai client with their endpoint and model.
	{Kind: ASR, ID: StepFunASR, Name: "StepFun", Mode: ModeFile, Endpoint: stepFunEndpoint,
		Model: "step-asr", Models: []string{"step-asr", "step-asr-1.1"},
		KeyURL: "https://platform.stepfun.com/interface-key", Docs: "https://platform.stepfun.com/docs/"},
	{Kind: ASR, ID: GLMASR, Name: "Z.ai GLM-ASR", Mode: ModeFile, Endpoint: glmEndpoint,
		Model: "glm-asr-2512", Models: []string{"glm-asr-2512", "glm-asr-1"}, Prompt: true,
		KeyURL: "https://bigmodel.cn/usercenter/proj-mgmt/apikeys", Docs: "https://docs.bigmodel.cn/"},
	{Kind: ASR, ID: mimo.ID, Name: "Xiaomi MiMo", Mode: ModeFile, Endpoint: mimo.DefaultEndpoint,
		Model: mimo.DefaultModel, Models: mimo.Models,
		KeyURL: "https://platform.xiaomimimo.com/", Docs: "https://platform.xiaomimimo.com/"},
}

// StepFunASR and GLMASR are StepFun's and Z.ai's speech services:
// OpenAI-compatible uploads.
const (
	StepFunASR      = "stepfun"
	stepFunEndpoint = "https://api.stepfun.com/v1/audio/transcriptions"
	GLMASR          = "glm"
	glmEndpoint     = "https://open.bigmodel.cn/api/paas/v4/audio/transcriptions"
)

// Presets returns kind's presets in the page's order (a copy).
func Presets(kind Kind) []Preset {
	if kind == ASR {
		return slices.Clone(asrPresets)
	}
	var out []Preset
	for _, p := range llm.Presets() {
		out = append(out, Preset{Kind: LLM, ID: p.ID, Name: p.Name, Mode: p.Transport, Endpoint: p.BaseURL,
			Model: p.DefaultModel, Models: p.Models, KeyOptional: p.KeyOptional, Custom: p.BaseURL == "",
			SignIn: p.Auth == llm.AuthOAuth, Responses: p.Responses,
			KeyURL: p.Docs, Docs: p.Docs})
	}
	return out
}

// LookupPreset finds kind's preset id.
func LookupPreset(kind Kind, id string) (Preset, bool) {
	for _, p := range Presets(kind) {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Account is the Keychain account of a provider's key: "asr.<id>",
// "llm.<id>" (internal/secrets).
func Account(kind Kind, id string) string { return string(kind) + "." + id }

// list is s's setups of kind.
func list(s settings.Settings, kind Kind) []settings.Provider {
	if kind == ASR {
		return s.Providers.ASR
	}
	return s.Providers.LLM
}

// Setup is the user's setup of provider id, and whether there is one.
func Setup(s settings.Settings, kind Kind, id string) (settings.Provider, bool) {
	for _, p := range list(s, kind) {
		if p.ID == id {
			return p, true
		}
	}
	return settings.Provider{ID: id}, false
}

// PresetOf is the preset of a setup (its Preset, else its ID).
func PresetOf(kind Kind, p settings.Provider) (Preset, bool) {
	id := p.Preset
	if id == "" {
		id = p.ID
	}
	return LookupPreset(kind, id)
}

// Put sets p in s's kind list, replacing the setup with its ID or
// appending it.
func Put(s *settings.Settings, kind Kind, p settings.Provider) {
	l := &s.Providers.LLM
	if kind == ASR {
		l = &s.Providers.ASR
	}
	if i := slices.IndexFunc(*l, func(x settings.Provider) bool { return x.ID == p.ID }); i >= 0 {
		(*l)[i] = p
		return
	}
	*l = append(*l, p)
}

// Name is how the page names a setup: the user's label, else its preset's.
func Name(kind Kind, p settings.Provider) string {
	if p.Name != "" {
		return p.Name
	}
	if pr, ok := PresetOf(kind, p); ok {
		return pr.Name
	}
	return p.ID
}

// Model is the model a setup uses: its own, else its preset's default.
func Model(kind Kind, p settings.Provider) string {
	if p.Model != "" {
		return p.Model
	}
	if pr, ok := PresetOf(kind, p); ok {
		return pr.Model
	}
	return ""
}

// Missing says what a setup lacks before it can be used: "" when it is
// ready; else MissingSetup (never set up), MissingOff, MissingKey or
// MissingEndpoint.
func Missing(kind Kind, p settings.Provider, configured, hasKey bool) string {
	pr, ok := PresetOf(kind, p)
	switch {
	case !configured || !ok:
		return MissingSetup
	case !p.Enabled:
		return MissingOff
	case pr.Custom && p.Endpoint == "":
		return MissingEndpoint
	case !hasKey && !pr.KeyOptional:
		return MissingKey
	}
	return ""
}

// What a setup can lack (Missing).
const (
	MissingSetup    = "setup"
	MissingOff      = "off"
	MissingKey      = "key"
	MissingEndpoint = "endpoint"
)

// Ready lists kind's setups that can be used now, in the page's order:
// the presets' order, then custom setups in the order they were added.
func Ready(s settings.Settings, kind Kind, hasKey func(account string) bool) []settings.Provider {
	var out []settings.Provider
	setups := slices.Clone(list(s, kind))
	order := func(p settings.Provider) int {
		pr, _ := PresetOf(kind, p)
		for i, x := range Presets(kind) {
			if x.ID == pr.ID {
				return i
			}
		}
		return 1 << 20
	}
	slices.SortStableFunc(setups, func(a, b settings.Provider) int { return order(a) - order(b) })
	for _, p := range setups {
		if Missing(kind, p, true, hasKey(Account(kind, p.ID))) == "" {
			out = append(out, p)
		}
	}
	return out
}

// Lookup is the setup id of kind when it is ready (Missing is ""):
// the remote service a feature chose, settings.IsRemote.
func Lookup(s settings.Settings, kind Kind, id string, hasKey func(account string) bool) (settings.Provider, bool) {
	for _, p := range Ready(s, kind, hasKey) {
		if p.ID == id {
			return p, true
		}
	}
	return settings.Provider{}, false
}
