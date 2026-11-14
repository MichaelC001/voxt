package settings

import (
	"encoding/json"
	"testing"
)

func f64(v float64) *float64 { return &v }

// TestLLMParams: values out of range are dropped, a model that sets
// nothing goes, the map survives a save and a read and is copied deeply.
func TestLLMParams(t *testing.T) {
	var m Models
	m.LLMParams = m.WithLLMParams("llm.balanced", LLMParams{Temperature: f64(3), TopP: f64(0.9), MaxTokens: 99999, RepetitionPenalty: f64(1.1)})
	p := m.LLMParamsOf("llm.balanced")
	if p.Temperature != nil || *p.TopP != 0.9 || p.MaxTokens != 0 || *p.RepetitionPenalty != 1.1 {
		t.Fatalf("normalized %+v", p)
	}
	if m.WithLLMParams("llm.balanced", LLMParams{Temperature: f64(-1)}) != nil {
		t.Error("a model that sets nothing stays")
	}
	s := Defaults()
	s.Models = m
	data, _ := json.Marshal(s)
	got, err := Migrate(data)
	if err != nil || *got.Models.LLMParamsOf("llm.balanced").TopP != 0.9 {
		t.Fatalf("round trip %+v %v", got.Models, err)
	}
	c := clone(got)
	*c.Models.LLMParams["llm.balanced"].TopP = 0.1
	if *got.Models.LLMParamsOf("llm.balanced").TopP != 0.9 {
		t.Error("clone shares the values")
	}
	bad, _ := Migrate([]byte(`{"version":5,"models":{"llmParams":{"x":{"temperature":9},"":{"topP":0.5}}}}`))
	if bad.Models.LLMParams != nil {
		t.Errorf("kept %+v", bad.Models.LLMParams)
	}
}
