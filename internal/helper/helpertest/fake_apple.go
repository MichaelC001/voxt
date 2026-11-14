package helpertest

// The fake helper's apple.* methods: AppleSim (apple.go) behind the
// protocol, its jobs on goroutines of their own (job.cancel ends them).

import (
	"context"
	"time"

	"github.com/hehehai/VoxtMini/internal/helper/proto"
)

// newApple sets up the pretend Apple models from the options: none
// unless "apple" (so that the other tests' Macs have only the models
// they install).
func (f *fake) newApple() {
	f.apple = NewAppleSim()
	f.appleJobs = map[string]context.CancelFunc{}
	if !f.has("apple") {
		f.apple.Engine, f.apple.Intelligence = "", proto.AppleReasonUnsupportedOS
	}
	switch v := f.opts["apple-speech"]; v {
	case "none":
		f.apple.Engine = ""
	case proto.AppleEngineAnalyzer, proto.AppleEngineRecognizer:
		f.apple.Engine = v
	}
	if v := f.opts["apple-ai"]; v != "" {
		f.apple.Intelligence = v
	}
	if v := f.opts["speech-perm"]; v != "" {
		f.apple.SpeechPerm = v
	}
}

// serveApple answers an apple.* request; a job runs on its own goroutine.
func (f *fake) serveApple(m proto.Message) {
	if f.has("no-apple") {
		f.fail(m.ID, proto.CodeMethodNotFound, "Unknown method %s", m.Method)
		return
	}
	var params any
	if p := proto.ParamsFor(m.Method); p != nil {
		if err := m.DecodeParams(p); err != nil {
			f.fail(m.ID, proto.CodeBadRequest, "%v", err)
			return
		}
		params = p
	}
	call := func(ctx context.Context) {
		raw, err := f.apple.Call(ctx, m.ID, m.Method, params, func(n proto.Message) { f.send(n) })
		if err != nil {
			if pe, ok := err.(*proto.Error); ok {
				f.send(proto.NewError(m.ID, pe))
			} else {
				f.fail(m.ID, proto.CodeInternal, "%v", err)
			}
			return
		}
		f.send(proto.Message{ID: m.ID, Result: raw})
	}
	if !proto.IsJob(m.Method) {
		call(context.Background()) // in order, on the reader: the audio's pieces keep theirs
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.mu.Lock()
	f.appleJobs[m.ID] = cancel
	f.mu.Unlock()
	go func() {
		call(ctx)
		f.mu.Lock()
		delete(f.appleJobs, m.ID)
		f.mu.Unlock()
		cancel()
	}()
}

// cancelApple is job.cancel of an apple.* job (false: not one).
func (f *fake) cancelApple(id, job string) bool {
	f.mu.Lock()
	cancel, ok := f.appleJobs[job]
	f.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	go func() {
		for {
			f.mu.Lock()
			_, running := f.appleJobs[job]
			f.mu.Unlock()
			if !running {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
		f.result(id, proto.CancelResult{Cancelled: true})
	}()
	return true
}
