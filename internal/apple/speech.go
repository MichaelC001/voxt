package apple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
)

// MaxHints is the most contextual strings sent (Apple's advice: about a
// hundred).
const MaxHints = 100

// openTimeout bounds the wait for the recogniser to start (the first use
// may show macOS's Speech Recognition prompt: the user answers it).
const openTimeout = 2 * time.Minute

// chunkBytes is the most audio one apple.speech.audio carries (1 s).
const chunkBytes = 2 * asr.SampleRate

// Speech is Apple Speech: a streamer over the helper's apple.speech.
type Speech struct {
	h Caller
}

var (
	_ asr.Streamer        = (*Speech)(nil)
	_ asr.FileTranscriber = (*Speech)(nil)
)

// Caps: live text and the dictionary's terms as contextual strings. Not
// File: a session streams to it; Transcribe (a history entry's retry)
// streams a whole recording too.
func (*Speech) Caps() asr.Caps { return asr.Caps{Stream: true, Hints: true} }

// Transcribe streams a whole recording (PCM at 16 kHz, or a WAV of it)
// as fast as the helper takes it.
func (s *Speech) Transcribe(ctx context.Context, req asr.FileRequest) (asr.Result, error) {
	pcm := req.Audio
	if req.Format == "wav" {
		var rate, channels int
		var err error
		if pcm, rate, channels, err = asr.ReadWAV(req.Audio); err != nil || rate != asr.SampleRate || channels != 1 {
			return asr.Result{}, &asr.Error{Kind: asr.KindAudio, Provider: Provider, Detail: fmt.Sprintf("a WAV of %d Hz, %d channels: %v", rate, channels, err)}
		}
	} else if req.SampleRate != 0 && req.SampleRate != asr.SampleRate {
		return asr.Result{}, &asr.Error{Kind: asr.KindAudio, Provider: Provider, Detail: fmt.Sprintf("audio at %d Hz, not %d", req.SampleRate, asr.SampleRate)}
	}
	return asr.StreamAll(ctx, s, asr.StreamRequest{Lang: req.Lang, Hints: req.Hints}, pcm, asr.Feed{})
}

// Open starts recognising req's language: it returns once the
// recogniser runs.
func (s *Speech) Open(ctx context.Context, req asr.StreamRequest) (asr.Stream, error) {
	if s.h == nil {
		return nil, &asr.Error{Kind: asr.KindConfig, Provider: Provider, Detail: "no helper", Err: ErrUnavailable}
	}
	if req.SampleRate != 0 && req.SampleRate != asr.SampleRate {
		return nil, &asr.Error{Kind: asr.KindAudio, Provider: Provider, Detail: fmt.Sprintf("audio at %d Hz, not %d", req.SampleRate, asr.SampleRate)}
	}
	loc := Locale(req.Lang)
	if loc == "" {
		return nil, &asr.Error{Kind: asr.KindConfig, Provider: Provider, Detail: fmt.Sprintf("no speech locale for %q", req.Lang), Err: ErrLanguage}
	}
	hints := req.Hints
	if len(hints) > MaxHints {
		hints = hints[:MaxHints]
	}
	jctx, cancel := context.WithCancel(ctx)
	st := &stream{h: s.h, job: jobID(), ctx: jctx, cancel: cancel,
		events: make(chan asr.Event, 64), started: make(chan struct{}), ended: make(chan struct{})}
	opened := make(chan error, 1)
	go st.run(proto.AppleSpeechParams{Locale: loc, Hints: hints}, opened)
	timer := time.NewTimer(openTimeout)
	defer timer.Stop()
	select {
	case <-st.started:
		return st, nil
	case err := <-opened:
		cancel()
		return nil, err
	case <-timer.C:
		st.Close()
		return nil, &asr.Error{Kind: asr.KindTimeout, Provider: Provider, Detail: "the recogniser did not start"}
	case <-ctx.Done():
		st.Close()
		return nil, asr.NetError(ctx, Provider, ctx.Err())
	}
}

// stream is one apple.speech job.
type stream struct {
	h      Caller
	job    string
	ctx    context.Context
	cancel context.CancelFunc

	events  chan asr.Event
	started chan struct{}
	ended   chan struct{} // closed when run is done (events closed)

	mu       sync.Mutex
	partial  string
	isOpen   bool
	finished bool
}

var (
	_ asr.Stream   = (*stream)(nil)
	_ asr.Salvager = (*stream)(nil)
)

