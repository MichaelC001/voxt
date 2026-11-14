package omni

import (
	stdmaps "maps"

	"github.com/hehehai/VoxtMini/internal/knobs"
)

// Options are the user's settings of a speech model (settings.
// ModelParams, the model's Settings dialog): only what its family
// takes (Spec.Knobs).
type Options struct {
	// DetectLanguage: no language hint, the model detects the language.
	DetectLanguage bool
	// Temperature replaces ASRTuning.Temperature (Whisper; nil: the
	// spec's).
	Temperature *float64
}

// SetOptions replaces the models' options, by model id; they apply to
// the next request (a server need not start again).
func (rt *Runtime) SetOptions(o map[string]Options) {
	c := stdmaps.Clone(o)
	rt.opts.Store(&c)
}

// Options are model id's options (zero for none).
func (rt *Runtime) Options(id string) Options {
	if m := rt.opts.Load(); m != nil {
		return (*m)[id]
	}
	return Options{}
}

// hint is the language hint of a request in code, the user speaking
// others too, under the model's options.
func (a *ASR) hint(code string, others []string) string {
	if a.rt.Options(a.spec.ID).DetectLanguage {
		return ""
	}
	return a.spec.languageHint(code, others)
}

// temperature is the decoding's temperature under the model's options.
func (a *ASR) temperature() float64 {
	if t := a.rt.Options(a.spec.ID).Temperature; t != nil && a.spec.Kind == kindWhisper {
		return *t
	}
	return a.spec.ASR.Temperature
}

// Knobs are the settings the model takes (its Settings dialog): the
// main language as the hint (every speech model; off, it detects), and
// Whisper's temperature (whisper_server's field; qwen3_asr_server takes
// none). Silero takes none.
func (s Spec) Knobs() []knobs.Field {
	if s.ASR == nil {
		return nil
	}
	out := []knobs.Field{{Knob: knobs.MainLanguage, Auto: []float64{1}}}
	if s.Kind == kindWhisper {
		out = append(out, knobs.Field{Knob: knobs.Temperature, Min: 0, Max: 1, Auto: []float64{s.ASR.Temperature}})
	}
	return out
}
