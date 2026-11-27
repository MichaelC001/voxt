package openai

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/privnet"
)

func TestRealtimeRouting(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		realtime bool
		want     string
	}{
		{"", false, DefaultRealtimeEndpoint},
		{"https://api.openai.com/v1/audio/transcriptions", false, ""},
		{"https://api.groq.com/openai/v1", false, ""},
		{"http://127.0.0.1:8080/v1", false, ""},
		{"wss://api.openai.com/v1/realtime?intent=transcription", true, "wss://api.openai.com/v1/realtime?intent=transcription"},
		{"wss://api.openai.com/v1/realtime", true, "wss://api.openai.com/v1/realtime?intent=transcription"},
		{"https://api.openai.com/v1/realtime", true, "wss://api.openai.com/v1/realtime?intent=transcription"},
		{"ws://127.0.0.1:8080/v1/realtime", true, "ws://127.0.0.1:8080/v1/realtime"},
		{"http://127.0.0.1:8080/v1/realtime", true, "ws://127.0.0.1:8080/v1/realtime"},
		{"ws://100.88.1.2:8080/v1/realtime?lang=zh", true, "ws://100.88.1.2:8080/v1/realtime?lang=zh"},
		{"wss://relay.example.com/custom/ws", true, "wss://relay.example.com/custom/ws"},
	} {
		if got := IsRealtimeEndpoint(tc.raw); got != tc.realtime {
			t.Errorf("IsRealtimeEndpoint(%q) = %v, want %v", tc.raw, got, tc.realtime)
		}
		if tc.want != "" {
			if got := RealtimeEndpoint(tc.raw); got != tc.want {
				t.Errorf("RealtimeEndpoint(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		}
	}
}

func TestResampler(t *testing.T) {
	// 16 kHz -> 24 kHz linear ramp split across two chunks: verify exact
	// interpolation and click-free continuity at the chunk boundary.
	r := newResampler(16000, 24000)
	makePCM := func(samples []int16) []byte {
		b := make([]byte, len(samples)*2)
		for i, v := range samples {
			binary.LittleEndian.PutUint16(b[i*2:], uint16(v))
		}
		return b
	}
	readPCM := func(b []byte) []int16 {
		out := make([]int16, len(b)/2)
		for i := range out {
			out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
		}
		return out
	}

	out1 := readPCM(r.process(makePCM([]int16{0, 300})))
	out2 := readPCM(r.process(makePCM([]int16{600, 900})))
	got := append(out1, out2...)
	want := []int16{0, 200, 400, 600, 800}
	if !slices.Equal(got, want) {
		t.Fatalf("resampled = %v, want %v", got, want)
	}

	// 24 kHz -> 24 kHz is a passthrough.
	pass := newResampler(24000, 24000)
	in := makePCM([]int16{10, 20, 30})
	if got := pass.process(in); !slices.Equal(got, in) {
		t.Fatalf("passthrough altered bytes")
	}
}

func startWSServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		handler(w, r, conn)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/realtime"
}

func sendJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, b)
}

