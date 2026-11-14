package main

// A local model's settings (settings.Models.Params; the model's Settings
// dialog, models_settings_view.go): what its family takes, as its
// runtime says (internal/knobs: llmrt's profiles, omni's specs, Apple
// Intelligence's), each Automatic (VoxtMini's tuned value) or the user's.
// A change applies to the next request (llmrt.Runtime.SetTuning,
// omni.Runtime.SetOptions, apple.Services.SetParams); a model need not
// load again.

import (
	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/knobs"
	"github.com/hehehai/VoxtMini/internal/llmrt"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/omni"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// modelKnobs are the settings model id takes (nil: none, no Settings
// entry): a language model's by its profile, a speech model's by its
// spec, Apple Intelligence's.
func modelKnobs(id string) []knobs.Field {
	if p, ok := llmrt.Lookup(id); ok {
		return p.Knobs()
	}
	if s, ok := omni.Lookup(id); ok {
		return s.Knobs()
	}
	if id == apple.IntelligenceID {
		return apple.IntelligenceKnobs()
	}
	return nil
}

// modelThinks reports whether local language model m thinks before it
// answers: its Thinking is on and its family can think (the session
// gives its requests longer deadlines, session.ThinkFactor).
func modelThinks(ms settings.Models, m local.Model) bool {
	if m.Remote || !ms.ParamsOf(m.ID).Think {
		return false
	}
	for _, f := range modelKnobs(m.ID) {
		if f.Knob == knobs.Thinking {
			return true
		}
	}
	return false
}

// llmTunings are m's language models' settings as the runtime's tunings.
func llmTunings(m settings.Models) map[string]llmrt.Tuning {
	out := map[string]llmrt.Tuning{}
	for id, p := range m.Params {
		if _, ok := llmrt.Lookup(id); ok {
			out[id] = llmrt.Tuning{Temperature: p.Temperature, TopP: p.TopP, TopK: p.TopK, MinP: p.MinP,
				MaxTokens: p.MaxTokens, RepetitionPenalty: p.RepetitionPenalty, Think: p.Think}
		}
	}
	return out
}

// omniOptions are m's speech models' settings as the runtime's options.
func omniOptions(m settings.Models) map[string]omni.Options {
	out := map[string]omni.Options{}
	for id, p := range m.Params {
		if s, ok := omni.Lookup(id); ok && s.ASR != nil {
			out[id] = omni.Options{DetectLanguage: p.DetectLanguage, Temperature: p.Temperature}
		}
	}
	return out
}

// appleParams are Apple Intelligence's settings in m.
func appleParams(m settings.Models) apple.Params {
	p := m.ParamsOf(apple.IntelligenceID)
	return apple.Params{Temperature: p.Temperature, MaxTokens: p.MaxTokens}
}

// applyModelParams gives the runtimes made so far m's settings
// (settingsChanged; llmRuntime, omniRuntime and openModels set them on
// the ones they make).
func (a *app) applyModelParams(m settings.Models) {
	if l := a.llmMade(); l != nil {
		l.SetTuning(llmTunings(m))
	}
	if a.apples != nil {
		a.apples.SetParams(appleParams(m))
	}
	a.async(func() {
		a.omniMu.Lock()
		rt := a.omniRT
		a.omniMu.Unlock()
		if rt != nil {
			rt.SetOptions(omniOptions(m))
		}
	})
}
