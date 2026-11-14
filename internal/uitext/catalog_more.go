package uitext

// What the catalogue's other models (internal/models' catalog_more.go:
// Voxt's other local models and the family's) are good at. Their names
// are their own (models.Model.Title).

import (
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
)

// moreAbout says what one of the other models is good at ("" unknown).
func moreAbout(id string) string {
	switch id {
	case models.ASRQwen6bit:
		return i18n.T("The accurate model, a little smaller: nearly as accurate with less memory.")
	case models.ASRWhisperTurbo:
		return i18n.T("OpenAI's Whisper, quick for its size. Text comes after you stop.")
	case models.ASRWhisperLarge:
		return i18n.T("OpenAI's most accurate Whisper, slower and heavier. Text comes after you stop.")
	case models.ASRWhisperSmall:
		return i18n.T("A small Whisper for a Mac with little memory. Text comes after you stop.")
	case models.LLMQwen3:
		return i18n.T("Cleans up, translates and rewrites well.")
	case models.LLMGemma4:
		return i18n.T("Writes well in many languages, but is slower and needs more memory.")
	case models.LLMQwen35_9B:
		return i18n.T("A larger Qwen3.5: better clean-up and rewriting, for a Mac with 16 GB of memory or more.")
	case models.LLMGemma4_12B:
		return i18n.T("A larger Gemma 4 that writes well in many languages, for a Mac with 16 GB of memory or more.")
	case models.LLMQwen36_27B:
		return i18n.T("The largest and best, for a Mac with 32 GB of memory or more.")
	case models.LLMMinistral3:
		return i18n.T("A compact Mistral model, best in English.")
	case models.LLMGemma4E2B:
		return i18n.T("A compact Gemma 4: quick and light.")
	case models.LLMLFM2_8B:
		return i18n.T("A mixture of experts: quick for its size, for simple clean-ups and rewrites.")
	case models.LLMLFM2:
		return i18n.T("Very light and quick, for simple clean-ups on a Mac with little memory.")
	}
	return ""
}
