package session

// The dictionary in a session: the terms of the session's app
// enhancement group (dict.Scoped), the aliases replaced in what was
// heard (spoken, cleanup.go), the hints and the glossary, and the uses
// counted when the session ends with a result.

import (
	"context"
	"slices"
	"time"

	"github.com/hehehai/VoxtMini/internal/appctx"
	"github.com/hehehai/VoxtMini/internal/dict"
	"github.com/hehehai/VoxtMini/internal/store"
)

// groupOf is a match's group id, "" for none.
func groupOf(m appctx.Match) string {
	if m.Matched() {
		return m.Group.ID
	}
	return ""
}

// termsIn are the dictionary's terms a session in group uses.
func (r *run) termsIn(group string) []store.Term {
	var exists func(string) bool
	if sc := r.m.cfg.Scene; sc != nil && sc.Rules != nil {
		rules, _ := sc.Rules()
		exists = func(id string) bool {
			for _, g := range rules.Groups {
				if g.ID == id {
					return true
				}
			}
			return false
		}
	}
	return dict.Scoped(r.terms, group, exists)
}

// sceneTerms are the terms of the session's group (waiting for it, as
// the composer does).
func (r *run) sceneTerms() []store.Term { return r.termsIn(groupOf(r.scene())) }

// earlyGroup is the session's group as far as it is known, without
// waiting: a browser whose address is still being read counts as an app
// (the speech model's hints go out as the recording starts).
func (r *run) earlyGroup() string {
	if m, ok := r.knownScene(); ok {
		return groupOf(m)
	}
	r.sc.mu.Lock()
	defer r.sc.mu.Unlock()
	return groupOf(r.sc.fallback)
}

// hints are the dictionary's terms for the speech model (dict.Hints:
// the enabled ones of the session's group, its own first, then the most
// used).
func (r *run) hints() []string {
	if !r.settings.Dictionary.Enabled || !r.settings.Dictionary.ASRHints {
		return nil
	}
	g := r.earlyGroup()
	return dict.Hints(r.termsIn(g), g)
}

// recordUses counts a use of each term the session's result has, or
// whose alias it replaced (Config.RecordUses). Not for tries.
func (r *run) recordUses(result string) {
	cfg := r.m.cfg
	if r.req.Try || cfg.RecordUses == nil || !r.settings.Dictionary.Enabled || result == "" || len(r.terms) == 0 {
		return
	}
	ids := dict.Used(result, r.sceneTerms())
	for _, c := range r.changes {
		ids = append(ids, c.TermID)
	}
	if len(ids) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cfg.RecordUses(ctx, ids, r.now()); err != nil {
		r.logf("warning", "dictionary uses: %v", err)
	}
}

// corrections are the history's record of the dictionary's changes.
func corrections(cs []dict.Change) []store.Correction {
	var out []store.Correction
	for _, c := range cs {
		sc := store.Correction{From: c.From, To: c.To}
		if !slices.Contains(out, sc) {
			out = append(out, sc)
		}
	}
	return out
}
