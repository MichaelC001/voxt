package llmrt

import (
	"maps"

	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/llmrt/wire"
)

// Tuning is the user's override of a model's generation
// (settings.LLMParams): a value set replaces the profile's for every
// skill; nil or 0 keeps the profile's, which is tuned per skill.
type Tuning struct {
	Temperature       *float64
	TopP              *float64
	MaxTokens         int
	RepetitionPenalty *float64
}

// apply sets p's sampling and budget from t.
func (t Tuning) apply(p *wire.ChatParams) {
	if t.Temperature != nil {
		p.Sampling.Temperature = *t.Temperature
	}
	if t.TopP != nil {
		p.Sampling.TopP = *t.TopP
	}
	if t.RepetitionPenalty != nil {
		p.Sampling.RepetitionPenalty = *t.RepetitionPenalty
		if p.Sampling.RepetitionContextSize == 0 && *t.RepetitionPenalty != 1 {
			p.Sampling.RepetitionContextSize = DefaultRepetitionContext
		}
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

// Defaults are a profile's tuned values the UI shows for "Automatic":
// for clean-up and translation (exact) and for Rewrite (and the
// budget's bounds, which depend on the input's length).
func (p Profile) Defaults() (exact, rewrite wire.Sampling, maxTokens int) {
	exact = p.SamplingFor(llm.SkillTranscribeEnhance)
	rewrite = p.SamplingFor(llm.SkillRewrite)
	if p.TranslateOnly() {
		rewrite = p.SamplingFor(llm.SkillTranslateText)
		exact = rewrite
	}
	return exact, rewrite, p.MaxTokens(llm.SkillRewrite, 1<<20)
}
