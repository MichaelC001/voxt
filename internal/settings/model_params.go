package settings

import "maps"

// ModelParams are a local model's settings as the user set them (the
// model's Settings dialog on the Models page; Voxt's per-model
// configuration sheets). Which of them a model takes depends on its
// family (internal/knobs, listed by each runtime); the zero value of
// each is VoxtMini's own: nil or 0 = the tuned value for each task
// (internal/llmrt's profiles: greedy for clean-up and translation,
// warmer for Rewrite), Think false = no thinking, DetectLanguage false =
// the main language as the hint. A value set applies to every task.
type ModelParams struct {
	Temperature       *float64 `json:"temperature,omitempty"`
	TopP              *float64 `json:"topP,omitempty"`
	TopK              int      `json:"topK,omitempty"`
	MinP              *float64 `json:"minP,omitempty"`
	MaxTokens         int      `json:"maxTokens,omitempty"`
	RepetitionPenalty *float64 `json:"repetitionPenalty,omitempty"`
	// Think: a language model thinks before it answers.
	Think bool `json:"think,omitempty"`
	// DetectLanguage: a speech model gets no language hint and detects
	// the language itself.
	DetectLanguage bool `json:"detectLanguage,omitempty"`
}

// Ranges of ModelParams' values (a family may take less: internal/knobs).
const (
	MaxTemperature       = 2.0
	MaxRepetitionPenalty = 2.0
	MinRepetitionPenalty = 0.5
	MaxTopK              = 200
	MaxLLMTokens         = 8192
)

// IsZero says whether p sets nothing.
func (p ModelParams) IsZero() bool { return p == ModelParams{} || p.Normalized() == ModelParams{} }

// Normalized is p with every value in its range (a value out of it, or
// not a number, is dropped: the tuned one applies), its pointers its own.
func (p ModelParams) Normalized() ModelParams {
	in := func(v *float64, lo, hi float64) *float64 {
		if v == nil || !(*v >= lo && *v <= hi) {
			return nil
		}
		x := *v
		return &x
	}
	p.Temperature = in(p.Temperature, 0, MaxTemperature)
	p.TopP = in(p.TopP, 0.01, 1)
	p.MinP = in(p.MinP, 0, 1)
	p.RepetitionPenalty = in(p.RepetitionPenalty, MinRepetitionPenalty, MaxRepetitionPenalty)
	if p.TopK < 0 || p.TopK > MaxTopK {
		p.TopK = 0
	}
	if p.MaxTokens < 0 || p.MaxTokens > MaxLLMTokens {
		p.MaxTokens = 0
	}
	return p
}

// ParamsOf are model id's settings (zero for none).
func (m Models) ParamsOf(id string) ModelParams { return m.Params[id] }

// WithParams is m.Params with id's set to p (removed when p sets
// nothing), a new map; nil when empty.
func (m Models) WithParams(id string, p ModelParams) map[string]ModelParams {
	out := cloneParams(m.Params)
	if p = p.Normalized(); p.IsZero() {
		delete(out, id)
	} else {
		if out == nil {
			out = map[string]ModelParams{}
		}
		out[id] = p
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cloneParams copies m deeply; nil for none.
func cloneParams(m map[string]ModelParams) map[string]ModelParams {
	if len(m) == 0 {
		return nil
	}
	out := maps.Clone(m)
	for id, p := range out {
		out[id] = p.Normalized() // its own pointers
	}
	return out
}

// normalizeParams keeps each model's values in range and drops the
// models that set nothing. The language models' parameters saved under
// "llmParams" before the Settings dialog (Models → Advanced) join them;
// a model set in both keeps its newer values.
func normalizeParams(m Models) Models {
	out := map[string]ModelParams{}
	for id, p := range m.LegacyLLMParams {
		if p = p.Normalized(); !p.IsZero() && id != "" {
			out[id] = p
		}
	}
	for id, p := range m.Params {
		if p = p.Normalized(); !p.IsZero() && id != "" {
			out[id] = p
		}
	}
	m.LegacyLLMParams, m.Params = nil, nil
	if len(out) > 0 {
		m.Params = out
	}
	return m
}
