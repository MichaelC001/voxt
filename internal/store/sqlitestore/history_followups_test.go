package sqlitestore

import (
	"errors"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/store"
)

func TestHistoryFollowUps(t *testing.T) {
	s := openTest(t, t.TempDir())
	h := s.History()
	at := time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)
	e := entry("01HFOLLOW", store.KindTranscribe, at, "raw", "result")
	if err := h.Put(ctx, e); err != nil {
		t.Fatal(err)
	}
	if got, err := h.FollowUps(ctx, e.ID); got != nil || err != nil {
		t.Fatalf("follow-ups of a new entry: %v, %v", got, err)
	}
	f := []store.FollowUp{{Question: "When?", Answer: "Thursday.", At: at}, {Question: "Who?", Answer: "The team | all of it", At: at.Add(time.Minute)}}
	if err := h.SetFollowUps(ctx, e.ID, f); err != nil {
		t.Fatal(err)
	}
	check := func(what string) {
		t.Helper()
		got, err := h.FollowUps(ctx, e.ID)
		if err != nil || len(got) != 2 || got[1].Answer != "The team | all of it" || !got[0].At.Equal(at) {
			t.Errorf("%s: %+v, %v", what, got, err)
		}
	}
	check("set")
	// Put (a retry, say) keeps them.
	e.Result = "another result"
	if err := h.Put(ctx, e); err != nil {
		t.Fatal(err)
	}
	check("after Put")
	if err := h.SetFollowUps(ctx, e.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.FollowUps(ctx, e.ID); len(got) != 0 {
		t.Errorf("cleared: %v", got)
	}
	if err := h.SetFollowUps(ctx, "01HNONE", f); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown entry: %v", err)
	}
	if _, err := h.FollowUps(ctx, "01HNONE"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown entry: %v", err)
	}
}
