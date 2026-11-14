package gemini

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/hehehai/VoxtMini/internal/asr"
)

// drainQuiet ends the wait for the final text once the service has said
// nothing for this long after audioStreamEnd: the live API documents no
// session-end event, and a pause in speech ends a turn anyway (Voxt waits
// 2.5 s after audioStreamEnd). A test shortens it.
var drainQuiet = 2500 * time.Millisecond

// transcript is the text so far: the settled segments and the draft of
// the current one. Goroutine-safe (Salvage reads it).
type transcript struct {
	mu        sync.Mutex
	committed []string
	interim   string
}

func (t *transcript) commit(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n := len(t.committed); s != "" && (n == 0 || t.committed[n-1] != s) {
		t.committed = append(t.committed, s)
	}
	t.interim = ""
}

func (t *transcript) setInterim(s string) {
	t.mu.Lock()
	t.interim = s
	t.mu.Unlock()
}

// String is the committed segments and the draft, joined.
func (t *transcript) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return join(append(append([]string(nil), t.committed...), t.interim))
}

// stream is an open session. The reader goroutine is the only sender on
// events.
type stream struct {
	ctx    context.Context
	conn   *websocket.Conn
	model  string
	rate   int
	text   transcript
	events chan asr.Event

	startc     chan error    // the reader's verdict on the setup (buffered)
	done       chan struct{} // closed by Close
	readerDone chan struct{}
	closeOnce  sync.Once

	rechunk    asr.Rechunk // Send's goroutine only
	sent       atomic.Int64
	finishing  atomic.Bool  // Finish was called
	ended      atomic.Bool  // audioStreamEnd went out
	drained    atomic.Bool  // the drain ended the wait: final with the text so far
	finishedAt atomic.Int64 // unix nanoseconds of Finish

	timerMu sync.Mutex
	quiet   *time.Timer // drainQuiet after the last message
	limit   *time.Timer // asr.FinalTimeout after Finish
}

func open(ctx context.Context, c *Client, req asr.StreamRequest) (*stream, error) {
	conn, _, err := asr.Dial(ctx, ID, c.connectURL(), asr.WSOptions{Client: c.http})
	if err != nil {
		return nil, redact(err, c.key)
	}
	s := &stream{
		ctx: ctx, conn: conn, model: c.model, rate: req.SampleRate,
		events:     make(chan asr.Event, 32),
		startc:     make(chan error, 1),
		done:       make(chan struct{}),
		readerDone: make(chan struct{}),
	}
	s.rechunk.Size = req.SampleRate * 2 / 10 // 100 ms
	go s.read()
	if err := conn.Write(ctx, websocket.MessageText, setupMessage(c.model, req)); err != nil {
		e := asr.WSError(ctx, ID, err)
		s.Close()
		return nil, e
	}
	timer := time.NewTimer(asr.ConnectTimeout)
	defer timer.Stop()
	select {
	case err := <-s.startc:
		if err != nil {
			s.Close()
			return nil, err
		}
		return s, nil
	case <-timer.C:
		s.Close()
		return nil, &asr.Error{Kind: asr.KindNetwork, Provider: ID, Detail: "the service did not complete the setup in time", Retryable: true}
	case <-ctx.Done():
		s.Close()
		return nil, asr.NetError(ctx, ID, ctx.Err())
	}
}

// read reads the service's messages until the end: it answers the setup
// on startc, then sends partials and the one final or error on events.
func (s *stream) read() {
	defer close(s.readerDone)
	defer close(s.events)
	started := false
	fail := func(e error) {
		if !started {
			s.startc <- e
			return
		}
		s.emit(asr.Event{Kind: asr.EventError, Err: e})
	}
	last := ""
	for {
		typ, data, err := s.conn.Read(s.ctx)
		if err != nil {
			if started && s.ctx.Err() == nil && (s.drained.Load() || s.ended.Load() && s.normalEnd(err)) {
				s.final()
				return
			}
			fail(readError(s.ctx, err))
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		m, ok := parse(data)
		if !ok {
			continue
		}
		if m.err != nil {
			fail(m.err)
			go s.conn.Close(websocket.StatusNormalClosure, "")
			return
		}
		if m.setupComplete && !started {
			started = true
			s.startc <- nil
			continue
		}
		if !started {
			continue
		}
		if m.final != "" {
			s.text.commit(m.final)
		} else if m.interim != "" {
			s.text.setInterim(m.interim)
		}
		if t := s.text.String(); t != last {
			last = t
			if !s.emit(asr.Event{Kind: asr.EventPartial, Text: t}) {
				return
			}
		}
		// A turn ends on every pause: only a terminal event after our
		// own audioStreamEnd ends the session.
		if s.ended.Load() {
			if m.turnComplete || m.goAway {
				s.final()
				return
			}
			s.resetQuiet()
		}
	}
}

// normalEnd: after audioStreamEnd, the socket's end is the session's
// end (Google closes it once the stream is over) when the text is in or
// the close was a normal one.
func (s *stream) normalEnd(err error) bool {
	if s.text.String() != "" {
		return true
	}
	st := websocket.CloseStatus(err)
	return st == websocket.StatusNormalClosure || st == websocket.StatusGoingAway
}

// final sends the final event with the text so far and closes.
func (s *stream) final() {
	s.stopTimers()
	text := s.text.String()
	r := asr.Result{
		Text:     text,
		Duration: time.Duration(s.sent.Load()/2) * time.Second / time.Duration(s.rate),
		Model:    s.model,
	}
	if at := s.finishedAt.Load(); at != 0 {
		r.Latency = time.Since(time.Unix(0, at))
	}
	s.emit(asr.Event{Kind: asr.EventFinal, Text: text, Result: r})
	go s.conn.Close(websocket.StatusNormalClosure, "")
}

// emit sends ev unless the stream was closed; it reports whether it did.
func (s *stream) emit(ev asr.Event) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case s.events <- ev:
		return true
	case <-s.done:
		return false
	}
}

