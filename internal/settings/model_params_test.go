package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

// TestModelParams: values out of range are dropped, a model that sets
// nothing goes, the map survives a save and a read and is copied deeply.
func TestModelParams(t *testing.T) {
	var m Models
	m.Params = m.WithParams("llm.balanced", ModelParams{Temperature: f64(3), TopP: f64(0.9), MaxTokens: 99999, RepetitionPenalty: f64(1.1), TopK: 999, MinP: f64(2), Think: true})
	p := m.ParamsOf("llm.balanced")
	if p.Temperature != nil || *p.TopP != 0.9 || p.MaxTokens != 0 || *p.RepetitionPenalty != 1.1 || p.TopK != 0 || p.MinP != nil || !p.Think {
		t.Fatalf("normalized %+v", p)
	}
	if m.WithParams("llm.balanced", ModelParams{Temperature: f64(-1)}) != nil {
		t.Error("a model that sets nothing stays")
	}
	m.Params = m.WithParams("asr.whisper-turbo", ModelParams{DetectLanguage: true})
	s := Defaults()
	s.Models = m
	data, _ := json.Marshal(s)
	if strings.Contains(string(data), "llmParams") {
		t.Errorf("wrote the old key: %s", data)
	}
	got, err := Migrate(data)
	if err != nil || *got.Models.ParamsOf("llm.balanced").TopP != 0.9 || !got.Models.ParamsOf("asr.whisper-turbo").DetectLanguage {
		t.Fatalf("round trip %+v %v", got.Models, err)
	}
	c := clone(got)
	*c.Models.Params["llm.balanced"].TopP = 0.1
	if *got.Models.ParamsOf("llm.balanced").TopP != 0.9 {
		t.Error("clone shares the values")
	}
	bad, _ := Migrate([]byte(`{"version":5,"models":{"modelParams":{"x":{"temperature":9},"":{"topP":0.5}}}}`))
	if bad.Models.Params != nil {
		t.Errorf("kept %+v", bad.Models.Params)
	}
}

// TestModelParamsMigrate: the language models' parameters Models →
// Advanced saved ("llmParams") move to the per-model map, every value
// kept; one set in both keeps the new map's.
func TestModelParamsMigrate(t *testing.T) {
	old := `{"version":5,"models":{"llmParams":{
		"llm.balanced":{"temperature":0.3,"topP":0.9,"maxTokens":512,"repetitionPenalty":1.1},
		"llm.translate":{"temperature":0.2}},
		"modelParams":{"llm.translate":{"temperature":0.5}}}}`
	s, err := Migrate([]byte(old))
	if err != nil {
		t.Fatal(err)
	}
	b := s.Models.ParamsOf("llm.balanced")
	if b.Temperature == nil || *b.Temperature != 0.3 || *b.TopP != 0.9 || b.MaxTokens != 512 || *b.RepetitionPenalty != 1.1 {
		t.Errorf("balanced %+v", b)
	}
	if tr := s.Models.ParamsOf("llm.translate"); tr.Temperature == nil || *tr.Temperature != 0.5 {
		t.Errorf("translate %+v", tr)
	}
	if s.Models.LegacyLLMParams != nil {
		t.Errorf("old map kept: %+v", s.Models.LegacyLLMParams)
	}
	data, _ := json.Marshal(s)
	if strings.Contains(string(data), "llmParams") || !strings.Contains(string(data), `"modelParams"`) {
		t.Errorf("saved %s", data)
	}
}
