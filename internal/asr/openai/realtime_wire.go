package openai

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/hehehai/VoxtMini/internal/asr"
)

// DefaultRealtimeEndpoint is OpenAI's Realtime transcription WebSocket URL.
const DefaultRealtimeEndpoint = "wss://api.openai.com/v1/realtime?intent=transcription"

// realtimeSampleRate is the PCM16 sample rate OpenAI Realtime expects (24 kHz mono).
const realtimeSampleRate = 24000

// IsRealtimeEndpoint reports whether raw routes to OpenAI Realtime
// WebSocket transcription rather than HTTP multipart upload: any wss://
// or ws:// URL, or an https:// or http:// URL whose path contains
// "/realtime".
func IsRealtimeEndpoint(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "wss", "ws":
		return true
	case "https", "http":
		return strings.Contains(strings.ToLower(u.Path), "/realtime")
	default:
		return false
	}
}

// RealtimeEndpoint normalises raw into a WebSocket URL: "" becomes
// DefaultRealtimeEndpoint, https/http become wss/ws, and api.openai.com
// gets ?intent=transcription when no intent or model query is set.
// Custom and local gateway URLs keep their path and query verbatim.
func RealtimeEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultRealtimeEndpoint
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw // CheckEndpoint reports it
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	if strings.EqualFold(u.Hostname(), "api.openai.com") {
		if u.Path == "" || u.Path == "/" {
			u.Path = "/v1/realtime"
		}
		q := u.Query()
		if q.Get("intent") == "" && q.Get("model") == "" {
			q.Set("intent", "transcription")
			u.RawQuery = q.Encode()
		}
	}
	return u.String()
}

type rtSessionUpdate struct {
	Type    string           `json:"type"`
	Session rtSessionPayload `json:"session"`
}

type rtSessionPayload struct {
	InputAudioFormat        string                 `json:"input_audio_format"`
	InputAudioTranscription rtTranscriptionPayload `json:"input_audio_transcription"`
	TurnDetection           rtTurnDetection        `json:"turn_detection"`
}

