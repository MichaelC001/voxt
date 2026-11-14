package main

import (
	"context"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/knobs"
	"github.com/hehehai/VoxtMini/internal/llmrt"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/omni"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

// TestModelKnobsCatalogue: every speech and language model of the
// catalogue has settings (a Settings entry), Silero none; Apple
// Intelligence has its two.
func TestModelKnobsCatalogue(t *testing.T) {
	for _, m := range models.Catalog() {
		k := modelKnobs(m.ID)
		if (m.Kind == models.VAD) != (len(k) == 0) {
			t.Errorf("%s (%v): %+v", m.ID, m.Kind, k)
		}
	}
	if k := modelKnobs(apple.IntelligenceID); len(k) != 2 || modelKnobs(apple.SpeechID) != nil {
		t.Errorf("Apple's: %+v", k)
	}
}

// knobsOf are the knobs model id shows.
func knobsOf(id string) []knobs.Knob {
	var out []knobs.Knob
	for _, f := range modelKnobs(id) {
		out = append(out, f.Knob)
	}
	return out
}

// TestModelSettingsDialog: an installed model's ⋯ opens its Settings with
// its family's fields only; a number typed is saved and reaches the
// running runtime; one out of range is not saved and says so; a switch
// saves at once; Reset brings Automatic back; Models → Advanced has no
// parameters any more.
func TestModelSettingsDialog(t *testing.T) {
	a, _, tt := newTestApp(t)
	tt.SetSize(windowWidth, 3200)
	seedModels(t, a, models.LLMBalanced, models.LLMLFM2)
	a.llmrt = llmrt.New(llmrt.Config{}) // not started: SetTuning only
	a.navigate(modelsPath + "/language")
	frames(tt)
	click(t, tt, "Advanced")
	if has(tt, "Temperature") || has(tt, "Generation") {
		t.Errorf("parameters left in Advanced: %q", tt.Texts())
	}
	chooseMenu(t, tt, "More actions for Balanced language model", "Settings")
	if !a.modelSet.open || a.modelSet.id != models.LLMBalanced {
		t.Fatalf("dialog %+v", a.modelSet)
	}
	for _, s := range []string{"Thinking", "Temperature", "Top-p", "Top-k", "Repetition penalty", "Max output tokens"} {
		if !hasExact(tt, s) {
			t.Errorf("no %q: %q", s, tt.Texts())
		}
	}
	if hasExact(tt, "Min-p") {
		t.Error("Qwen3.5 shows min-p")
	}
	clickBelow(t, tt, "Temperature")
	tt.Type("0.3")
	frames(tt)
	p := a.settings.Get().Models.ParamsOf(models.LLMBalanced)
	if p.Temperature == nil || *p.Temperature != 0.3 || p.TopP != nil {
		t.Fatalf("saved %+v", p)
	}
	eventually(t, "the runtime's tuning", func() bool {
		v := a.llmrt.Tuning(models.LLMBalanced).Temperature
		return v != nil && *v == 0.3
	})
	clickRowEnd(t, tt, "Thinking") // its switch
	if !a.settings.Get().Models.ParamsOf(models.LLMBalanced).Think {
		t.Fatalf("thinking not saved: %+v", a.settings.Get().Models.ParamsOf(models.LLMBalanced))
	}
	eventually(t, "the runtime's thinking", func() bool { return a.llmrt.Tuning(models.LLMBalanced).Think })
	clickBelow(t, tt, "Max output tokens")
	tt.Type("99999")
	if !has(tt, "Max output tokens is out of range.") || a.settings.Get().Models.ParamsOf(models.LLMBalanced).MaxTokens != 0 {
		t.Errorf("out of range: %q", tt.Texts())
	}
	click(t, tt, "Reset to default")
	if !a.settings.Get().Models.ParamsOf(models.LLMBalanced).IsZero() || a.modelSet.vals[knobs.Temperature] != "" || has(tt, "is out of range.") {
		t.Errorf("reset %+v %q", a.settings.Get().Models.Params, a.modelSet.vals)
	}
	eventually(t, "the runtime's reset", func() bool { return a.llmrt.Tuning(models.LLMBalanced).Temperature == nil })
	// A change elsewhere shows.
	a.updateSettings(func(s *settings.Settings) {
		s.Models.Params = s.Models.WithParams(models.LLMBalanced, settings.ModelParams{MaxTokens: 512})
	})
	frames(tt)
	if a.modelSet.vals[knobs.MaxTokens] != "512" {
		t.Errorf("from the settings: %q", a.modelSet.vals)
	}
	click(t, tt, "Done")
	if a.modelSet.open {
		t.Error("Done left it open")
	}
	// Another family: LFM2 has min-p, no thinking, no top-k.
	chooseMenu(t, tt, "More actions for LFM2 1.2B · 4-bit", "Settings")
	frames(tt)
	if !hasExact(tt, "Min-p") || hasExact(tt, "Thinking") || hasExact(tt, "Top-k") {
		t.Errorf("LFM2: %q", tt.Texts())
	}
}

// TestModelSettingsKinds: a speech model's and Apple Intelligence's
// settings reach their runtimes: Whisper's temperature and language,
// Apple's temperature and budget.
func TestModelSettingsKinds(t *testing.T) {
	if got := knobsOf(models.ASRAccurate); !slices.Equal(got, []knobs.Knob{knobs.MainLanguage}) {
		t.Errorf("Qwen3-ASR: %v", got)
	}
	if got := knobsOf(models.ASRWhisperTurbo); !slices.Equal(got, []knobs.Knob{knobs.MainLanguage, knobs.Temperature}) {
		t.Errorf("Whisper: %v", got)
	}
	half := 0.5
	m := settings.Models{}
	m.Params = m.WithParams(models.ASRWhisperTurbo, settings.ModelParams{DetectLanguage: true, Temperature: &half})
	m.Params = m.WithParams(apple.IntelligenceID, settings.ModelParams{Temperature: &half, MaxTokens: 300})
	m.Params = m.WithParams(models.LLMBalanced, settings.ModelParams{Think: true})
	o := omniOptions(m)
	if w := o[models.ASRWhisperTurbo]; !w.DetectLanguage || *w.Temperature != 0.5 || len(o) != 1 {
		t.Errorf("omni %+v", o)
	}
	if p := appleParams(m); *p.Temperature != 0.5 || p.MaxTokens != 300 {
		t.Errorf("apple %+v", p)
	}
	if l := llmTunings(m); len(l) != 1 || !l[models.LLMBalanced].Think {
		t.Errorf("llm %+v", l)
	}
	rt := omni.NewRuntime(omni.Config{})
	a, _, _ := newTestApp(t)
	a.omniRT = rt
	a.updateSettings(func(s *settings.Settings) { s.Models.Params = m.Params })
	eventually(t, "omni's options", func() bool { return rt.Options(models.ASRWhisperTurbo).DetectLanguage })
}

// TestModelThinks: a session's language model thinks (longer deadlines)
// only when its Thinking is on and its family can think; never a remote
// one; the resolver passes it on.
func TestModelThinks(t *testing.T) {
	m := settings.Models{}
	for _, id := range []string{models.LLMBalanced, models.LLMLFM2} {
		m.Params = m.WithParams(id, settings.ModelParams{Think: true})
	}
	for _, c := range []struct {
		m    local.Model
		want bool
	}{
		{local.Model{ID: models.LLMBalanced}, true},
		{local.Model{ID: models.LLMFast}, false}, // not on
		{local.Model{ID: models.LLMLFM2}, false}, // cannot think
		{local.Model{ID: models.LLMBalanced, Remote: true}, false},
	} {
		if got := modelThinks(m, c.m); got != c.want {
			t.Errorf("%+v: %v", c.m, got)
		}
	}
	s := settings.Defaults()
	s.Models = m
	s.Features.Rewrite.LLM = fakeLLMModel.ID
	lm := readyLocal()
	svc, err := resolver(lm)(context.Background(), store.KindRewrite, s)
	if err != nil || !svc.LLMInfo.Thinks { // fakeLLMModel is the balanced model, Thinking on
		t.Errorf("resolved %+v %v", svc.LLMInfo, err)
	}
}

// clickRowEnd clicks the end of the open dialog's row titled title: its
// control (dialogFrame: 520 wide at most, centred, its body padded by
// sp6).
func clickRowEnd(t *testing.T, tt *ui.Tester, title string) {
	t.Helper()
	frames(tt)
	r, ok := tt.Find(title)
	if !ok {
		t.Fatalf("no %q", title)
	}
	w := float32(windowWidth)
	pw := min(w-2*sp6, 520)
	tt.ClickAt((w+pw)/2-sp6-12, r.Y+r.H/2)
	frames(tt)
}

// clickBelow clicks the field under the label: a click on the label
// itself finds its text, not the field.
func clickBelow(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	frames(tt)
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("no %q", label)
	}
	tt.ClickAt(r.X+8, r.Y+r.H+16)
	frames(tt)
}