func TestRealtimeMultiTurnAndFinalWord(t *testing.T) {
	var (
		mu         sync.Mutex
		gotAuth    string
		gotBeta    string
		gotSession rtSessionUpdate
		audioBytes int
		commits    int
	)

	ep := startWSServer(t, func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		gotBeta = r.Header.Get("OpenAI-Beta")
		mu.Unlock()

		// Send session.created first (should be ignored until updated).
		_ = sendJSON(ctx, conn, map[string]any{"type": "transcription_session.created"})

		appends := 0
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				Type  string `json:"type"`
				Audio string `json:"audio"`
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "transcription_session.update":
				mu.Lock()
				_ = json.Unmarshal(data, &gotSession)
				mu.Unlock()
				_ = sendJSON(ctx, conn, map[string]any{"type": "transcription_session.updated"})
			case "input_audio_buffer.append":
				raw, err := base64.StdEncoding.DecodeString(msg.Audio)
				if err == nil {
					mu.Lock()
					audioBytes += len(raw)
					mu.Unlock()
				}
				appends++
				// After 3 chunks (300 ms), simulate server VAD committing turn 1.
				if appends == 3 {
					_ = sendJSON(ctx, conn, map[string]any{
						"type":             "input_audio_buffer.committed",
						"item_id":          "item_1",
						"previous_item_id": "",
					})
					_ = sendJSON(ctx, conn, map[string]any{
						"type":    "conversation.item.input_audio_transcription.delta",
						"item_id": "item_1",
						"delta":   "Hello ",
					})
					_ = sendJSON(ctx, conn, map[string]any{
						"type":    "conversation.item.input_audio_transcription.delta",
						"item_id": "item_1",
						"delta":   "wor",
					})
					_ = sendJSON(ctx, conn, map[string]any{
						"type":       "conversation.item.input_audio_transcription.completed",
						"item_id":    "item_1",
						"transcript": "Hello world.",
					})
				}
			case "input_audio_buffer.commit":
				mu.Lock()
				commits++
				mu.Unlock()
				// Final word spoken right before stop.
				_ = sendJSON(ctx, conn, map[string]any{
					"type":             "input_audio_buffer.committed",
					"item_id":          "item_2",
					"previous_item_id": "item_1",
				})
				_ = sendJSON(ctx, conn, map[string]any{
					"type":    "conversation.item.input_audio_transcription.delta",
					"item_id": "item_2",
					"delta":   "final",
				})
				_ = sendJSON(ctx, conn, map[string]any{
					"type":       "conversation.item.input_audio_transcription.completed",
					"item_id":    "item_2",
					"transcript": "final word.",
				})
			}
		}
	})

	c, err := NewRealtime(asr.Config{
		Endpoint: ep,
		Key:      "sk-test",
		Model:    "gpt-4o-transcribe",
		Prompt:   "VoxtMini context",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Caps().Stream || c.Caps().File || !c.Caps().Prompt {
		t.Fatalf("unexpected caps: %+v", c.Caps())
	}

	var partials []string
	res, err := asr.StreamAll(context.Background(), c, asr.StreamRequest{
		SampleRate: asr.SampleRate,
		Lang:       "zh-Hans",
		Hints:      []string{"VoxtMini", "OpenAI"},
	}, asr.Silence(500*time.Millisecond), asr.Feed{
		OnPartial: func(text string) { partials = append(partials, text) },
	})
	if err != nil {
		t.Fatalf("StreamAll: %v", err)
	}
	if res.Text != "Hello world. final word." {
		t.Fatalf("final text = %q, want %q", res.Text, "Hello world. final word.")
	}
	if res.Model != "gpt-4o-transcribe" {
		t.Errorf("model = %q", res.Model)
	}
	wantPartials := []string{
		"Hello",
		"Hello wor",
		"Hello world.",
		"Hello world. final",
		"Hello world. final word.",
	}
	if !slices.Equal(partials, wantPartials) {
		t.Errorf("partials = %#v, want %#v", partials, wantPartials)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "Bearer sk-test" || gotBeta != "realtime=v1" {
		t.Errorf("headers auth=%q beta=%q", gotAuth, gotBeta)
	}
	if gotSession.Session.InputAudioFormat != "pcm16" ||
		gotSession.Session.InputAudioTranscription.Model != "gpt-4o-transcribe" ||
		gotSession.Session.InputAudioTranscription.Language != "zh" ||
		gotSession.Session.InputAudioTranscription.Prompt != "VoxtMini context\nVoxtMini, OpenAI" {
		t.Errorf("session update = %+v", gotSession)
	}
	if commits != 1 {
		t.Errorf("commits = %d, want 1", commits)
	}
	// 500 ms at 16 kHz = 8000 samples -> resampled to 24 kHz = ~12000 samples (~24000 bytes).
	if audioBytes < 23990 || audioBytes > 24010 {
		t.Errorf("resampled audio bytes = %d, want ~24000", audioBytes)
	}
}

func TestRealtimeOutOfOrderCompletion(t *testing.T) {
	ep := startWSServer(t, func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "transcription_session.update":
				_ = sendJSON(ctx, conn, map[string]any{"type": "session.updated"})
			case "input_audio_buffer.commit":
				// Commit item_1 then item_2, but complete item_2 BEFORE item_1.
				_ = sendJSON(ctx, conn, map[string]any{"type": "input_audio_buffer.committed", "item_id": "item_1", "previous_item_id": ""})
				_ = sendJSON(ctx, conn, map[string]any{"type": "input_audio_buffer.committed", "item_id": "item_2", "previous_item_id": "item_1"})
				_ = sendJSON(ctx, conn, map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "item_2", "transcript": "世界。"})
				_ = sendJSON(ctx, conn, map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "item_1", "transcript": "你好，"})
			}
		}
	})

	c, err := NewRealtime(asr.Config{Endpoint: ep}) // keyless local ws:// gateway
	if err != nil {
		t.Fatal(err)
	}
	res, err := asr.StreamAll(context.Background(), c, asr.StreamRequest{}, asr.Silence(200*time.Millisecond), asr.Feed{})
	if err != nil {
		t.Fatalf("StreamAll: %v", err)
	}
	if res.Text != "你好，世界。" {
		t.Fatalf("text = %q, want %q", res.Text, "你好，世界。")
	}
}