type rtTranscriptionPayload struct {
	Model    string `json:"model"`
	Language string `json:"language,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
}

type rtTurnDetection struct {
	Type              string  `json:"type"`
	Threshold         float64 `json:"threshold"`
	PrefixPaddingMS   int     `json:"prefix_padding_ms"`
	SilenceDurationMS int     `json:"silence_duration_ms"`
}

type rtAudioAppend struct {
	Type  string `json:"type"`
	Audio string `json:"audio"`
}

type rtAudioCommit struct {
	Type string `json:"type"`
}

func sessionUpdateMessage(model, lang, prompt string) []byte {
	tr := rtTranscriptionPayload{
		Model:    model,
		Language: isoLanguage(lang),
	}
	if prompt != "" && !strings.Contains(model, "diarize") {
		tr.Prompt = prompt
	}
	msg := rtSessionUpdate{
		Type: "transcription_session.update",
		Session: rtSessionPayload{
			InputAudioFormat:        "pcm16",
			InputAudioTranscription: tr,
			TurnDetection: rtTurnDetection{
				Type:              "server_vad",
				Threshold:         0.5,
				PrefixPaddingMS:   300,
				SilenceDurationMS: 500,
			},
		},
	}
	b, _ := json.Marshal(msg)
	return b
}

func audioAppendMessage(pcm24k []byte) []byte {
	b, _ := json.Marshal(rtAudioAppend{
		Type:  "input_audio_buffer.append",
		Audio: base64.StdEncoding.EncodeToString(pcm24k),
	})
	return b
}

func audioCommitMessage() []byte {
	b, _ := json.Marshal(rtAudioCommit{Type: "input_audio_buffer.commit"})
	return b
}

type rtServerEvent struct {
	Type           string `json:"type"`
	ItemID         string `json:"item_id"`
	PreviousItemID string `json:"previous_item_id"`
	Delta          string `json:"delta"`
	Transcript     string `json:"transcript"`
	Item           *struct {
		ID string `json:"id"`
	} `json:"item"`
}

// rtTranscript accumulates transcription items in spoken order,
// supporting live deltas and authoritative completed reconciliation.
// Goroutine-safe (Salvage reads it from the session goroutine).
type rtTranscript struct {
	mu    sync.Mutex
	order []string
	items map[string]*rtItem
}

type rtItem struct {
	id        string
	delta     strings.Builder
	final     string
	completed bool
}

func (t *rtTranscript) ensureLocked(id, prevID string) *rtItem {
	if t.items == nil {
		t.items = make(map[string]*rtItem)
	}
	if it, ok := t.items[id]; ok {
		return it
	}
	it := &rtItem{id: id}
	t.items[id] = it
	if prevID != "" {
		if idx := slices.Index(t.order, prevID); idx >= 0 {
			t.order = slices.Insert(t.order, idx+1, id)
			return it
		}
	}
	t.order = append(t.order, id)
	return it
}

func (t *rtTranscript) register(id, prevID string) {
	if id == "" {
		return
	}
	t.mu.Lock()
	t.ensureLocked(id, prevID)
	t.mu.Unlock()
}

func (t *rtTranscript) addDelta(id, delta string) {
	if id == "" || delta == "" {
		return
	}
	t.mu.Lock()
	it := t.ensureLocked(id, "")
	if !it.completed {
		it.delta.WriteString(delta)
	}
	t.mu.Unlock()
}

func (t *rtTranscript) complete(id, text string) {
	if id == "" {
		return
	}
	t.mu.Lock()
	it := t.ensureLocked(id, "")
	it.final = strings.TrimSpace(text)
	it.completed = true
	t.mu.Unlock()
}

func (t *rtTranscript) known(id string) bool {
	if id == "" {
		return false
	}
	t.mu.Lock()
	_, ok := t.items[id]
	t.mu.Unlock()
	return ok
}

func (t *rtTranscript) hasPending() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range t.order {
		if !t.items[id].completed {
			return true
		}
	}
	return false
}

func (t *rtTranscript) String() string {
	t.mu.Lock()
	parts := make([]string, 0, len(t.order))
	for _, id := range t.order {
		it := t.items[id]
		var s string
		if it.completed {
			s = it.final
		} else {
			s = strings.TrimSpace(it.delta.String())
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	t.mu.Unlock()
	return joinText(parts)
}

// joinText joins segments with a space between spaced scripts (Latin,
// Cyrillic…) and directly where either side is CJK or full-width
// punctuation.
func joinText(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			last, _ := utf8.DecodeLastRuneInString(b.String())
			first, _ := utf8.DecodeRuneInString(p)
			if !wide(last) && !wide(first) && !unicode.IsSpace(last) {
				b.WriteByte(' ')
			}
		}
		b.WriteString(p)
	}
	return b.String()
}

func wide(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar) ||
		(r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF)
}

// resampler is a stateful linear PCM16LE mono resampler (used for
// 16 kHz → 24 kHz, an exact 2:3 ratio) with exact integer phase stepping
// and click-free continuity across streaming frames.
type resampler struct {
	inStep  int64
	outStep int64
	hasPrev bool
	prev    int16
	acc     int64
}

func newResampler(inRate, outRate int) *resampler {
	if inRate <= 0 {
		inRate = asr.SampleRate
	}
	if outRate <= 0 {
		outRate = realtimeSampleRate
	}
	g := gcd(inRate, outRate)
	return &resampler{
		inStep:  int64(inRate / g),
		outStep: int64(outRate / g),
	}
}

func (r *resampler) process(pcm []byte) []byte {
	if r.inStep == r.outStep {
		return pcm
	}
	n := len(pcm) / 2
	if n == 0 {
		return nil
	}
	// Upper bound on output samples for this chunk.
	maxOut := (int64(n)*r.outStep)/r.inStep + 2
	out := make([]byte, 0, maxOut*2)
	appendSample := func(v int16) {
		out = binary.LittleEndian.AppendUint16(out, uint16(v))
	}
	for i := range n {
		cur := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
		if !r.hasPrev {
			appendSample(cur)
			r.prev = cur
			r.hasPrev = true
			r.acc = r.inStep
			continue
		}
		for r.acc <= r.outStep {
			v := int64(r.prev) + (int64(cur)-int64(r.prev))*r.acc/r.outStep
			appendSample(int16(v))
			r.acc += r.inStep
		}
		r.acc -= r.outStep
		r.prev = cur
	}
	return out
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a <= 0 {
		return 1
	}
	return a
}

// isCommitEmptyError reports an OpenAI Realtime error caused by
// committing an empty or <100 ms audio buffer (e.g. after server VAD
// already committed the last speech, or on a silent recording).
func isCommitEmptyError(code, msg string) bool {
	c := strings.ToLower(strings.TrimSpace(code))
	if c == "input_audio_buffer_commit_empty" {
		return true
	}
	m := strings.ToLower(msg)
	return strings.Contains(m, "buffer too small") ||
		(strings.Contains(m, "commit") && strings.Contains(m, "empty"))
}

// realtimeError classifies an OpenAI Realtime protocol error event.
func realtimeError(code, msg string, itemFailed bool) *asr.Error {
	e := &asr.Error{Provider: ID, Code: code, Detail: asr.Clip(msg, 300)}
	low := strings.ToLower(code + " " + msg)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(low, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("invalid_api_key", "unauthorized", "authentication"):
		e.Kind = asr.KindAuth
	case has("rate_limit", "too many requests") && !has("insufficient_quota", "billing", "balance"):
		e.Kind, e.Retryable = asr.KindQuota, true
	case has("insufficient_quota", "quota", "billing", "balance"):
		e.Kind = asr.KindQuota
	case has("model_not_found", "invalid_model", "does not exist", "model not found"):
		e.Kind = asr.KindConfig
	case has("server_error", "internal_error", "service_unavailable", "overloaded"):
		e.Kind, e.Retryable = asr.KindServer, true
	case has("audio") && has("too long", "too large", "format", "sample_rate", "sample rate", "unsupported"):
		e.Kind = asr.KindAudio
	default:
		if itemFailed {
			e.Kind, e.Retryable = asr.KindServer, true
		} else {
			e.Kind = asr.KindProtocol
		}
	}
	if e.Detail == "" {
		e.Detail = "the service reported an error"
	}
	return e
}
