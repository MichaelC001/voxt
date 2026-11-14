package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/store"
)

// askIn types q into the follow-up field and asks.
func askIn(t *testing.T, a *app, tt *ui.Tester, q string) {
	t.Helper()
	a.hist.ask.input = q
	click(t, tt, "Ask")
}

func TestHistoryFollowUp(t *testing.T) {
	a, _, tt := newTestApp(t)
	seedHistory(t, a)
	chat := &fakeChat{}
	chat.set("Thursday afternoon.", 0)
	a.local = withChat(readyLocal(), chat)
	tt.SetSize(1200, 1600) // the whole details panel in view
	a.navigate("/history")
	click(t, tt, "Let's move the design review to Thursday afternoon.")
	if !has(tt, "Follow-up questions") || !a.hist.ask.loaded {
		t.Fatalf("no follow-up section: %q", tt.Texts())
	}
	askIn(t, a, tt, "When is the review?")
	frames(tt)
	f := &a.hist.ask
	if len(f.list) != 1 || f.list[0].Answer != "Thursday afternoon." || f.asking != "" || f.input != "" {
		t.Fatalf("after asking: %+v", f)
	}
	if !has(tt, "When is the review?") || !has(tt, "Thursday afternoon.") {
		t.Errorf("the chat does not show: %q", tt.Texts())
	}
	reqs := chat.requests()
	if len(reqs) != 1 || reqs[0]["stream"] != true {
		t.Fatalf("requests %v", reqs)
	}
	msgs := reqs[0]["messages"].([]any)
	user := msgs[len(msgs)-1].(map[string]any)["content"].(string)
	if len(msgs) != 2 || !strings.Contains(user, "<result>\nLet's move the design review") || !strings.HasSuffix(user, "When is the review?") {
		t.Errorf("request %q", msgs)
	}
	got, err := a.lib.store.History().FollowUps(context.Background(), histInserted)
	if err != nil || len(got) != 1 || got[0].Question != "When is the review?" {
		t.Errorf("kept %+v, %v", got, err)
	}

	// The next question carries the chat; a Traditional answer comes in
	// Simplified characters.
	chat.set("會議改到線上。", 0)
	askIn(t, a, tt, "Where?")
	frames(tt)
	if len(f.list) != 2 || f.list[1].Answer != "会议改到线上。" {
		t.Fatalf("second: %+v", f.list)
	}
	msgs = chat.requests()[1]["messages"].([]any)
	if len(msgs) != 4 || msgs[2].(map[string]any)["content"] != "Thursday afternoon." {
		t.Errorf("second request %q", msgs)
	}

	// Leaving and coming back reads them again.
	click(t, tt, "Close")
	click(t, tt, "Let's move the design review to Thursday afternoon.")
	frames(tt)
	if len(f.list) != 2 || !has(tt, "会议改到线上。") {
		t.Errorf("read again: %+v", f.list)
	}

	click(t, tt, "Clear the follow-up questions")
	frames(tt)
	if got, _ := a.lib.store.History().FollowUps(context.Background(), histInserted); len(got) != 0 || len(f.list) != 0 {
		t.Errorf("cleared: %+v / %+v", got, f.list)
	}
}

func TestHistoryFollowUpFails(t *testing.T) {
	a, _, tt := newTestApp(t)
	seedHistory(t, a)
	chat := &fakeChat{}
	chat.set("", 400)
	a.local = withChat(readyLocal(), chat)
	tt.SetSize(1200, 1600) // the whole details panel in view
	a.navigate("/history")
	click(t, tt, "Let's move the design review to Thursday afternoon.")
	askIn(t, a, tt, "When?")
	frames(tt)
	f := &a.hist.ask
	if f.err == "" || !strings.HasPrefix(f.err, "No answer: ") || f.input != "When?" || len(f.list) != 0 {
		t.Errorf("after a failure: %+v", f)
	}
	if got, _ := a.lib.store.History().FollowUps(context.Background(), histInserted); len(got) != 0 {
		t.Errorf("kept %+v", got)
	}
}

func TestHistoryFollowUpNoModel(t *testing.T) {
	a, _, tt := newTestApp(t)
	seedHistory(t, a)
	a.local = &fakeLocal{ASRP: &fakeTranscriptions{}}
	tt.SetSize(1200, 1600) // the whole details panel in view
	a.navigate("/history")
	click(t, tt, "Let's move the design review to Thursday afternoon.")
	if !has(tt, "Rewrite has no language model to answer with.") || tt.HasText("Ask") {
		t.Errorf("without a model: %q", tt.Texts())
	}
}

// snapshotFollowUps saves the details of an entry with a recording that
// plays and two follow-up questions, the second on its way.
func snapshotFollowUps(a *app, tt *ui.Tester, save func(string)) {
	a.navigate("/history")
	ctx := context.Background()
	at := time.Now().Add(-time.Minute)
	f := []store.FollowUp{{Question: "When is the review?", Answer: "Thursday afternoon; the new agenda comes tonight.", At: at}}
	if i18n.Locale() == "zh-Hans" {
		f[0] = store.FollowUp{Question: "评审改到什么时候？", Answer: "周四下午；新的议程今晚发出。", At: at}
	}
	_ = a.lib.store.History().SetFollowUps(ctx, histInserted, f)
	was := a.local
	a.local = withChat(readyLocal(), &fakeChat{}) // Rewrite's model answers
	defer func() { a.local = was }()
	a.closeEntry()
	a.hist.sel = histInserted
	a.hist.play.open = func(string) (recPlayer, error) { return &fakeRec{pos: 3 * time.Second}, nil }
	a.toggleRecording(store.Entry{ID: histInserted, Audio: histInserted + ".wav"})
	a.loadFollowUps(store.Entry{ID: histInserted})
	if i18n.Locale() == "zh-Hans" {
		a.hist.ask.asking, a.hist.ask.partial = "谁来准备？", "保存的结果里没有"
	} else {
		a.hist.ask.asking, a.hist.ask.partial = "Who prepares it?", "The saved result does not"
	}
	save("history-detail-play")
	frames(tt)
	tt.Scroll(700, 300, 0, 370) // the details panel, to its end
	save("history-detail-followup")
	a.closeEntry()
	a.hist.play.open = nil
	_ = a.lib.store.History().SetFollowUps(ctx, histInserted, nil)
}