// readError classifies a read error; a close frame's reason corrects the
// kind (Google closes with 1007/1008 and the reason for a bad key or an
// unknown model).
func readError(ctx context.Context, err error) *asr.Error {
	e := asr.WSError(ctx, ID, err)
	var ce websocket.CloseError
	if ctx.Err() == nil && errors.As(err, &ce) && ce.Reason != "" &&
		ce.Code != websocket.StatusNormalClosure && ce.Code != websocket.StatusGoingAway {
		reclassify(e, ce.Reason)
	}
	return e
}

// Send frames pcm in 100 ms frames and sends the full ones.
func (s *stream) Send(pcm []byte) error {
	if s.finishing.Load() {
		return asr.Errorf(ID, asr.KindProtocol, "send after finish")
	}
	if err := s.closed(); err != nil {
		return err
	}
	for _, f := range s.rechunk.Add(pcm) {
		if err := s.write(f); err != nil {
			return err
		}
	}
	return nil
}

func (s *stream) write(frame []byte) error {
	if err := s.conn.Write(s.ctx, websocket.MessageText, audioMessage(frame, s.rate)); err != nil {
		return asr.WSError(s.ctx, ID, err)
	}
	s.sent.Add(int64(len(frame)))
	return nil
}

// Finish sends what is left of the audio and audioStreamEnd, then waits
// for a terminal event, the socket's end, drainQuiet of silence or at
// most asr.FinalTimeout: the final text is the text so far then.
func (s *stream) Finish() error {
	if !s.finishing.CompareAndSwap(false, true) {
		return nil
	}
	if err := s.closed(); err != nil {
		return err
	}
	if rest := s.rechunk.Rest(); len(rest) > 0 {
		if err := s.write(rest); err != nil {
			return err
		}
	}
	s.finishedAt.Store(time.Now().UnixNano())
	if err := s.conn.Write(s.ctx, websocket.MessageText, endMessage()); err != nil {
		return asr.WSError(s.ctx, ID, err)
	}
	s.ended.Store(true)
	s.timerMu.Lock()
	s.quiet = time.AfterFunc(drainQuiet, s.drain)
	s.limit = time.AfterFunc(asr.FinalTimeout, s.drain)
	s.timerMu.Unlock()
	return nil
}

// drain ends the wait: the reader sees the closed socket and sends the
// final text.
func (s *stream) drain() {
	s.drained.Store(true)
	s.conn.CloseNow()
}

func (s *stream) resetQuiet() {
	s.timerMu.Lock()
	if s.quiet != nil {
		s.quiet.Reset(drainQuiet)
	}
	s.timerMu.Unlock()
}

func (s *stream) stopTimers() {
	s.timerMu.Lock()
	for _, t := range []*time.Timer{s.quiet, s.limit} {
		if t != nil {
			t.Stop()
		}
	}
	s.timerMu.Unlock()
}

func (s *stream) closed() error {
	select {
	case <-s.done:
		return asr.Errorf(ID, asr.KindCancelled, "the stream is closed")
	default:
		return nil
	}
}

// Salvage is the text so far.
func (s *stream) Salvage() string { return s.text.String() }

// Close abandons the stream: no final event, the connection closed, and
// Events closed when it returns.
func (s *stream) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.stopTimers()
		s.conn.CloseNow()
	})
	<-s.readerDone
}

func (s *stream) Events() <-chan asr.Event { return s.events }

var _ asr.Salvager = (*stream)(nil)

// redact removes the key from a handshake's error: a failed dial's error
// names the URL, query included.
func redact(err error, key string) error {
	var e *asr.Error
	if !errors.As(err, &e) {
		return &redacted{msg: scrub(err.Error(), key), err: err}
	}
	x := *e
	x.Detail = scrub(x.Detail, key)
	if x.Err != nil {
		x.Err = &redacted{msg: scrub(x.Err.Error(), key), err: x.Err}
	}
	return &x
}

func scrub(s, key string) string {
	for _, k := range []string{key, url.QueryEscape(key)} {
		if k != "" {
			s = strings.ReplaceAll(s, k, "REDACTED")
		}
	}
	return s
}

// redacted is an error with its message scrubbed of the key. It does not
// unwrap: the cause's message has the key (the *asr.Error's Kind is the
// classification callers use).
type redacted struct {
	msg string
	err error
}

func (r *redacted) Error() string { return r.msg }
