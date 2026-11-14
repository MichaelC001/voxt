package gemini

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hehehai/VoxtMini/internal/asr"
)

// maxVocabulary bounds customVocabulary, as Voxt does.
const maxVocabulary = 1000

type setupMsg struct {
	Setup setup `json:"setup"`
}

type setup struct {
	Model            string           `json:"model"`
	GenerationConfig generationConfig `json:"generationConfig"`
	Transcription    transcription    `json:"inputAudioTranscription"`
}

type generationConfig struct {
	ResponseModalities []string `json:"responseModalities"`
}

type transcription struct {
	// LanguageCodes empty: the service detects the language, which also
	// keeps mid-sentence code-mixing working.
	LanguageCodes    []string `json:"languageCodes"`
	Mode             string   `json:"mode"`
	CustomVocabulary []string `json:"customVocabulary,omitempty"`
}

// setupMessage is the first message: the model, text output only, and the
// input transcription's languages and hot words.
func setupMessage(model string, req asr.StreamRequest) []byte {
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultModel
	}
	if !strings.HasPrefix(model, "models/") {
		model = "models/" + model
	}
	return mustJSON(setupMsg{Setup: setup{
		Model:            model,
		GenerationConfig: generationConfig{ResponseModalities: []string{"TEXT"}},
		Transcription: transcription{
			LanguageCodes:    languageCodes(req),
			Mode:             "SMART",
			CustomVocabulary: vocabulary(req.Hints),
		},
	}})
}

// languageCodes are the base codes of the hinted language and the user's
// other ones (Voxt sends all of the user's languages), deduplicated; none
// when there is no hint (detect).
func languageCodes(req asr.StreamRequest) []string {
	out := []string{}
	if strings.TrimSpace(req.Lang) == "" {
		return out
	}
	seen := map[string]bool{}
	for _, l := range append([]string{req.Lang}, req.Others...) {
		l = strings.ToLower(strings.TrimSpace(l))
		if i := strings.IndexAny(l, "-_"); i >= 0 {
			l = l[:i]
		}
		if l != "" && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// vocabulary is hints trimmed and deduplicated, at most maxVocabulary.
func vocabulary(hints []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, h := range hints {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
		if len(out) == maxVocabulary {
			break
		}
	}
	return out
}

type audioMsg struct {
	RealtimeInput realtimeInput `json:"realtimeInput"`
}

type realtimeInput struct {
	Audio          *blob `json:"audio,omitempty"`
	AudioStreamEnd bool  `json:"audioStreamEnd,omitempty"`
}

type blob struct {
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// audioMessage is one frame of PCM16 at rate Hz.
func audioMessage(pcm []byte, rate int) []byte {
	return mustJSON(audioMsg{RealtimeInput: realtimeInput{Audio: &blob{
		Data:     base64.StdEncoding.EncodeToString(pcm),
		MimeType: fmt.Sprintf("audio/pcm;rate=%d", rate),
	}}})
}

// endMessage says the audio is over.
func endMessage() []byte {
	return mustJSON(audioMsg{RealtimeInput: realtimeInput{AudioStreamEnd: true}})
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // plain structs: cannot fail
	}
	return b
}

// message is what one server message says.
type message struct {
	setupComplete bool
	interim       string // a segment's draft, "" for none
	final         string // a settled segment, "" for none
	turnComplete  bool
	goAway        bool
	err           *asr.Error
}

type object = map[string]json.RawMessage

// pick is the value of the camelCase or the snake_case key.
func pick(o object, camel, snake string) (json.RawMessage, bool) {
	if v, ok := o[camel]; ok {
		return v, true
	}
	v, ok := o[snake]
	return v, ok
}

// parse reads one server message; ok false for one that is not a JSON
// object.
func parse(data []byte) (m message, ok bool) {
	var o object
	if json.Unmarshal(data, &o) != nil || o == nil {
		return m, false
	}
	if raw, ok := o["error"]; ok && string(raw) != "null" {
		m.err = packetError(raw)
		return m, true
	}
	_, m.setupComplete = pick(o, "setupComplete", "setup_complete")
	_, m.goAway = pick(o, "goAway", "go_away")
	raw, ok := pick(o, "serverContent", "server_content")
	if !ok {
		return m, true
	}
	var sc object
	if json.Unmarshal(raw, &sc) != nil {
		return m, true
	}
	m.interim = textOf(sc, "interimInputTranscription", "interim_input_transcription")
	m.final = textOf(sc, "inputTranscription", "input_transcription")
	if raw, ok := pick(sc, "turnComplete", "turn_complete"); ok {
		json.Unmarshal(raw, &m.turnComplete)
	}
	return m, true
}

func textOf(o object, camel, snake string) string {
	raw, ok := pick(o, camel, snake)
	if !ok {
		return ""
	}
	var t struct {
		Text string `json:"text"`
	}
	json.Unmarshal(raw, &t)
	return strings.TrimSpace(t.Text)
}

// packetError is the error of an error packet: {"code":…,"message":…,
// "status":…} (Google's RPC status) or a bare string.
func packetError(raw json.RawMessage) *asr.Error {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return serviceError(0, "", s)
	}
	var e struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	}
	json.Unmarshal(raw, &e)
	code := 0
	switch c := e.Code.(type) {
	case float64:
		code = int(c)
	case string:
		if e.Status == "" {
			e.Status = c
		}
	}
	return serviceError(code, e.Status, e.Message)
}