func TestRealtimeLocalAndPublicWS(t *testing.T) {
	// Keyless loopback ws:// works and omits Authorization header.
	var gotAuth string
	ep := startWSServer(t, func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		gotAuth = r.Header.Get("Authorization")
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "transcription_session.update":
				_ = sendJSON(ctx, conn, map[string]any{"type": "transcription_session.updated"})
			case "input_audio_buffer.commit":
				_ = sendJSON(ctx, conn, map[string]any{
					"type":       "conversation.item.input_audio_transcription.completed",
					"item_id":    "item_1",
					"transcript": "local ok",
				})
			}
		}
	})

	c, err := NewRealtime(asr.Config{Endpoint: ep})
	if err != nil {
		t.Fatalf("keyless local ws: %v", err)
	}
	res, err := asr.StreamAll(context.Background(), c, asr.StreamRequest{}, asr.Silence(100*time.Millisecond), asr.Feed{})
	if err != nil || res.Text != "local ok" {
		t.Fatalf("local ws result = %+v, err = %v", res, err)
	}
	if gotAuth != "" {
		t.Errorf("expected empty Authorization header for keyless local ws, got %q", gotAuth)
	}

	// Public IP over plain ws:// is refused at config time.
	if _, err := NewRealtime(asr.Config{Key: "sk-test", Endpoint: "ws://203.0.113.5:8080/v1/realtime"}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("public ws IP: got %v, want KindConfig", err)
	}
	// wss:// without API key is refused at config time.
	if _, err := NewRealtime(asr.Config{Endpoint: "wss://api.openai.com/v1/realtime?intent=transcription"}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("wss without key: got %v, want KindConfig", err)
	}
	// Public hostname over plain ws:// is refused at dial time by privnet.Guard.
	pubClient, err := NewRealtime(asr.Config{
		Key:      "sk-test",
		Endpoint: "ws://example.com/v1/realtime",
		HTTPClient: &http.Client{
			Transport: privnet.Guard(&http.Transport{}, staticResolver("93.184.216.34")),
		},
	})
	if err != nil {
		t.Fatalf("NewRealtime public host: %v", err)
	}
	_, err = pubClient.Open(context.Background(), asr.StreamRequest{})
	if !errors.Is(err, privnet.ErrPublic) {
		t.Errorf("public ws hostname: got %v, want privnet.ErrPublic", err)
	}
}

type staticResolver string

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP(string(r))}}, nil
}

