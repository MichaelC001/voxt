package main

// The UI's idle costs (audit #37, WP-D): log lines redraw only shown
// logs, the Local models page stops measuring while the window hides, and
// apps' icons go when no list shows them.

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/appicons"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
)

// TestLogLinesRedrawOnlyShownLogs: lines that arrive while another page
// shows post nothing; on the logs page (or with the panel open) they do,
// and the window's hiding stops them again.
func TestLogLinesRedrawOnlyShownLogs(t *testing.T) {
	a, _, tt := newTestApp(t)
	var posts atomic.Int32
	a.postQueue = func(fn func()) { posts.Add(1); fn() }
	line := func() {
		a.logs.Append(core.LogEntry{Time: time.Now(), Level: core.SeverityInfo, Source: "app", Message: "a line"})
	}
	a.navigate("/home")
	frames(tt)
	posts.Store(0)
	for range 50 {
		line()
	}
	if n := posts.Load(); n != 0 {
		t.Errorf("%d posts for lines on Home", n)
	}
	a.navigate("/logs")
	frames(tt)
	posts.Store(0)
	line()
	if n := posts.Load(); n != 1 {
		t.Errorf("%d posts for a line on the logs page", n)
	}
	a.navigate("/home")
	a.logsPanel = true
	frames(tt)
	posts.Store(0)
	line()
	if n := posts.Load(); n != 1 {
		t.Errorf("%d posts for a line with the panel open", n)
	}
	a.windowAway()
	frames(tt) // a build while hidden keeps them off
	posts.Store(0)
	line()
	if n := posts.Load(); n != 0 {
		t.Errorf("%d posts with the window hidden", n)
	}
	a.windowBack()
	frames(tt) // shown again: the build reads them
	if !a.logsOnScreen.Load() {
		t.Error("the panel shows, lines do not redraw")
	}
}

// TestMemTickStopsWhileHidden: the Local models page measures every
// memPoll while the window shows; hidden, the tick stops and sets no
// timer; shown again, it measures at once and ticks again.
func TestMemTickStopsWhileHidden(t *testing.T) {
	a, _, tt := newTestApp(t)
	timers := (&fakeTimers{}).install(a)
	shown := true
	a.winShown = func() bool { return shown }
	a.navigate(modelsPath)
	frames(tt)
	if !a.memPolling {
		t.Fatal("not measuring on the page")
	}
	shown = false
	timers.advance(memPoll)
	if a.memPolling {
		t.Error("still measuring while hidden")
	}
	added := timers.added
	timers.advance(10 * memPoll)
	if timers.added != added {
		t.Errorf("%d timers set while hidden", timers.added-added)
	}
	shown = true
	a.windowBack()
	if !a.memPolling {
		t.Error("shown again, not measuring")
	}
	a.navigate("/home")
	a.windowAway()
	a.windowBack()
	timers.advance(memPoll)
	if a.memPolling {
		t.Error("measuring on Home")
	}
}

// TestIconsDroppedOffPage: leaving App enhancement frees the icons'
// bitmaps; the next visit asks for them again; a batch that comes back
// after the drop is not kept.
func TestIconsDroppedOffPage(t *testing.T) {
	a, fs, tt := aeTester(t)
	snapshotIcons(fs, "")
	a.setRules(sampleRules())
	a.aePage.sel = "g-chat"
	frames(tt)
	slack := proto.AppRef{BundleID: "com.tinyspeck.slackmacgap"}
	if b, _ := a.appIconBitmap(slack, false); b == nil {
		t.Fatal("no icon on the page")
	}
	a.navigate("/home")
	frames(tt)
	if len(a.iconBmps) != 0 {
		t.Errorf("%d bitmaps kept off the page", len(a.iconBmps))
	}
	if _, st := a.appIconBitmap(slack, false); st != appicons.Unknown {
		t.Errorf("state %v after the drop", st)
	}
	a.navigate("/app-enhancement")
	a.aePage.sel = "g-chat"
	frames(tt)
	if b, _ := a.appIconBitmap(slack, false); b == nil {
		t.Error("the icon did not come back")
	}
}
