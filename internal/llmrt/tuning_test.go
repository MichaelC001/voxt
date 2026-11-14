package llmrt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	exact, rewrite, max := mustLookup(t, Balanced).Defaults()
	if exact.Temperature != 0 || rewrite.Temperature != 0.7 || max != 1536 {
		t.Errorf("defaults %+v %+v %d", exact, rewrite, max)
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