func TestRealtimeEmptyAndSilence(t *testing.T) {
	t.Run("zero audio", func(t *testing.T) {
		ep := startWSServer(t, func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
			ctx := r.Context()
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var msg struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &msg) == nil && msg.Type == "transcription_session.update" {
					_ = sendJSON(ctx, conn, map[string]any{"type": "transcription_session.updated"})
				}
			}
		})
		c, err := NewRealtime(asr.Config{Endpoint: ep})
		if err != nil {
			t.Fatal(err)
		}
		res, err := asr.StreamAll(context.Background(), c, asr.StreamRequest{}, nil, asr.Feed{})
		if err != nil || res.Text != "" {
			t.Fatalf("zero audio: res=%+v err=%v", res, err)
		}
	})

	t.Run("commit empty on silence", func(t *testing.T) {
		ep := startWSServer(t, func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
			ctx := r.Context()
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var msg struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &msg) != nil {
					continue
				}
				switch msg.Type {
				case "transcription_session.update":
					_ = sendJSON(ctx, conn, map[string]any{"type": "transcription_session.updated"})
				case "input_audio_buffer.commit":
					_ = sendJSON(ctx, conn, map[string]any{
						"type": "error",
						"error": map[string]any{
							"type":    "invalid_request_error",
							"code":    "input_audio_buffer_commit_empty",
							"message": "Error committing input audio buffer: buffer too small.",
						},
					})
				}
			}
		})
		c, err := NewRealtime(asr.Config{Endpoint: ep})
		if err != nil {
			t.Fatal(err)
		}
		res, err := asr.StreamAll(context.Background(), c, asr.StreamRequest{}, asr.Silence(time.Second), asr.Feed{})
		if err != nil || res.Text != "" {
			t.Fatalf("silence commit_empty: res=%+v err=%v", res, err)
		}
	})
}

func TestRealtimeErrorsAndTimeout(t *testing.T) {
	t.Run("handshake 401", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_api_key","message":"bad key"}}`))
		}))
		defer srv.Close()
		ep := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/realtime"
		c, err := NewRealtime(asr.Config{Endpoint: ep, Key: "sk-bad"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Open(context.Background(), asr.StreamRequest{}); asr.KindOf(err) != asr.KindAuth {
			t.Fatalf("401: got %v, want KindAuth", err)
		}
	})

	t.Run("transcription failed preserves salvage", func(t *testing.T) {
		ep := startWSServer(t, func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
			ctx := r.Context()
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var msg struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &msg) != nil {
					continue
				}
				switch msg.Type {
				case "transcription_session.update":
					_ = sendJSON(ctx, conn, map[string]any{"type": "transcription_session.updated"})
				case "input_audio_buffer.commit":
					_ = sendJSON(ctx, conn, map[string]any{
						"type":       "conversation.item.input_audio_transcription.completed",
						"item_id":    "item_1",
						"transcript": "first sentence",
					})
					_ = sendJSON(ctx, conn, map[string]any{
						"type":    "conversation.item.input_audio_transcription.failed",
						"item_id": "item_2",
						"error":   map[string]any{"type": "server_error", "code": "server_error", "message": "upstream boom"},
					})
				}
			}
		})
		c, err := NewRealtime(asr.Config{Endpoint: ep})
		if err != nil {
			t.Fatal(err)
		}
		st, err := c.Open(context.Background(), asr.StreamRequest{})
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		if err := st.Send(asr.Silence(100 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if err := st.Finish(); err != nil {
			t.Fatal(err)
		}
		var last asr.Event
		for ev := range st.Events() {
			last = ev
		}
		if last.Kind != asr.EventError || asr.KindOf(last.Err) != asr.KindServer {
			t.Fatalf("last = %+v, want KindServer error", last)
		}
		if got := st.(asr.Salvager).Salvage(); got != "first sentence" {
			t.Errorf("Salvage() = %q, want %q", got, "first sentence")
		}
	})

	t.Run("finish timeout", func(t *testing.T) {
		ep := startWSServer(t, func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
			ctx := r.Context()
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var msg struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &msg) == nil && msg.Type == "transcription_session.update" {
					_ = sendJSON(ctx, conn, map[string]any{"type": "transcription_session.updated"})
				}
				// Intentionally ignore input_audio_buffer.commit.
			}
		})
		c, err := NewRealtime(asr.Config{Endpoint: ep})
		if err != nil {
			t.Fatal(err)
		}
		c.finishTimeout = 150 * time.Millisecond
		_, err = asr.StreamAll(context.Background(), c, asr.StreamRequest{}, asr.Silence(100*time.Millisecond), asr.Feed{})
		if asr.KindOf(err) != asr.KindNetwork {
			t.Fatalf("timeout: got %v, want KindNetwork", err)
		}
	})
}
