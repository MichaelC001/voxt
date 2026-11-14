// Package knobs describes the settings a local model's family takes
// (the model's Settings dialog on the Models page; Voxt's per-model
// configuration sheets): which knobs, their ranges and VoxtMini's tuned
// values for "Automatic". Each runtime says what its families honour
// (internal/llmrt's profiles, internal/omni's specs, internal/apple):
// a knob is listed only when the runtime applies it.
package knobs

// Knob is one setting. The values are settings.ModelParams' fields.
type Knob string

// The knobs.
const (
	// Thinking lets a language model think before it answers (its chat
	// template's enable_thinking). A switch, off by default.
	Thinking Knob = "thinking"
	// MainLanguage gives a speech model the user's main language as a
	// hint; off, the model detects the language. A switch, on by default.
	MainLanguage      Knob = "mainLanguage"
	Temperature       Knob = "temperature"
	TopP              Knob = "topP"
	TopK              Knob = "topK"
	MinP              Knob = "minP"
	RepetitionPenalty Knob = "repetitionPenalty"
	MaxTokens         Knob = "maxTokens"
)

// Switch reports whether k is on or off rather than a number.
func (k Knob) Switch() bool { return k == Thinking || k == MainLanguage }

// Whole reports whether k's value is a whole number.
func (k Knob) Whole() bool { return k == TopK || k == MaxTokens }

// Field is a knob as a family takes it: its range (a number) and its
// tuned value. Auto has one value for every task, or three: clean-up,
// translation and Rewrite (a language model tuned per task). For
// MaxTokens it is the most the automatic budget gives (it follows the
// input's length); for a switch 1 is on.
type Field struct {
	Knob     Knob
	Min, Max float64
	Auto     []float64
}

// On is a switch's automatic state.
func (f Field) On() bool { return len(f.Auto) > 0 && f.Auto[0] != 0 }