// run is the job: its notifications become events, its answer the final
// event (or opened's error, before the recogniser started).
func (st *stream) run(p proto.AppleSpeechParams, opened chan<- error) {
	defer close(st.ended)
	defer close(st.events)
	start := time.Now()
	raw, err := st.h.Call(st.ctx, st.job, proto.MethodAppleSpeech, p, st.onNote)
	st.mu.Lock()
	wasOpen := st.isOpen
	st.mu.Unlock()
	if err != nil {
		err = speechErr(st.ctx, err)
		if !wasOpen {
			opened <- err
			return
		}
		st.emit(asr.Event{Kind: asr.EventError, Err: err})
		return
	}
	var res proto.AppleSpeechResult
	if err := json.Unmarshal(raw, &res); err != nil {
		st.emit(asr.Event{Kind: asr.EventError, Err: &asr.Error{Kind: asr.KindProtocol, Provider: Provider, Detail: err.Error()}})
		return
	}
	r := asr.Result{Text: res.Text, Lang: Language(res.Locale), Duration: time.Duration(res.AudioMs) * time.Millisecond, Model: SpeechID + " (" + res.Engine + ")", Latency: time.Since(start)}
	st.emit(asr.Event{Kind: asr.EventFinal, Text: res.Text, Result: r})
}

// onNote takes the job's notifications (on the helper's reader
// goroutine: it must not block).
func (st *stream) onNote(m proto.Message) {
	switch m.Method {
	case proto.MethodAppleStarted:
		st.mu.Lock()
		if !st.isOpen {
			st.isOpen = true
			close(st.started)
		}
		st.mu.Unlock()
	case proto.MethodApplePartial:
		var p proto.ApplePartialParams
		if m.DecodeParams(&p) != nil {
			return
		}
		st.mu.Lock()
		st.partial = p.Text
		st.mu.Unlock()
		select { // a partial is dropped rather than wait: the next one has all of it
		case st.events <- asr.Event{Kind: asr.EventPartial, Text: p.Text}:
		default:
		}
	}
}

// emit sends the last event, unless the stream was closed.
func (st *stream) emit(ev asr.Event) {
	select {
	case st.events <- ev:
	case <-st.ctx.Done():
		if ev.Kind == asr.EventError { // a cancel after Close: nobody reads
			return
		}
		select {
		case st.events <- ev:
		default:
		}
	}
}

// Send sends PCM16LE mono at 16 kHz, in pieces of at most a second.
func (st *stream) Send(pcm []byte) error {
	for len(pcm) > 0 {
		n := min(len(pcm), chunkBytes)
		n -= n % 2
		if n == 0 {
			return nil
		}
		if _, err := st.h.Call(st.ctx, "", proto.MethodAppleSpeechAudio, proto.AppleAudioParams{Job: st.job, Data: pcm[:n]}, nil); err != nil {
			return speechErr(st.ctx, err)
		}
		pcm = pcm[n:]
	}
	return nil
}

// Finish says the audio ended: the final text follows.
func (st *stream) Finish() error {
	st.mu.Lock()
	if st.finished {
		st.mu.Unlock()
		return nil
	}
	st.finished = true
	st.mu.Unlock()
	if _, err := st.h.Call(st.ctx, "", proto.MethodAppleSpeechFinish, proto.AppleJobParams{Job: st.job}, nil); err != nil {
		return speechErr(st.ctx, err)
	}
	return nil
}

// Close abandons the job (job.cancel) and waits for it to end.
func (st *stream) Close() {
	st.cancel()
	<-st.ended
}

func (st *stream) Events() <-chan asr.Event { return st.events }

// Salvage is the last partial text.
func (st *stream) Salvage() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.partial
}

// speechErr is a helper failure as asr's error.
func speechErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return asr.NetError(ctx, Provider, ctx.Err())
	}
	var pe *proto.Error
	if errors.As(err, &pe) {
		e := &asr.Error{Kind: asr.KindServer, Provider: Provider, Code: pe.Code, Detail: pe.Message, Err: pe}
		switch pe.Code {
		case proto.CodePermissionDenied:
			e.Kind, e.Err = asr.KindConfig, fmt.Errorf("%w: %w", ErrPermission, pe)
		case proto.CodeUnsupported:
			e.Kind, e.Err = asr.KindConfig, fmt.Errorf("%w: %w", ErrUnavailable, pe)
			if capability(pe) == proto.CapabilityLocale {
				e.Err = fmt.Errorf("%w: %w", ErrLanguage, pe)
			}
		case proto.CodeCancelled:
			e.Kind = asr.KindCancelled
		case proto.CodeBadRequest:
			e.Kind = asr.KindProtocol
		case proto.CodeMethodNotFound:
			e.Kind, e.Err = asr.KindConfig, fmt.Errorf("%w: %w", ErrUnavailable, pe)
		default:
			e.Retryable = true
		}
		return e
	}
	return &asr.Error{Kind: asr.KindServer, Provider: Provider, Detail: err.Error(), Retryable: errors.Is(err, core.ErrHelper), Err: err}
}

// jobID is a new job's id.
func jobID() string { return core.NewID(time.Now()) }

func jsonUnmarshal(raw []byte, v any) error { return json.Unmarshal(raw, v) }
