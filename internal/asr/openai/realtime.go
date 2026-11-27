package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/hehehai/VoxtMini/internal/asr"
)

// settleGrace is how long Finish waits after the last known item completes
// before finalizing, so an in-flight server VAD commit cannot cut off a
// final-word commit arriving right behind it.
const defaultSettleGrace = 150 * time.Millisecond

// RealtimeClient streams audio to an OpenAI Realtime transcription
// WebSocket endpoint (wss://api.openai.com/v1/realtime?intent=transcription
// or a compatible custom/local gateway). Goroutine-safe.
type RealtimeClient struct {
	endpoint      string
	model         string
	key           string
	prompt        string
	http          *http.Client
	finishTimeout time.Duration
	settleGrace   time.Duration
}

var _ asr.Streamer = (*RealtimeClient)(nil)

// NewRealtime checks cfg and returns a Realtime WebSocket streamer.
// The endpoint must be wss (or ws to loopback / a private network);
// https/http endpoints with /realtime are converted to wss/ws. An API
// key is required for wss endpoints and optional for plain ws local
// gateways.
func NewRealtime(cfg asr.Config) (*RealtimeClient, error) {
	ep := RealtimeEndpoint(cfg.Endpoint)
	u, err := asr.CheckEndpoint(ID, ep, "wss")
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(cfg.Key)
	if key == "" && u.Scheme != "ws" {
		return nil, asr.Errorf(ID, asr.KindConfig, "no API key")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	client := cfg.HTTPClient
	if client == nil {
		client = asr.WSClient(cfg.Proxy)
	}
	return &RealtimeClient{
		endpoint:      ep,
		model:         model,
		key:           key,
		prompt:        strings.TrimSpace(cfg.Prompt),
		http:          client,
		finishTimeout: asr.FinalTimeout,
		settleGrace:   defaultSettleGrace,
	}, nil
}

// Caps reports a streamer that takes a prompt (dictionary hints fold
// into it).
func (c *RealtimeClient) Caps() asr.Caps {
	return asr.Caps{Stream: true, Prompt: true}
}

// Model is the transcription model the client asks for.
func (c *RealtimeClient) Model() string { return c.model }

// Endpoint is the normalised WebSocket URL the client connects to.
func (c *RealtimeClient) Endpoint() string { return c.endpoint }

// Open dials the WebSocket, sends transcription_session.update, and
// returns once the server confirms the session is ready.
func (c *RealtimeClient) Open(ctx context.Context, req asr.StreamRequest) (asr.Stream, error) {
	if req.SampleRate <= 0 {
		req.SampleRate = asr.SampleRate
	}
	h := http.Header{}
	if c.key != "" {
		h.Set("Authorization", "Bearer "+c.key)
	}
	h.Set("OpenAI-Beta", "realtime=v1")
	h.Set("User-Agent", "VoxtMini")

	conn, _, err := asr.Dial(ctx, ID, c.endpoint, asr.WSOptions{Client: c.http, Header: h})
	if err != nil {
		return nil, err
	}
	ft := c.finishTimeout
	if ft <= 0 {
		ft = asr.FinalTimeout
	}
	sg := c.settleGrace
	if sg <= 0 {
		sg = defaultSettleGrace
	}
	s := &rtStream{
		ctx:           ctx,
		conn:          conn,
		model:         c.model,
		rate:          req.SampleRate,
		finishTimeout: ft,
		settleGrace:   sg,
		resamp:        newResampler(req.SampleRate, realtimeSampleRate),
		events:        make(chan asr.Event, 32),
		startc:        make(chan error, 1),
		done:          make(chan struct{}),
		readerDone:    make(chan struct{}),
	}
	s.rechunk.Size = req.SampleRate * 2 / 10 // 100 ms of input PCM
	go s.read()

	prompt := Prompt(firstNonEmpty(req.Prompt, c.prompt), req.Hints)
	if err := conn.Write(ctx, websocket.MessageText, sessionUpdateMessage(c.model, req.Lang, prompt)); err != nil {
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
		return nil, &asr.Error{Kind: asr.KindNetwork, Provider: ID, Detail: "the service did not start the session in time", Retryable: true}
	case <-ctx.Done():
		s.Close()
		return nil, asr.NetError(ctx, ID, ctx.Err())
	}
}

type rtStream struct {
	ctx           context.Context
	conn          *websocket.Conn
	model         string
	rate          int
	finishTimeout time.Duration
	settleGrace   time.Duration
	text          rtTranscript
	events        chan asr.Event

	startc     chan error
	done       chan struct{}
	readerDone chan struct{}
	closeOnce  sync.Once

	rechunk    asr.Rechunk // Send's goroutine only
	resamp     *resampler  // Send's goroutine only
	sent       atomic.Int64
	finishing  atomic.Bool
	committed  atomic.Bool
	noAudio    atomic.Bool
	settled    atomic.Bool
	timedOut   atomic.Bool
	finishedAt atomic.Int64

	timerMu sync.Mutex
	timer   *time.Timer
	settle  *time.Timer
}

var _ asr.Salvager = (*rtStream)(nil)

func (s *rtStream) read() {
	defer close(s.readerDone)
	defer close(s.events)
	started := false
	commitAck := false
	commitEmpty := false
	fail := func(e error) {
		s.stopTimers()
		if !started {
			s.startc <- e
			return
		}
		s.emit(asr.Event{Kind: asr.EventError, Err: e})
	}
	last := ""
	emitPartial := func() bool {
		if !started {
			return true
		}
		if t := s.text.String(); t != last {
			last = t
			return s.emit(asr.Event{Kind: asr.EventPartial, Text: t})
		}
		return true
	}

	for {
		typ, data, err := s.conn.Read(s.ctx)
		if err != nil {
			if started && s.ctx.Err() == nil {
				if s.noAudio.Load() || s.settled.Load() {
					s.final()
					return
				}
				if s.timedOut.Load() {
					fail(&asr.Error{Kind: asr.KindNetwork, Provider: ID, Detail: "no final result in time", Retryable: true, Err: err})
					return
				}
				if s.committed.Load() {
					st := websocket.CloseStatus(err)
					if st == websocket.StatusNormalClosure || st == websocket.StatusGoingAway {
						s.final()
						return
					}
				}
			}
			fail(asr.WSError(s.ctx, ID, err))
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var ev rtServerEvent
		if json.Unmarshal(data, &ev) != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(ev.Type)) {
		case "transcription_session.updated", "session.updated":
			if !started {
				started = true
				s.startc <- nil
			}
		case "input_audio_buffer.committed":
			s.stopSettle()
			s.text.register(ev.ItemID, ev.PreviousItemID)
			if s.committed.Load() {
				commitAck = true
			}
		case "conversation.item.created":
			s.stopSettle()
			id := ev.ItemID
			if id == "" && ev.Item != nil {
				id = ev.Item.ID
			}
			s.text.register(id, ev.PreviousItemID)
		case "conversation.item.input_audio_transcription.delta":
			s.stopSettle()
			s.text.addDelta(ev.ItemID, ev.Delta)
			if !emitPartial() {
				return
			}
		case "conversation.item.input_audio_transcription.completed":
			wasKnown := s.text.known(ev.ItemID)
			s.text.complete(ev.ItemID, ev.Transcript)
			if !emitPartial() {
				return
			}
			if s.committed.Load() {
				if !wasKnown {
					commitAck = true
				}
				if !s.text.hasPending() {
					if commitEmpty {
						s.final()
						return
					}
					if commitAck {
						s.armSettle()
					}
				}
			}
		case "conversation.item.input_audio_transcription.failed":
			code, msg := asr.ErrorBody(data)
			if asr.NoSpeech(code, msg) {
				s.text.complete(ev.ItemID, "")
				if s.committed.Load() && !s.text.hasPending() {
					if commitEmpty {
						s.final()
						return
					}
					if commitAck {
						s.armSettle()
					}
				}
				continue
			}
			fail(realtimeError(code, msg, true))
			go s.conn.Close(websocket.StatusNormalClosure, "")
			return
		case "error":
			code, msg := asr.ErrorBody(data)
			if started && s.committed.Load() && (isCommitEmptyError(code, msg) || asr.NoSpeech(code, msg)) {
				commitAck = true
				commitEmpty = true
				if !s.text.hasPending() {
					s.final()
					return
				}
				continue
			}
			fail(realtimeError(code, msg, false))
			go s.conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}

func (s *rtStream) final() {
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

func (s *rtStream) emit(ev asr.Event) bool {
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

// Send frames pcm in 100 ms chunks, resamples to 24 kHz PCM16LE, and
// sends input_audio_buffer.append events.
func (s *rtStream) Send(pcm []byte) error {
	if s.finishing.Load() {
		return asr.Errorf(ID, asr.KindProtocol, "send after finish")
	}
	if err := s.closed(); err != nil {
		return err
	}
	for _, f := range s.rechunk.Add(pcm) {
		if err := s.writeAudio(f); err != nil {
			return err
		}
	}
	return nil
}

func (s *rtStream) writeAudio(frame []byte) error {
	out := s.resamp.process(frame)
	if len(out) == 0 {
		return nil
	}
	if err := s.conn.Write(s.ctx, websocket.MessageText, audioAppendMessage(out)); err != nil {
		return asr.WSError(s.ctx, ID, err)
	}
	s.sent.Add(int64(len(frame)))
	return nil
}

// Finish flushes any remaining buffered audio, sends
// input_audio_buffer.commit, and waits up to finishTimeout for pending
// transcriptions to complete.
func (s *rtStream) Finish() error {
	if !s.finishing.CompareAndSwap(false, true) {
		return nil
	}
	if err := s.closed(); err != nil {
		return err
	}
	if rest := s.rechunk.Rest(); len(rest) > 0 {
		if err := s.writeAudio(rest); err != nil {
			return err
		}
	}
	s.finishedAt.Store(time.Now().UnixNano())
	if s.sent.Load() == 0 {
		s.noAudio.Store(true)
		s.conn.CloseNow()
		return nil
	}
	s.committed.Store(true)
	if err := s.conn.Write(s.ctx, websocket.MessageText, audioCommitMessage()); err != nil {
		return asr.WSError(s.ctx, ID, err)
	}
	s.timerMu.Lock()
	s.timer = time.AfterFunc(s.finishTimeout, func() {
		s.timedOut.Store(true)
		s.conn.CloseNow()
	})
	s.timerMu.Unlock()
	return nil
}

func (s *rtStream) armSettle() {
	s.timerMu.Lock()
	if s.settle != nil {
		s.settle.Reset(s.settleGrace)
	} else {
		s.settle = time.AfterFunc(s.settleGrace, func() {
			s.settled.Store(true)
			s.conn.CloseNow()
		})
	}
	s.timerMu.Unlock()
}

func (s *rtStream) stopSettle() {
	s.timerMu.Lock()
	if s.settle != nil {
		s.settle.Stop()
		s.settle = nil
	}
	s.timerMu.Unlock()
}

func (s *rtStream) stopTimers() {
	s.timerMu.Lock()
	for _, t := range []*time.Timer{s.timer, s.settle} {
		if t != nil {
			t.Stop()
		}
	}
	s.timer, s.settle = nil, nil
	s.timerMu.Unlock()
}

func (s *rtStream) closed() error {
	select {
	case <-s.done:
		return asr.Errorf(ID, asr.KindCancelled, "the stream is closed")
	default:
		return nil
	}
}

// Salvage returns the text accumulated so far.
func (s *rtStream) Salvage() string { return s.text.String() }

// Close abandons the stream, closes the connection, and waits for
// Events to close.
func (s *rtStream) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.stopTimers()
		s.conn.CloseNow()
	})
	<-s.readerDone
}

func (s *rtStream) Events() <-chan asr.Event { return s.events }
