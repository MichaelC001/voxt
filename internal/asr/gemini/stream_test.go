package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/asr/asrtest"
)

// testKey has characters a URL escapes, to check both forms stay hidden.
const testKey = "AIza-test/key+="

func client(t *testing.T, url string) *Client {
	t.Helper()
	c, err := New(asr.Config{Endpoint: url, Key: testKey, Model: "gemini-test-live"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// serve starts a fake Live API whose connections run handle and returns a
// client of it.
func serve(t *testing.T, handle func(c *asrtest.Conn)) *Client {
	return client(t, asrtest.WSServer(t, "", handle))
}

// handshake reads the setup and answers setupComplete; it returns the setup.
func handshake(c *asrtest.Conn) map[string]any {
	_, data := c.Read()
	var m map[string]any
	json.Unmarshal(data, &m)
	c.WriteText(`{"setupComplete":{}}`)
	return m
}

// readUntilEnd reads the client's audio until audioStreamEnd and returns
// the PCM bytes it got.
func readUntilEnd(c *asrtest.Conn) int {
	n := 0
	for {
		_, data, ok := c.TryRead()
		if !ok {
			return n
		}
		var m audioMsg
		var raw struct {
			RealtimeInput struct {
				Audio struct{ Data string } `json:"audio"`
			} `json:"realtimeInput"`
		}
		json.Unmarshal(data, &m)
		json.Unmarshal(data, &raw)
		if m.RealtimeInput.AudioStreamEnd {
			return n
		}
		pcm, _ := base64.StdEncoding.DecodeString(raw.RealtimeInput.Audio.Data)
		n += len(pcm)
	}
}

// next waits for the next event.
func next(t *testing.T, s asr.Stream) (asr.Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-s.Events():
		return ev, ok
	case <-time.After(5 * time.Second):
		t.Fatal("no event in time")
		return asr.Event{}, false
	}
}

// rest collects the events up to the stream's end.
func rest(t *testing.T, s asr.Stream) (partials []string, last asr.Event) {
	t.Helper()
	for {
		ev, ok := next(t, s)
		if !ok {
			return partials, last
		}
		if ev.Kind == asr.EventPartial {
			partials = append(partials, ev.Text)
		}
		last = ev
	}
}

func TestStream(t *testing.T) {
	got := make(chan int, 1)
	c := serve(t, func(c *asrtest.Conn) {
		if c.Req.URL.Path != defaultPath || c.Req.URL.Query().Get("key") != testKey || c.Req.Header.Get("Authorization") != "" {
			t.Errorf("handshake: path %q query %q auth %q", c.Req.URL.Path, c.Req.URL.RawQuery, c.Req.Header.Get("Authorization"))
		}
		setup := handshake(c)
		b, _ := json.Marshal(setup)
		if want := `{"setup":{"generationConfig":{"responseModalities":["TEXT"]},"inputAudioTranscription":{"customVocabulary":["VoxtMini"],"languageCodes":["en","zh"],"mode":"SMART"},"model":"models/gemini-test-live"}}`; string(b) != want {
			t.Errorf("setup %s", b)
		}
		c.Read() // the first frame
		c.WriteText(`{"serverContent":{"interimInputTranscription":{"text":"hel"}}}`)
		c.WriteText(`{"serverContent":{"inputTranscription":{"text":"hello"}}}`)
		c.WriteText(`{"serverContent":{"turnComplete":true}}`) // a pause: keeps going
		c.WriteText(`{"server_content":{"interim_input_transcription":{"text":"wor"}}}`)
		got <- 3200 + readUntilEnd(c)
		c.WriteText(`{"serverContent":{"inputTranscription":{"text":"world"}}}`)
		c.WriteText(`{"serverContent":{"inputTranscription":{"text":"你好"}}}`)
		c.WriteText(`{"serverContent":{"turnComplete":true}}`)
		c.TryRead()
	})
	s, err := c.Open(context.Background(), asr.StreamRequest{Lang: "en-US", Others: []string{"zh"}, Hints: []string{"VoxtMini"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Send(make([]byte, 6400+100)); err != nil {
		t.Fatal(err)
	}
	var partials []string
	for len(partials) == 0 || partials[len(partials)-1] != "hello wor" {
		ev, ok := next(t, s)
		if !ok || ev.Kind != asr.EventPartial {
			t.Fatalf("before Finish: %+v %v", ev, ok)
		}
		partials = append(partials, ev.Text)
	}
	if got := s.(asr.Salvager).Salvage(); got != "hello wor" {
		t.Errorf("Salvage = %q", got)
	}
	if err := s.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := s.Send([]byte{0, 0}); asr.KindOf(err) != asr.KindProtocol {
		t.Errorf("send after finish: %v", err)
	}
	more, final := rest(t, s)
	partials = append(partials, more...)
	if want := []string{"hel", "hello", "hello wor", "hello world", "hello world你好"}; strings.Join(partials, "|") != strings.Join(want, "|") {
		t.Errorf("partials %q", partials)
	}
	if final.Kind != asr.EventFinal || final.Text != "hello world你好" || final.Result.Model != "gemini-test-live" {
		t.Fatalf("final %+v", final)
	}
	if n := <-got; n != 6500 {
		t.Errorf("server got %d bytes", n)
	}
	if final.Result.Duration != 6500/2*time.Second/16000 {
		t.Errorf("duration %v", final.Result.Duration)
	}
}

// finishWith runs a session whose server answers audioStreamEnd with
// after, and returns the last event.
func finishWith(t *testing.T, after func(c *asrtest.Conn)) asr.Event {
	t.Helper()
	c := serve(t, func(c *asrtest.Conn) {
		handshake(c)
		c.WriteText(`{"serverContent":{"inputTranscription":{"text":"one"}}}`)
		readUntilEnd(c)
		after(c)
	})
	s, err := c.Open(context.Background(), asr.StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if ev, _ := next(t, s); ev.Text != "one" {
		t.Fatalf("partial %+v", ev)
	}
	if err := s.Finish(); err != nil {
		t.Fatal(err)
	}
	_, last := rest(t, s)
	return last
}

func TestFinishEnds(t *testing.T) {
	cases := map[string]struct {
		after func(c *asrtest.Conn)
		text  string
	}{
		"goAway": {func(c *asrtest.Conn) {
			c.WriteText(`{"serverContent":{"interimInputTranscription":{"text":"two"}}}`)
			c.WriteText(`{"go_away":{"timeLeft":"1s"}}`)
			c.TryRead()
		}, "one two"},
		"socket closed with text": {func(c *asrtest.Conn) {
			c.WriteText(`{"serverContent":{"interimInputTranscription":{"text":"two"}}}`)
			c.Close(websocket.StatusInternalError, "bye")
		}, "one two"},
		"closed normally": {func(c *asrtest.Conn) {}, "one"},
		"quiet": {func(c *asrtest.Conn) {
			c.WriteText(`{"serverContent":{"interimInputTranscription":{"text":"tail"}}}`)
			c.TryRead() // holds until the client goes
		}, "one tail"},
	}
	defer func(d time.Duration) { drainQuiet = d }(drainQuiet)
	drainQuiet = 100 * time.Millisecond
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ev := finishWith(t, tc.after)
			if ev.Kind != asr.EventFinal || ev.Text != tc.text {
				t.Errorf("last %+v", ev)
			}
		})
	}
}

func TestClosedEarly(t *testing.T) {
	// Closed after audioStreamEnd with nothing heard and an abnormal code: an error.
	c := serve(t, func(c *asrtest.Conn) {
		handshake(c)
		readUntilEnd(c)
		c.Close(websocket.StatusInternalError, "internal")
	})
	s, err := c.Open(context.Background(), asr.StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Finish()
	if _, last := rest(t, s); last.Kind != asr.EventError || asr.KindOf(last.Err) != asr.KindServer {
		t.Errorf("last %+v", last)
	}
	// Closed before Finish, with text: an error too (Salvage has the text).
	c = serve(t, func(c *asrtest.Conn) {
		handshake(c)
		c.WriteText(`{"serverContent":{"inputTranscription":{"text":"half"}}}`)
		c.Close(websocket.StatusGoingAway, "")
	})
	s, err = c.Open(context.Background(), asr.StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, last := rest(t, s); last.Kind != asr.EventError || s.(asr.Salvager).Salvage() != "half" {
		t.Errorf("last %+v", last)
	}
}

func TestErrorPacket(t *testing.T) {
	c := serve(t, func(c *asrtest.Conn) {
		handshake(c)
		c.WriteText(`{"serverContent":{"inputTranscription":{"text":"x"}}}`)
		c.WriteText(`{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`)
		c.TryRead()
	})
	s, err := c.Open(context.Background(), asr.StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, last := rest(t, s)
	var e *asr.Error
	if last.Kind != asr.EventError || !asAsr(last.Err, &e) || e.Kind != asr.KindQuota || e.Status != 429 {
		t.Errorf("last %+v", last)
	}
}

func TestOpenFails(t *testing.T) {
	cases := map[string]struct {
		handle func(c *asrtest.Conn)
		kind   asr.Kind
	}{
		"error packet": {func(c *asrtest.Conn) {
			c.Read()
			c.WriteText(`{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`)
			c.TryRead()
		}, asr.KindAuth},
		"close reason": {func(c *asrtest.Conn) {
			c.Read()
			c.Close(websocket.StatusPolicyViolation, "models/gemini-test-live is not found for API version v1beta, or is not supported for bidiGenerateContent.")
		}, asr.KindConfig},
		"bad key close": {func(c *asrtest.Conn) {
			c.Read()
			c.Close(websocket.StatusInvalidFramePayloadData, "API key not valid. Please pass a valid API key.")
		}, asr.KindAuth},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s, err := serve(t, tc.handle).Open(context.Background(), asr.StreamRequest{})
			if s != nil || asr.KindOf(err) != tc.kind {
				t.Errorf("Open = %v, %v", s, err)
			}
		})
	}
	// Transcripts before setupComplete are not a start.
	c := serve(t, func(c *asrtest.Conn) {
		c.Read()
		c.WriteText(`{"serverContent":{"inputTranscription":{"text":"x"}}}`)
		c.Close(websocket.StatusNormalClosure, "")
	})
	if _, err := c.Open(context.Background(), asr.StreamRequest{}); asr.KindOf(err) != asr.KindProtocol {
		t.Errorf("no setupComplete: %v", err)
	}
}

func TestKeyNeverInErrors(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close() // nobody listens there now
	_, err = client(t, "ws://"+addr).Open(context.Background(), asr.StreamRequest{})
	if asr.KindOf(err) != asr.KindNetwork {
		t.Errorf("refused: %v", err)
	}
	refused := client(t, asrtest.RefuseServer(t, http.StatusForbidden, `{"error":{"code":403,"message":"denied","status":"PERMISSION_DENIED"}}`))
	_, err2 := refused.Open(context.Background(), asr.StreamRequest{})
	if asr.KindOf(err2) != asr.KindAuth {
		t.Errorf("403: %v", err2)
	}
	for _, e := range []error{err, err2} {
		if msg := e.Error(); strings.Contains(msg, "AIza") || strings.Contains(msg, "test%2Fkey") {
			t.Errorf("the key leaked: %s", msg)
		}
	}
}

func TestCancel(t *testing.T) {
	c := serve(t, func(c *asrtest.Conn) {
		handshake(c)
		c.TryRead()
	})
	ctx, cancel := context.WithCancel(context.Background())
	s, err := c.Open(ctx, asr.StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Send(make([]byte, 3200))
	cancel()
	if _, last := rest(t, s); last.Kind != asr.EventError || asr.KindOf(last.Err) != asr.KindCancelled {
		t.Errorf("last %+v", last)
	}
}

func TestClose(t *testing.T) {
	c := serve(t, func(c *asrtest.Conn) {
		handshake(c)
		c.TryRead()
	})
	s, err := c.Open(context.Background(), asr.StreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Close()
	if _, ok := <-s.Events(); ok {
		t.Error("an event after Close")
	}
	if err := s.Send([]byte{0, 0}); asr.KindOf(err) != asr.KindCancelled {
		t.Errorf("send after close: %v", err)
	}
}

func asAsr(err error, e **asr.Error) bool {
	x, ok := err.(*asr.Error)
	*e = x
	return ok
}
