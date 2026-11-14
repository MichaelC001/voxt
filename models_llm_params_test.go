package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/hehehai/VoxtMini/internal/llmrt"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestLLMParamsSection: Advanced lists the installed language model's
// parameters as Automatic; a number typed is saved and reaches the
// running runtime; one out of range is not saved and says so; Reset
// brings Automatic back.
func TestLLMParamsSection(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.local = readyLocal()
	a.llmrt = llmrt.New(llmrt.Config{}) // not started: SetTuning only
	tt.SetSize(windowWidth, 3200)
	a.navigate(modelsPath + "/local")
	click(t, tt, "Advanced")
	if !hasExact(tt, "Generation") || !hasExact(tt, "Balanced language model") {
		t.Fatalf("no parameters: %q", tt.Texts())
	}
	clickBelow(t, tt, "Temperature")
	tt.Type("0.3")
	frames(tt)
	p := a.settings.Get().Models.LLMParamsOf(models.LLMBalanced)
	if p.Temperature == nil || *p.Temperature != 0.3 || p.TopP != nil {
		t.Fatalf("saved %+v", p)
	}
	eventually(t, "the runtime's tuning", func() bool {
		v := a.llmrt.Tuning(models.LLMBalanced).Temperature
		return v != nil && *v == 0.3
	})
	clickBelow(t, tt, "Max output tokens")
	tt.Type("99999")
	if !has(tt, "Max output tokens is out of range.") || a.settings.Get().Models.LLMParamsOf(models.LLMBalanced).MaxTokens != 0 {
		t.Errorf("out of range: %q", tt.Texts())
	}
	a.llmForm.maxTok = ""
	click(t, tt, "Reset to default")
	if !a.settings.Get().Models.LLMParamsOf(models.LLMBalanced).IsZero() || a.llmForm.temp != "" {
		t.Errorf("reset %+v %q", a.settings.Get().Models.LLMParams, a.llmForm.temp)
	}
	eventually(t, "the runtime's reset", func() bool { return a.llmrt.Tuning(models.LLMBalanced).Temperature == nil })
	// A change elsewhere shows.
	a.updateSettings(func(s *settings.Settings) {
		s.Models.LLMParams = s.Models.WithLLMParams(models.LLMBalanced, settings.LLMParams{MaxTokens: 512})
	})
	frames(tt)
	if a.llmForm.maxTok != "512" {
		t.Errorf("from the settings: %q", a.llmForm.maxTok)
	}
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