// statusHTTP is the HTTP status of Google's RPC status names.
var statusHTTP = map[string]int{
	"INVALID_ARGUMENT":    400,
	"FAILED_PRECONDITION": 400,
	"OUT_OF_RANGE":        400,
	"UNAUTHENTICATED":     401,
	"PERMISSION_DENIED":   403,
	"NOT_FOUND":           404,
	"RESOURCE_EXHAUSTED":  429,
	"CANCELLED":           499,
	"INTERNAL":            500,
	"UNKNOWN":             500,
	"DATA_LOSS":           500,
	"UNAVAILABLE":         503,
	"DEADLINE_EXCEEDED":   504,
}

// serviceError classifies an error the service reported: by its code (an
// HTTP status) or its status name as asr.HTTPError does, then by its
// words (Google answers a bad key with 400 INVALID_ARGUMENT "API key not
// valid").
func serviceError(code int, status, msg string) *asr.Error {
	status = strings.TrimSpace(status)
	msg = strings.TrimSpace(msg)
	httpStatus := code
	if httpStatus < 100 || httpStatus > 599 {
		httpStatus = statusHTTP[strings.ToUpper(status)]
	}
	var e *asr.Error
	if httpStatus != 0 {
		e = asr.HTTPError(ID, httpStatus, status, msg)
		if code < 100 || code > 599 {
			e.Status = 0 // derived from the status name, not reported
		}
	} else {
		e = &asr.Error{Kind: asr.KindProtocol, Provider: ID, Code: status, Detail: asr.Clip(msg, 300)}
	}
	reclassify(e, status+" "+msg)
	if e.Detail == "" {
		e.Detail = status
	}
	if e.Detail == "" {
		e.Detail = "the service failed the session"
	}
	return e
}

// reclassify corrects e's kind by what the service's text says.
func reclassify(e *asr.Error, text string) {
	low := strings.ToLower(text)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(low, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("api key not valid", "api_key_invalid", "invalid api key", "api key expired", "api key not found", "unauthenticated"):
		e.Kind, e.Retryable = asr.KindAuth, false
	case has("resource_exhausted", "quota", "rate limit"):
		e.Kind, e.Retryable = asr.KindQuota, has("rate limit", "per minute")
	case has("models/", "model") && has("not found", "not supported", "does not exist", "is not available"):
		e.Kind, e.Retryable = asr.KindConfig, false
	case has("location is not supported", "region is not supported"):
		e.Kind, e.Retryable = asr.KindConfig, false
	}
}

// join joins transcript segments, trimmed: a space only between two
// non-CJK boundaries (Gemini's Chinese segments must not be spaced, its
// English ones must).
func join(segments []string) string {
	var b strings.Builder
	last := rune(0)
	for _, s := range segments {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		first := []rune(s)[0]
		if b.Len() > 0 && !isCJK(last) && !isCJK(first) {
			b.WriteByte(' ')
		}
		b.WriteString(s)
		r := []rune(s)
		last = r[len(r)-1]
	}
	return b.String()
}

// isCJK: CJK symbols and ideographs, full-width forms, kana and Hangul
// syllables (Voxt's ranges).
func isCJK(r rune) bool {
	switch {
	case r >= 0x3000 && r <= 0x303F, r >= 0x3400 && r <= 0x4DBF, r >= 0x4E00 && r <= 0x9FFF,
		r >= 0xF900 && r <= 0xFAFF, r >= 0xFF00 && r <= 0xFFEF,
		r >= 0x3040 && r <= 0x30FF, r >= 0xAC00 && r <= 0xD7AF:
		return true
	}
	return false
}
