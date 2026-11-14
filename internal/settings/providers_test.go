package settings

import (
	"slices"
	"testing"
)

// TestDropRemovedProviders: a setup of a removed preset (Codex) goes, a
// feature that used it is Automatic again, the others stay.
func TestDropRemovedProviders(t *testing.T) {
	s, err := Migrate([]byte(`{"version":5,
		"features":{"transcribe":{"llm":"remote:codex"},"translate":{"llm":"remote:work"},"rewrite":{"llm":"remote:deepseek"}},
		"models":{"hidden":["remote:llm.codex","remote:llm.work","asr.accurate"]},
		"providers":{"llm":[{"id":"codex","enabled":true},{"id":"work","preset":"codex","enabled":true},{"id":"deepseek","enabled":true}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	f := s.Features
	if f.Transcribe.LLM != "" || f.Translate.LLM != "" || f.Rewrite.LLM != "remote:deepseek" {
		t.Errorf("features %q %q %q", f.Transcribe.LLM, f.Translate.LLM, f.Rewrite.LLM)
	}
	if len(s.Providers.LLM) != 1 || s.Providers.LLM[0].ID != "deepseek" {
		t.Errorf("providers %+v", s.Providers.LLM)
	}
	if !slices.Equal(s.Models.Hidden, []string{"asr.accurate"}) {
		t.Errorf("hidden %q", s.Models.Hidden)
	}
}
