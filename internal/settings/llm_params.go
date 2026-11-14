package settings

import "maps"

// LLMParams are a local language model's generation parameters as the
// user set them (the Models page's Advanced section; Voxt's
// CustomLLMGenerationSettingsSheet): nil or 0 = VoxtMini's tuned value
// for each task (internal/llmrt's profiles: greedy for clean-up and
// translation, warmer for Rewrite). A value set applies to every task.
type LLMParams struct {
	Temperature       *float64 `json:"temperature,omitempty"`
	TopP              *float64 `json:"topP,omitempty"`
	MaxTokens         int      `json:"maxTokens,omitempty"`
	RepetitionPenalty *float64 `json:"repetitionPenalty,omitempty"`
}

// Ranges of LLMParams' values.
const (
	MaxTemperature       = 2.0
	MaxRepetitionPenalty = 2.0
	MinRepetitionPenalty = 0.5
	MaxLLMTokens         = 8192
)

// IsZero says whether p sets nothing.
func (p LLMParams) IsZero() bool {
	return p.Temperature == nil && p.TopP == nil && p.MaxTokens <= 0 && p.RepetitionPenalty == nil
}

// Normalized is p with every value in its range (a value out of it, or
// not a number, is dropped: the tuned one applies).
func (p LLMParams) Normalized() LLMParams {
	in := func(v *float64, lo, hi float64) *float64 {
		if v == nil || !(*v >= lo && *v <= hi) {
			return nil
		}
		x := *v
		return &x
	}
	p.Temperature = in(p.Temperature, 0, MaxTemperature)
	p.TopP = in(p.TopP, 0.01, 1)
	p.RepetitionPenalty = in(p.RepetitionPenalty, MinRepetitionPenalty, MaxRepetitionPenalty)
	if p.MaxTokens < 0 || p.MaxTokens > MaxLLMTokens {
		p.MaxTokens = 0
	}
	return p
}

// LLMParamsOf are model id's parameters (zero for none).
func (m Models) LLMParamsOf(id string) LLMParams { return m.LLMParams[id] }

// WithLLMParams is m.LLMParams with id's set to p (removed when p sets
// nothing), a new map; nil when empty.
func (m Models) WithLLMParams(id string, p LLMParams) map[string]LLMParams {
	out := cloneLLMParams(m.LLMParams)
	if p = p.Normalized(); p.IsZero() {
		delete(out, id)
	} else {
		if out == nil {
			out = map[string]LLMParams{}
		}
		out[id] = p
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cloneLLMParams copies m deeply; nil for none.
func cloneLLMParams(m map[string]LLMParams) map[string]LLMParams {
	if len(m) == 0 {
		return nil
	}
	out := maps.Clone(m)
	for id, p := range out {
		out[id] = p.Normalized() // its own pointers
	}
	return out
}

// normalizeLLMParams keeps each model's values in range and drops the
// models that set nothing.
func normalizeLLMParams(m map[string]LLMParams) map[string]LLMParams {
	out := map[string]LLMParams{}
	for id, p := range m {
		if p = p.Normalized(); !p.IsZero() && id != "" {
			out[id] = p
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
