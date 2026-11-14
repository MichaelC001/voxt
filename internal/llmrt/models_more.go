package llmrt

import (
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/llmrt/wire"
)

// The rest of the catalogue's language models (internal/models'
// catalog_more.go): Voxt's other local models, which voxtmini-llm loads
// with no code of its own (their model types are in mlx-swift-lm's
// registry: qwen3_5, gemma4, gemma4_unified, mistral3, lfm2,
// lfm2_moe). Experimental until checked on a Mac (docs/llm-models.md,
// "More models"): their sampling follows each model card, greedy for the
// exact skills as the curated ones.
const (
	Qwen35_9B  = "llm.qwen35-9b"  // Qwen3.5 9B OptiQ
	Gemma4_12B = "llm.gemma4-12b" // Gemma 4 12B OptiQ
	Qwen36_27B = "llm.qwen36-27b" // Qwen3.6 27B
	Ministral3 = "llm.ministral3" // Ministral 3 3B
	Gemma4E2B  = "llm.gemma4-e2b" // Gemma 4 E2B
	LFM2_8B    = "llm.lfm2-8b"    // LFM2 8B-A1B
	LFM2       = "llm.lfm2"       // LFM2 1.2B
)

var (
	// Gemma 4's card: 1.0, top-p 0.95, top-k 64 for rewriting; greedy
	// without a repetition penalty for the rest; no thinking.
	gemmaExact   = wire.Sampling{Temperature: 0, TopP: 1, RepetitionPenalty: 1}
	gemmaRewrite = wire.Sampling{Temperature: 1.0, TopP: 0.95, TopK: 64, RepetitionPenalty: 1}
	// LFM2's card: temperature 0.3, min-p 0.15, repetition penalty 1.05.
	lfmExact   = wire.Sampling{Temperature: 0, TopP: 1, RepetitionPenalty: 1.05, RepetitionContextSize: 32}
	lfmRewrite = wire.Sampling{Temperature: 0.3, MinP: 0.15, RepetitionPenalty: 1.05, RepetitionContextSize: 32}
	// Mistral's card: a low temperature (0.15) for instruction following.
	mistralRewrite = wire.Sampling{Temperature: 0.15, TopP: 1, RepetitionPenalty: 1}
)

// qwenProfile is a Qwen3.5-family model run as llm.balanced.
func qwenProfile(id, repo string) Profile {
	return Profile{ID: id, Format: wire.FormatMLX, Repo: repo,
		Sampling: map[llm.Skill]wire.Sampling{llm.SkillRewrite: qwenRewrite},
		Default:  qwenExact, Template: qwenNoThink}
}

// gemmaProfile is a Gemma 4 model run as llm.gemma4.
func gemmaProfile(id, repo string) Profile {
	return Profile{ID: id, Format: wire.FormatMLX, Repo: repo,
		Sampling: map[llm.Skill]wire.Sampling{llm.SkillRewrite: gemmaRewrite},
		Default:  gemmaExact, Template: qwenNoThink}
}

var moreProfiles = []Profile{
	qwenProfile(Qwen35_9B, "mlx-community/Qwen3.5-9B-OptiQ-4bit"),
	gemmaProfile(Gemma4_12B, "mlx-community/gemma-4-12B-it-OptiQ-4bit"),
	qwenProfile(Qwen36_27B, "mlx-community/Qwen3.6-27B-4bit"),
	{
		ID: Ministral3, Format: wire.FormatMLX, Repo: "mlx-community/Ministral-3-3B-Instruct-2512-4bit",
		Sampling: map[llm.Skill]wire.Sampling{llm.SkillRewrite: mistralRewrite},
		Default:  gemmaExact,
	},
	gemmaProfile(Gemma4E2B, "mlx-community/gemma-4-e2b-it-4bit"),
	{
		ID: LFM2_8B, Format: wire.FormatMLX, Repo: "mlx-community/LFM2-8B-A1B-3bit-MLX",
		Sampling: map[llm.Skill]wire.Sampling{llm.SkillRewrite: lfmRewrite},
		Default:  lfmExact,
	},
	{
		ID: LFM2, Format: wire.FormatMLX, Repo: "mlx-community/LFM2-1.2B-4bit",
		Sampling: map[llm.Skill]wire.Sampling{llm.SkillRewrite: lfmRewrite},
		Default:  lfmExact,
	},
}
