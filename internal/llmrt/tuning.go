package llmrt

import (
	"maps"

	"github.com/hehehai/VoxtMini/internal/knobs"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/llmrt/wire"
)

// Tuning is the user's override of a model's generation
// (settings.ModelParams, the model's Settings dialog): a value set
// replaces the profile's for every skill; nil or 0 keeps the profile's,
// which is tuned per skill. Only what the model's family takes is set
// (Profile.Knobs).
type Tuning struct {
	Temperature       *float64
	TopP              *float64
	TopK              int
	MinP              *float64
	MaxTokens         int
	RepetitionPenalty *float64
	// Think turns the chat template's thinking on (a profile whose
	// template has enable_thinking): the answer's budget grows by
	// ThinkTokens unless MaxTokens sets it.
	Think bool
}

// ThinkTokens are the tokens added to an answer's budget for its
// thinking (llm.Client strips the thought; one cut off leaves no answer).
const ThinkTokens = 1024

// thinkVar is the chat template's variable that turns thinking on.
const thinkVar = "enable_thinking"

// apply sets p's sampling, budget and thinking from t.
func (t Tuning) apply(p *wire.ChatParams) {
	if t.Temperature != nil {
		p.Sampling.Temperature = *t.Temperature
	}
	if t.TopP != nil {
		p.Sampling.TopP = *t.TopP
	}
	if t.TopK > 0 {
		p.Sampling.TopK = t.TopK
	}
	if t.MinP != nil {
		p.Sampling.MinP = *t.MinP
	}
	if t.RepetitionPenalty != nil {
		p.Sampling.RepetitionPenalty = *t.RepetitionPenalty
		if p.Sampling.RepetitionContextSize == 0 && *t.RepetitionPenalty != 1 {
			p.Sampling.RepetitionContextSize = DefaultRepetitionContext
		}
	}
	if _, ok := p.Template[thinkVar]; ok && t.Think {
		p.Template = maps.Clone(p.Template)
		p.Template[thinkVar] = true
		p.MaxTokens += ThinkTokens
	}
	if t.MaxTokens > 0 {
		p.MaxTokens = t.MaxTokens
	}
}

// DefaultRepetitionContext is the tokens a repetition penalty looks back
// over when the profile sets none (Qwen's 32 tokens, as qwenExact).
const DefaultRepetitionContext = 32

// SetTuning replaces the models' tunings, by profile id; it applies to
// the next chat (a model need not load again).
func (r *Runtime) SetTuning(t map[string]Tuning) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tunings = maps.Clone(t)
}

// Tuning is model id's tuning (zero for none).
func (r *Runtime) Tuning(id string) Tuning {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tunings[id]
}

// Knobs are the settings the profile's model takes (its Settings
// dialog), with the profile's values for Automatic: temperature, top-p,
// the repetition penalty and the answer's budget for every model;
// thinking when its chat template can think (Qwen3.5, Gemma 4); top-k
// and min-p when its tuned sampling uses them (its model card's).
func (p Profile) Knobs() []knobs.Field {
	skills := []llm.Skill{llm.SkillTranscribeEnhance, llm.SkillTranslateText, llm.SkillRewrite}
	if p.TranslateOnly() {
		skills = []llm.Skill{llm.SkillTranslateText}
	}
	auto := func(v func(wire.Sampling) float64) []float64 {
		var out []float64
		for _, s := range skills {
			out = append(out, v(p.SamplingFor(s)))
		}
		return out
	}
	uses := func(v func(wire.Sampling) float64) bool {
		for _, s := range auto(v) {
			if s > 0 {
				return true
			}
		}
		return false
	}
	topP := func(s wire.Sampling) float64 {
		if s.TopP == 0 {
			return 1 // absent: neutral
		}
		return s.TopP
	}
	rep := func(s wire.Sampling) float64 {
		if s.RepetitionPenalty == 0 {
			return 1
		}
		return s.RepetitionPenalty
	}
	topK := func(s wire.Sampling) float64 { return float64(s.TopK) }
	minP := func(s wire.Sampling) float64 { return s.MinP }
	var out []knobs.Field
	if _, ok := p.Template[thinkVar]; ok {
		out = append(out, knobs.Field{Knob: knobs.Thinking, Auto: []float64{0}})
	}
	out = append(out, knobs.Field{Knob: knobs.Temperature, Min: 0, Max: 2, Auto: auto(func(s wire.Sampling) float64 { return s.Temperature })})
	if uses(minP) {
		out = append(out, knobs.Field{Knob: knobs.MinP, Min: 0, Max: 1, Auto: auto(minP)})
	}
	out = append(out, knobs.Field{Knob: knobs.TopP, Min: 0.01, Max: 1, Auto: auto(topP)})
	if uses(topK) {
		out = append(out, knobs.Field{Knob: knobs.TopK, Min: 1, Max: 200, Auto: auto(topK)})
	}
	out = append(out,
		knobs.Field{Knob: knobs.RepetitionPenalty, Min: 0.5, Max: 2, Auto: auto(rep)},
		knobs.Field{Knob: knobs.MaxTokens, Min: 16, Max: 8192, Auto: []float64{float64(p.MaxTokens(skills[len(skills)-1], 1<<20))}},
	)
	return out
}
