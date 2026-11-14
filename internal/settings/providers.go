package settings

import (
	"slices"
	"strings"
)

// Providers lists the remote speech and language services the user set
// up on the Remote models page (docs/remote-models.md), in the order they
// were added. A preset nobody set up has no entry; one entry per ID.
// Empty in older documents: nothing remote until the user sets it up.
type Providers struct {
	ASR []Provider `json:"asr,omitempty"`
	LLM []Provider `json:"llm,omitempty"`
}

// Provider is one remote service the user set up. Its key is in the
// Keychain (internal/secrets: "asr.<ID>", "llm.<ID>"), never here. ""
// and zero fields use the preset's defaults (internal/providers).
type Provider struct {
	// ID is unique within its list and names the Keychain item; for a
	// preset it is the preset's id ("openai", "doubao", "deepseek"…).
	ID     string `json:"id"`
	Preset string `json:"preset,omitempty"` // "" = ID
	Name   string `json:"name,omitempty"`   // the user's label ("" = the preset's name)
	// Enabled offers it to the features; off, it keeps its setup and key.
	Enabled bool `json:"enabled"`
	// Endpoint: a speech service's full URL (wss://…, https://…/audio/
	// transcriptions), a language model's API root (https://…/v1).
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	// AppID is Doubao's App ID (its Access Token is the key).
	AppID string `json:"appID,omitempty"`

	// Speech services' advanced parameters.
	//
	// Prompt is the recognition prompt (context, spelling), for services
	// that take one; the dictionary's terms are added to it unless
	// NoHints. Language is the language hint ("" = the main language, as
	// the local models; "auto" = none: the service detects).
	Prompt   string `json:"prompt,omitempty"`
	NoHints  bool   `json:"noHints,omitempty"`
	Language string `json:"language,omitempty"`

	// Language models' advanced parameters (nil / 0 / "": VoxtMini's
	// per-skill defaults and the preset's body).
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
	MaxTokens   int      `json:"maxTokens,omitempty"`
	// ExtraBody is a JSON object merged into every request's body over
	// the preset's (`{"thinking":{"type":"enabled"}}`).
	ExtraBody string `json:"extraBody,omitempty"`
	// API is how a language model is asked, for a service that offers
	// both: "" its usual chat completions, "responses" OpenAI's Responses
	// API. WebSearch lets it search the web while rewriting (services
	// that host a search tool; never for dictation or translation).
	API       string `json:"api,omitempty"`
	WebSearch bool   `json:"webSearch,omitempty"`

	// CredentialRef names the ActNow family's key the service uses
	// instead of a key of its own ("actnow:vendor.deepseek"; family spec
	// §8): the family's Keychain item, read only once the user chose "Use
	// this key" in this app. "" = its own Keychain item.
	CredentialRef string `json:"credentialRef,omitempty"`
}

// RemotePrefix starts a Feature's ASR or LLM when it names a remote
// service: "remote:<Provider.ID>".
const RemotePrefix = "remote:"

// RemoteID is the Feature value of remote service id.
func RemoteID(id string) string { return RemotePrefix + id }

// IsRemote says whether a Feature's ASR or LLM names a remote service,
// and which.
func IsRemote(v string) (id string, ok bool) {
	return strings.CutPrefix(strings.TrimSpace(v), RemotePrefix)
}

func cloneProviders(p Providers) Providers {
	cp := func(l []Provider) []Provider {
		out := slices.Clone(l)
		for i := range out {
			if v := out[i].Temperature; v != nil {
				x := *v
				out[i].Temperature = &x
			}
			if v := out[i].TopP; v != nil {
				x := *v
				out[i].TopP = &x
			}
		}
		return out
	}
	return Providers{ASR: cp(p.ASR), LLM: cp(p.LLM)}
}
