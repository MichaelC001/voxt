package llmrt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/knobs"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/llmrt/llmrttest"
	"github.com/hehehai/VoxtMini/internal/llmrt/wire"
)

func f64(v float64) *float64 { return &v }

// TestTuningApply: a value set replaces the profile's; nil and 0 keep it;
// a repetition penalty gets a context when the profile has none.
func TestTuningApply(t *testing.T) {
	p := wire.ChatParams{Sampling: qwenRewrite, MaxTokens: 400}
	Tuning{}.apply(&p)
	if p.Sampling != qwenRewrite || p.MaxTokens != 400 {
		t.Fatalf("zero tuning changed %+v", p)
	}
	Tuning{Temperature: f64(0), TopP: f64(0.9), MaxTokens: 1000}.apply(&p)
	if p.Sampling.Temperature != 0 || p.Sampling.TopP != 0.9 || p.Sampling.TopK != 20 || p.MaxTokens != 1000 || p.Sampling.RepetitionContextSize != 32 {
		t.Errorf("tuned %+v", p)
	}
	g := wire.ChatParams{Sampling: gemmaExact}
	Tuning{RepetitionPenalty: f64(1.1)}.apply(&g)
	if g.Sampling.RepetitionPenalty != 1.1 || g.Sampling.RepetitionContextSize != DefaultRepetitionContext {
		t.Errorf("penalty %+v", g.Sampling)
	}
	l := wire.ChatParams{Sampling: lfmRewrite}
	Tuning{TopK: 40, MinP: f64(0)}.apply(&l)
	if l.Sampling.TopK != 40 || l.Sampling.MinP != 0 {
		t.Errorf("top-k, min-p %+v", l.Sampling)
	}
	// Thinking: only a template that can think, with room for the thought.
	q := wire.ChatParams{Template: qwenNoThink, MaxTokens: 400}
	Tuning{Think: true}.apply(&q)
	if q.Template[thinkVar] != true || qwenNoThink[thinkVar] != false || q.MaxTokens != 400+ThinkTokens {
		t.Errorf("think %+v (profile's %v)", q, qwenNoThink)
	}
	q = wire.ChatParams{Template: qwenNoThink, MaxTokens: 400}
	Tuning{Think: true, MaxTokens: 600}.apply(&q)
	if q.MaxTokens != 600 {
		t.Errorf("think, budget set: %d", q.MaxTokens)
	}
	n := wire.ChatParams{MaxTokens: 400}
	Tuning{Think: true}.apply(&n)
	if n.Template != nil || n.MaxTokens != 400 {
		t.Errorf("no thinking template: %+v", n)
	}
}

// TestProfileKnobs: every profile has settings; each family shows what
// its runtime applies (thinking for Qwen3.5 and Gemma 4, top-k where its
// sampling has one, min-p for LFM2), with its tuned values.
func TestProfileKnobs(t *testing.T) {
	names := func(fs []knobs.Field) string {
		var out []string
		for _, f := range fs {
			out = append(out, string(f.Knob))
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct{ id, want string }{
		{Balanced, "thinking,temperature,topP,topK,repetitionPenalty,maxTokens"},
		{Fast, "thinking,temperature,topP,topK,repetitionPenalty,maxTokens"},
		{Qwen36_27B, "thinking,temperature,topP,topK,repetitionPenalty,maxTokens"},
		{Gemma4, "thinking,temperature,topP,topK,repetitionPenalty,maxTokens"},
		{Qwen3, "temperature,topP,topK,repetitionPenalty,maxTokens"},
		{Translate, "temperature,topP,topK,repetitionPenalty,maxTokens"},
		{Ministral3, "temperature,topP,repetitionPenalty,maxTokens"},
		{LFM2, "temperature,minP,topP,repetitionPenalty,maxTokens"},
		{LFM2_8B, "temperature,minP,topP,repetitionPenalty,maxTokens"},
	} {
		if got := names(mustLookup(t, c.id).Knobs()); got != c.want {
			t.Errorf("%s: %s, want %s", c.id, got, c.want)
		}
	}
	for _, p := range profiles {
		if len(p.Knobs()) == 0 {
			t.Errorf("%s has no settings", p.ID)
		}
	}
	ks := mustLookup(t, Balanced).Knobs()
	if f := ks[1]; f.Knob != knobs.Temperature || len(f.Auto) != 3 || f.Auto[0] != 0 || f.Auto[1] != 0 || f.Auto[2] != 0.7 {
		t.Errorf("temperature %+v", f)
	}
	if f := ks[5]; f.Auto[0] != 1536 {
		t.Errorf("budget %+v", f)
	}
	if f := mustLookup(t, Translate).Knobs()[0]; len(f.Auto) != 1 || f.Auto[0] != 0.7 {
		t.Errorf("Hy-MT2's temperature %+v", f)
	}
}

func mustLookup(t *testing.T, id string) Profile {
	t.Helper()
	p, ok := Lookup(id)
	if !ok {
		t.Fatalf("no profile %s", id)
	}
	return p
}

// TestRuntimeTuning: the runtime's tuning reaches the process's chat, for
// its model only, and a change applies to the next chat.
func TestRuntimeTuning(t *testing.T) {
	log := filepath.Join(t.TempDir(), "chats.jsonl")
	rt, _ := newRuntime(t, llmrttest.Opt("reply", "Hi.")+","+llmrttest.Opt("chat-log", log), nil)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Hy-MT2-1.8B-Q4_K_M.gguf"), []byte("GGUF"), 0o644)
	chat, err := Open(context.Background(), rt, Translate, dir)
	if err != nil {
		t.Fatal(err)
	}
	req := llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "你好"}}, Skill: llm.SkillTranslateText, Translate: &llm.TranslateHint{Text: "你好", Target: "en"}}
	rt.SetTuning(map[string]Tuning{Translate: {Temperature: f64(0.2), MaxTokens: 300, RepetitionPenalty: f64(1.1)}, Balanced: {Temperature: f64(1.5)}})
	if _, err := chat.Chat(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	rt.SetTuning(nil)
	if _, err := chat.Chat(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	chats := chatLog(t, log)
	if c := chats[0]; c.Sampling.Temperature != 0.2 || c.MaxTokens != 300 || c.Sampling.RepetitionPenalty != 1.1 {
		t.Errorf("tuned chat %+v", c)
	}
	if c := chats[1]; c.Sampling.Temperature != 0.7 || c.MaxTokens == 300 {
		t.Errorf("untuned chat %+v", c)
	}
}
