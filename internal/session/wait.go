package session

import (
	"fmt"
	"time"
)

// Wait is what a session waits for before its recording works in full
// (Snapshot.Waiting).
type Wait string

const (
	WaitNone Wait = ""
	// WaitMic: the microphone has given no audio yet (capture.started
	// has not come): what is said meanwhile is not recorded. It takes
	// 150–400 ms; 5.9 s once on 10-20, while three models loaded.
	WaitMic Wait = "mic"
	// WaitModel: the speech model is still starting (its stream is not
	// open, or the services are not resolved): the recording goes on and
	// is sent to it once it is ready, so the live text comes late. Also
	// after Stop, while recognition waits for it.
	WaitModel Wait = "model"
	// WaitPermission: macOS asks the user for the microphone (its
	// permission was not determined when the session started,
	// StartRequest.MicPrompt): the recording starts once they answer.
	// Said at once, and the microphone has no time limit meanwhile.
	WaitPermission Wait = "permission"
)

// slowMic is a microphone start worth a warning in the log.
const slowMic = time.Second

// waitOf is what s waits for: macOS's microphone prompt at once (until
// the first audio); otherwise nothing before Config.WaitNote passed (a
// short wait is not worth a word), the microphone while recording
// without audio, then the speech model while it starts (r.opening).
func (r *run) waitOf(s *Snapshot) Wait {
	if r.req.MicPrompt && s.State == Recording && s.HeardAt.IsZero() {
		return WaitPermission
	}
	if !r.waitDue.Load() {
		return WaitNone
	}
	switch {
	case s.State == Recording && s.HeardAt.IsZero():
		return WaitMic
	case (s.State == Recording || s.State == Recognizing) && r.opening.Load():
		return WaitModel
	}
	return WaitNone
}

// noteWait updates Snapshot.Waiting.
func (r *run) noteWait() {
	r.m.update(r, func(s *Snapshot) {
		if w := r.waitOf(s); w != s.Waiting {
			s.Waiting = w
		}
	})
}

// setOpening records whether the speech model is still starting, and
// what the snapshot waits for.
func (r *run) setOpening(v bool) {
	if r.opening.Swap(v) != v {
		r.noteWait()
	}
}

// waitTimer fires once Config.WaitNote has passed since the session
// started (nil channel when it has).
func (r *run) waitTimer(started time.Time) <-chan time.Time {
	left := r.m.cfg.WaitNote - r.now().Sub(started)
	if left <= 0 {
		r.waitDue.Store(true)
		return nil
	}
	return time.After(left)
}

// micTimer fires when the microphone has had Config.MicStartLimit to
// give its first audio; nil while macOS asks for it (the user answers
// in their own time).
func (r *run) micTimer() <-chan time.Time {
	if r.req.MicPrompt {
		return nil
	}
	return time.After(r.m.cfg.MicStartLimit)
}

// heardAudio marks the microphone's first audio (onAudio, on the
// helper's reader goroutine).
func (r *run) heardAudio() {
	if !r.micOn.Swap(true) {
		close(r.heardCh)
	}
}

// micProblem is a recording that never got audio: the microphone did
// not start within the limit, or the user stopped before it did.
func (r *run) micProblem(stopped bool) *Problem {
	after := r.now().Sub(r.m.Current().StartedAt).Round(time.Millisecond)
	err := fmt.Errorf("no audio from the microphone %v after the start", after)
	if stopped {
		err = fmt.Errorf("stopped %v after the start, before the microphone gave any audio", after)
	}
	return &Problem{Code: ProblemMicStart, Err: err}
}
