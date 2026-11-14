package main

// Playing an entry's recording in its details (history_detail.go): play,
// pause, how far it is. MyGo's audio package streams the WAV from the
// disk as it plays (about a second ahead), so a long recording costs no
// memory; one recording plays at a time, and leaving the entry, closing
// the panel or deleting it stops it.

import (
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/store"
)

// recPlayer is what plays a recording: MyGo's *audio.Player, a fake in
// tests. Safe from any goroutine.
type recPlayer interface {
	Play()
	Pause()
	Playing() bool
	Position() time.Duration
	Duration() time.Duration
	Close()
}

// recPlayback is the recording that plays (or is paused) in the details.
type recPlayback struct {
	id      string // the entry whose recording it is ("" none)
	p       recPlayer
	opening bool // the file is being opened
	ticking bool // a ticker redraws the progress
	// open opens a recording's file, paused; nil is openRecording. Tests
	// set it.
	open func(path string) (recPlayer, error)
}

// openRecording streams the WAV at path.
func openRecording(path string) (recPlayer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	p, err := audio.OpenStream(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return p, nil
}

// playing reports whether the recording of entry id plays now.
func (r *recPlayback) playing(id string) bool {
	return r.id == id && r.p != nil && r.p.Playing()
}

// stop closes the recording that plays, if any.
func (a *app) stopRecording() {
	r := &a.hist.play
	if r.p != nil {
		r.p.Close()
	}
	r.id, r.p, r.opening = "", nil, false
}

// toggleRecording plays e's recording, or pauses it while it plays.
func (a *app) toggleRecording(e store.Entry) {
	r := &a.hist.play
	if r.id == e.ID && r.p != nil {
		if r.p.Playing() {
			r.p.Pause()
		} else {
			r.p.Play()
			a.tickRecording()
		}
		return
	}
	a.stopRecording()
	r.id, r.opening = e.ID, true
	open := r.open
	if open == nil {
		open = openRecording
	}
	hs := a.lib.store.History()
	a.async(func() {
		p, err := open(hs.AudioPath(e.Audio)) // AudioPath reads the disk
		a.post(func() {
			if r.id != e.ID || !r.opening { // stopped, or another one, meanwhile
				if p != nil {
					p.Close()
				}
				return
			}
			r.opening = false
			if err != nil {
				r.id = ""
				a.toast(i18n.T("Could not play the recording: %v", err))
				return
			}
			r.p = p
			p.Play()
			a.tickRecording()
		})
	})
}

// tickRecording redraws the progress while the recording plays (only
// with a window: tests draw when they want).
func (a *app) tickRecording() {
	r := &a.hist.play
	if r.ticking || a.win == nil {
		return
	}
	r.ticking = true
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			done := make(chan bool, 1)
			a.post(func() {
				still := r.p != nil && r.p.Playing()
				if !still {
					r.ticking = false
				}
				done <- still
			})
			if !<-done {
				return
			}
		}
	}()
}

// recordingRow is the play button, the progress and the times of e's
// recording (an entry with one).
func (a *app) recordingRow(c *ui.Context, e store.Entry) {
	t := c.Theme()
	k := tok(t)
	r := &a.hist.play
	var pos, dur time.Duration
	if r.id == e.ID && r.p != nil {
		pos, dur = r.p.Position(), r.p.Duration()
	}
	if dur <= 0 {
		dur = time.Duration(e.AudioMS) * time.Millisecond
	}
	frac := 0.0
	if dur > 0 {
		frac = min(1, float64(pos)/float64(dur))
	}
	ui.Row(c.Key("hist-play")).Gap(px(t, sp3)).AlignItems(ui.Center).Children(func() {
		if playButton(c.Key("hist-play-btn"), r.playing(e.ID), false, 28).Clicked() && !r.opening {
			a.toggleRecording(e)
		}
		ui.Progress(c, frac).Label(i18n.T("Recording")).Grow(1).MinWidth(0)
		ui.Text(c, clockTime(pos)+" / "+clockTime(dur)).FontSize(px(t, fsCaption)).TextColor(k.Text2).Shrink(0)
	})
}

// clockTime is d as m:ss (h:mm:ss from an hour).
func clockTime(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
