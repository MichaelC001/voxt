package settings

import (
	"slices"
	"time"
)

// Models: how the local models' runtimes keep what they loaded (the
// Models page's Advanced section), and the models the user hid.
type Models struct {
	// KeepLoaded is how long a loaded model stays in memory unused: one
	// of KeepChoices. "" (Automatic), or a value VoxtMini does not know,
	// keeps each runtime's own idle time (speech 10 min, language 5 min),
	// so older documents need no migration (10-17). The ActNow family's
	// memory rules stay above it: another app's memory request and the
	// memory budget still unload idle models (internal/actnow).
	KeepLoaded string `json:"keepLoaded,omitempty"`
	// Hidden are the models and remote services hidden on the Models page
	// and in the features' model selects (sorted, no repeats): a local
	// model's catalogue id ("asr.accurate"), a remote service's
	// HiddenRemote key. Hiding deletes nothing and changes no feature's
	// choice (docs/models-page.md).
	Hidden []string `json:"hidden,omitempty"`
	// Params are the local models' settings the user set (each model's
	// Settings dialog), by catalogue id ("llm.balanced", "asr.whisper-
	// turbo", "apple.intelligence"); a model not listed runs with
	// VoxtMini's tuned ones (model_params.go).
	Params map[string]ModelParams `json:"modelParams,omitempty"`
	// LegacyLLMParams are the language models' parameters as Models →
	// Advanced saved them (2026-11-14, before the Settings dialog): read
	// only, normalize moves them into Params.
	LegacyLLMParams map[string]ModelParams `json:"llmParams,omitempty"`
}

// HiddenRemote is the Hidden key of remote service id of kind ("asr",
// "llm"): "remote:asr.openai".
func HiddenRemote(kind, id string) string { return "remote:" + kind + "." + id }

// IsHidden says whether key is hidden.
func (m Models) IsHidden(key string) bool { return slices.Contains(m.Hidden, key) }

// SetHidden is m.Hidden with key hidden (on) or shown, a new slice.
func (m Models) SetHidden(key string, on bool) []string {
	out := slices.DeleteFunc(slices.Clone(m.Hidden), func(k string) bool { return k == key })
	if on {
		out = append(out, key)
		slices.Sort(out)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// The choices of Models.KeepLoaded.
const (
	KeepAuto      = ""
	Keep5Min      = "5m"
	Keep15Min     = "15m"
	Keep30Min     = "30m"
	Keep1Hour     = "1h"
	KeepUntilQuit = "quit"
)

// KeepChoices are Models.KeepLoaded's choices in the order the UI lists
// them.
var KeepChoices = []string{KeepAuto, Keep5Min, Keep15Min, Keep30Min, Keep1Hour, KeepUntilQuit}

// KeepFor is the idle time KeepLoaded sets for every runtime: auto when
// it leaves each runtime its own; a negative d keeps models until VoxtMini
// quits.
func (m Models) KeepFor() (d time.Duration, auto bool) {
	switch m.KeepLoaded {
	case Keep5Min:
		return 5 * time.Minute, false
	case Keep15Min:
		return 15 * time.Minute, false
	case Keep30Min:
		return 30 * time.Minute, false
	case Keep1Hour:
		return time.Hour, false
	case KeepUntilQuit:
		return -1, false
	}
	return 0, true
}
