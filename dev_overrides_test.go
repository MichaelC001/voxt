package main

import (
	"testing"

	"github.com/hehehai/VoxtMini/internal/llmrt"
)

// TestDevOverrides: tests run as a development build (no update feed
// linked), and a published build ignores VOXTMINI_DEV_LLM's models.
func TestDevOverrides(t *testing.T) {
	if !devOverrides() {
		t.Fatal("a test binary counts as a published build")
	}
	a, _, _ := newTestApp(t)
	t.Setenv(llmrt.DevEnv, "llm.balanced="+t.TempDir())
	saved := devOverrides
	defer func() { devOverrides = saved }()
	devOverrides = func() bool { return false }
	base := a.local
	if got := a.withDevLLM(base); got != base {
		t.Error("a published build listed VOXTMINI_DEV_LLM's models")
	}
	logIgnoredOverrides() // only logs
}
