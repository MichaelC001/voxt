package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRec is a recording that plays as the test says.
type fakeRec struct {
	playing bool
	pos     time.Duration
	closed  bool
}

func (f *fakeRec) Play()                   { f.playing = true }
func (f *fakeRec) Pause()                  { f.playing = false }
func (f *fakeRec) Playing() bool           { return f.playing }
func (f *fakeRec) Position() time.Duration { return f.pos }
func (f *fakeRec) Duration() time.Duration { return 7400 * time.Millisecond }
func (f *fakeRec) Close()                  { f.closed, f.playing = true, false }

func TestHistoryPlayback(t *testing.T) {
	a, _, tt := newTestApp(t)
	seedHistory(t, a)
	var opened []string
	var recs []*fakeRec
	a.hist.play.open = func(path string) (recPlayer, error) {
		opened = append(opened, path)
		r := &fakeRec{}
		recs = append(recs, r)
		return r, nil
	}
	a.navigate("/history")
	click(t, tt, "Let's move the design review to Thursday afternoon.")
	if !has(tt, "0:00 / 0:07") {
		t.Errorf("no length before playing: %q", tt.Texts())
	}
	click(t, tt, "Play")
	if want := filepath.Join(a.lib.store.History().AudioDir(), histInserted+".wav"); len(opened) != 1 || opened[0] != want {
		t.Fatalf("opened %v, want %s", opened, want)
	}
	r := recs[0]
	if !r.playing || !a.hist.play.playing(histInserted) {
		t.Fatal("not playing")
	}
	r.pos = 3 * time.Second
	frames(tt)
	if !has(tt, "0:03 / 0:07") || !has(tt, "Pause") {
		t.Errorf("while playing: %q", tt.Texts())
	}
	click(t, tt, "Pause")
	if r.playing || r.closed {
		t.Errorf("Pause: playing %v closed %v", r.playing, r.closed)
	}
	click(t, tt, "Play") // goes on, the same file
	if len(opened) != 1 || !r.playing {
		t.Errorf("resume opened %v, playing %v", opened, r.playing)
	}

	// Another entry stops it; so does leaving the page.
	click(t, tt, "明天上午十点的站会改到线上，链接我稍后发到群里。")
	frames(tt)
	if !r.closed || a.hist.play.id != "" {
		t.Errorf("another entry: closed %v, id %q", r.closed, a.hist.play.id)
	}
	click(t, tt, "Let's move the design review to Thursday afternoon.")
	click(t, tt, "Play")
	a.navigate("/home")
	if !recs[1].closed {
		t.Error("still playing after leaving History")
	}
}

func TestHistoryPlaybackError(t *testing.T) {
	a, _, tt := newTestApp(t)
	seedHistory(t, a)
	a.hist.play.open = func(string) (recPlayer, error) { return nil, errors.New("no such file") }
	a.navigate("/history")
	click(t, tt, "Let's move the design review to Thursday afternoon.")
	click(t, tt, "Play")
	const msg = "Could not play the recording: no such file"
	if a.hist.play.id != "" || (!strings.Contains(strings.Join(a.toasts, "\n"), msg) && !has(tt, msg)) {
		t.Errorf("id %q, toasts %q", a.hist.play.id, a.toasts)
	}
}

func TestClockTime(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "0:00", 7400 * time.Millisecond: "0:07", 75 * time.Second: "1:15", 3725 * time.Second: "1:02:05"} {
		if got := clockTime(d); got != want {
			t.Errorf("clockTime(%v) = %q, want %q", d, got, want)
		}
	}
}
