package apple

import "github.com/hehehai/VoxtMini/internal/knobs"

// Params are the user's settings of Apple Intelligence (settings.
// ModelParams, its Settings dialog): what the helper passes to
// FoundationModels' GenerationOptions. nil or 0: VoxtMini's per task.
type Params struct {
	Temperature *float64
	MaxTokens   int
}

// SetParams replaces Apple Intelligence's settings; they apply to the
// next request.
func (s *Services) SetParams(p Params) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.params = p
}

func (s *Services) paramsNow() Params {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.params
}

// apply sets req's temperature and budget from p (p's win, as a local
// model's tuning).
func (p Params) apply(temp *float64, budget *int) {
	if p.Temperature != nil {
		*temp = *p.Temperature
	}
	if p.MaxTokens > 0 {
		*budget = p.MaxTokens
	}
}

// IntelligenceKnobs are the settings Apple Intelligence takes: the
// temperature (llm.DefaultTemperature's per task: clean-up,
// translation, Rewrite) and the answer's budget (at most what its window
// leaves; MaxAnswer by default).
func IntelligenceKnobs() []knobs.Field {
	return []knobs.Field{
		{Knob: knobs.Temperature, Min: 0, Max: 2, Auto: []float64{0.1, 0.2, 0.5}},
		{Knob: knobs.MaxTokens, Min: 16, Max: Window, Auto: []float64{MaxAnswer}},
	}
}
