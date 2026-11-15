package main

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/permission"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/sounds"
)

// cueLog records the cues a session app plays.
type cueLog struct {
	mu    sync.Mutex
	names []string
}

func (l *cueLog) listen(a *sessionApp) {
	a.playSound = func(s sounds.Target) { l.mu.Lock(); l.names = append(l.names, s.Name); l.mu.Unlock() }
}

func (l *cueLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.names)
}

// TestSessionMicNeverStarted is 11-14's first activation after launch
// with the real helper's protocol: the microphone gives no audio (a cold
// audio engine, mic-start=), the user stops after 0.5 s. One failure:
// the notice says the microphone did not start, the failure's cue plays
// once and no stop cue before it; nothing is pasted or kept.
func TestSessionMicNeverStarted(t *testing.T) {
	set, keys, tr := fileSetup(t, "never", &fakeChat{text: "x"})
	ev := script(t, hotkeyLine(0, "transcribe/0", proto.PhaseTrigger), hotkeyLine(500, "transcribe/0", proto.PhaseTrigger))
	a := newSessionApp(t, "mic-start=5000,"+ev, set, keys)
	var cues cueLog
	cues.listen(a)
	a.pump("the end", func() bool { return a.sess.State.Ended() })
	a.mute.Wait() // the failure's cue comes from the mute's goroutine, posted
	a.pump("the failure's cue", func() bool { return len(cues.get()) >= 2 })
	if a.sess.State != session.Failed || a.sess.Problem == nil || a.sess.Problem.Code != session.ProblemMicStart {
		t.Fatalf("end %+v", a.sess)
	}
	if a.notice == nil || a.notice.text != "The microphone did not start. Try again." || a.ov.mode != ovNotice {
		t.Errorf("notice %+v, overlay %s", a.notice, a.ov.mode)
	}
	if want := []string{a.cueTarget(startCue).Name, a.cueTarget(failCue).Name}; !slices.Equal(cues.get(), want) {
		t.Errorf("cues %q, want %q", cues.get(), want)
	}
	if calls, _ := tr.uploads(); calls != 0 || len(a.pasted()) != 0 || len(a.history()) != 0 {
		t.Errorf("uploads %d, pasted %+v, history %+v", calls, a.pasted(), a.history())
	}
}

// TestSessionMicSlowStart: a microphone that starts late (0.6 s) still
// records the rest; the session works.
func TestSessionMicSlowStart(t *testing.T) {
	set, keys, _ := fileSetup(t, "hello there", &fakeChat{text: "Hello there."})
	ev := script(t, hotkeyLine(0, "transcribe/0", proto.PhaseTrigger), hotkeyLine(1500, "transcribe/0", proto.PhaseTrigger))
	a := newSessionApp(t, "mic-start=600,"+ev, set, keys)
	waited := false
	a.pump("the end", func() bool {
		waited = waited || a.sess.Waiting == session.WaitMic && a.ov.mode == ovRecording
		return a.sess.State.Ended()
	})
	if a.sess.State != session.Done || a.sess.Result != "Hello there." {
		t.Fatalf("end %+v", a.sess)
	}
	if !waited {
		t.Error("never showed the microphone starting")
	}
}

// TestSessionMicPrompt: the microphone's permission not determined, the
// first session says macOS asks (the pill's line), records once allowed,
// and the next one does not say it again.
func TestSessionMicPrompt(t *testing.T) {
	set, keys, _ := fileSetup(t, "hello there", &fakeChat{text: "Hello there."})
	ev := script(t, hotkeyLine(0, "transcribe/0", proto.PhaseTrigger), hotkeyLine(1200, "transcribe/0", proto.PhaseTrigger))
	a := newSessionApp(t, "mic=notDetermined,mic-start=300,"+ev, func(s *settings.Settings) { set(s) }, keys)
	asked := false
	a.pump("the end", func() bool {
		if a.sess.Waiting == session.WaitPermission {
			text, hint := pillLine(a.sess)
			asked = asked || hint && text == "Allow the microphone in the macOS dialog"
		}
		return a.sess.State.Ended()
	})
	if a.sess.State != session.Done || !asked {
		t.Fatalf("end %+v, asked %v", a.sess, asked)
	}
	if a.perms.mic != permission.Granted {
		t.Errorf("microphone %v after the first audio", a.perms.mic)
	}
}

// slowOutput is a Mac output that starts its sounds late (a cold engine).
type slowOutput struct {
	delay   time.Duration
	mu      sync.Mutex
	stopped int
}

func (o *slowOutput) Load(string, []byte) error { return nil }
func (o *slowOutput) Play(string, float64) (sounds.Voice, error) {
	time.Sleep(o.delay)
	return slowVoice{o}, nil
}
func (o *slowOutput) PlaySystem(string, float64) (sounds.Voice, error) { return o.Play("", 0) }

type slowVoice struct{ o *slowOutput }

func (v slowVoice) Stop() { v.o.mu.Lock(); v.o.stopped++; v.o.mu.Unlock() }

// TestLateCueDropped: a cue the output starts later than cueLate is
// stopped at once (11-14: the start cue, held up by a cold output, would
// sound when the recording ends, beside the stop cue); a prompt one plays.
func TestLateCueDropped(t *testing.T) {
	a, _, _ := newTestApp(t)
	for _, tc := range []struct {
		delay   time.Duration
		stopped int
	}{{10 * time.Millisecond, 0}, {cueLate + 100*time.Millisecond, 1}} {
		out := &slowOutput{delay: tc.delay}
		a.cuePlayer = sounds.NewPlayer(a.soundCat, out)
		a.playCueWait(sounds.Target{Kind: sounds.KindSystem, Name: "Pop"})
		if out.stopped != tc.stopped {
			t.Errorf("%v late: stopped %d times", tc.delay, out.stopped)
		}
	}
}
