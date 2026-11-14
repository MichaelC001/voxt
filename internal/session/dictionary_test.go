package session

import (
	"context"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/appctx"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

// TestDictionaryScopeUsesCorrections: a session uses the terms of no
// group, of its own group and of a group that no longer exists (not
// another group's); its own group's terms go first in the hints; the
// history keeps what the aliases became, and the terms the result has
// (or whose alias it replaced) count a use. A try (no group) counts
// none.
func TestDictionaryScopeUsesCorrections(t *testing.T) {
	terms := []store.Term{
		{ID: "t1", Term: "VoxtMini", Aliases: []string{"aura link"}, Enabled: true, Uses: 5},
		{ID: "t2", Term: "Slackbot", Aliases: []string{"slack bought"}, Enabled: true, Group: "g-chat"},
		{ID: "t3", Term: "Codex", Aliases: []string{"code ex"}, Enabled: true, Group: "g-code"},
		{ID: "t4", Term: "Gone", Aliases: []string{"gone term"}, Enabled: true, Group: "g-deleted"},
		{ID: "t5", Term: "Kubernetes", Enabled: true},
		{ID: "t6", Term: "Unused", Enabled: true},
	}
	for _, try := range []bool{false, true} {
		var mu sync.Mutex
		var used [][]string
		file := &fakeFile{text: "aura link and slack bought, code ex, gone term, Aura Link, Kubernetes"}
		h := newHarness(t, file, nil, func(c *Config, s *settings.Settings) {
			c.Glossary = func(context.Context) ([]store.Term, error) { return terms, nil }
			c.RecordUses = func(_ context.Context, ids []string, at time.Time) error {
				mu.Lock()
				used = append(used, ids)
				mu.Unlock()
				return nil
			}
			c.Scene = &SceneConfig{Rules: func() (appctx.Rules, bool) { return sceneRules(), true }}
		})
		id, err := h.m.Start(StartRequest{Kind: store.KindTranscribe, Target: slack, Try: try})
		if err != nil {
			t.Fatal(err)
		}
		h.speak(id, 3)
		end := h.rec.wait(t)
		result, wantHints := "VoxtMini and Slackbot, code ex, Gone, VoxtMini, Kubernetes", []string{"Slackbot", "VoxtMini", "Gone", "Kubernetes", "Unused"}
		if try {
			result, wantHints = "VoxtMini and slack bought, code ex, Gone, VoxtMini, Kubernetes", []string{"VoxtMini", "Gone", "Kubernetes", "Unused"}
		}
		if end.Result != result {
			t.Fatalf("try %v: result %q", try, end.Result)
		}
		file.mu.Lock()
		hints := file.hints
		file.mu.Unlock()
		if !slices.Equal(hints, wantHints) {
			t.Errorf("try %v: hints %q, want %q", try, hints, wantHints)
		}
		mu.Lock()
		got := used
		mu.Unlock()
		if try {
			if len(got) != 0 || len(h.hist.all()) != 0 {
				t.Errorf("a try counted %q", got)
			}
			continue
		}
		if len(got) != 1 {
			t.Fatalf("uses %q", got)
		}
		ids := slices.Clone(got[0])
		slices.Sort(ids)
		if ids = slices.Compact(ids); !slices.Equal(ids, []string{"t1", "t2", "t4", "t5"}) {
			t.Errorf("used %q", ids)
		}
		es := h.hist.all()
		want := []store.Correction{{From: "aura link", To: "VoxtMini"}, {From: "slack bought", To: "Slackbot"}, {From: "gone term", To: "Gone"}, {From: "Aura Link", To: "VoxtMini"}}
		if len(es) != 1 || !reflect.DeepEqual(es[0].Corrections, want) {
			t.Errorf("corrections %+v", es)
		}
	}
}
