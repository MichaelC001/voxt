// Package mimo is Xiaomi MiMo's speech recognition: a recording sent as
// an OpenAI-style chat completion whose user message is the audio
// (input_audio, a base64 data URI), with asr_options.language; the text
// is the answer's message. Ported from Voxt (RemoteASRFileRequests
// transcribeXiaomiMiMo, RemoteASRTextSupport.xiaomiMiMoASRPayload).
//
// Imports: asr.
package mimo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
)

// ID is the service's preset id.
const ID = "mimo"

// DefaultEndpoint and DefaultModel are Xiaomi's.
const (
	DefaultEndpoint = "https://api.xiaomimimo.com/v1/chat/completions"
	DefaultModel    = "mimo-v2.5-asr"
)

// Models are the suggested models.
var Models = []string{DefaultModel}

// MaxBytes bounds a recording (as a data URI in a JSON body).
const MaxBytes = 20 << 20

// Client is a configured transcriber. Goroutine-safe.
type Client struct {
	endpoint, model, key string
	http                 *http.Client
}

var _ asr.FileTranscriber = (*Client)(nil)

// New checks cfg and returns a client. The endpoint may be the full
// chat completions URL or an API root (…/v1).
func New(cfg asr.Config) (*Client, error) {
	key := strings.TrimSpace(cfg.Key)
	if key == "" {
		return nil, asr.Errorf(ID, asr.KindConfig, "no API key")
	}
	ep := Endpoint(cfg.Endpoint)
	if _, err := asr.CheckEndpoint(ID, ep, "https"); err != nil {
		return nil, err
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultModel
	}
	client := cfg.HTTPClient
	if client == nil {
		client = asr.NewHTTPClient(cfg.Proxy, 60*time.Second)
	}
	return &Client{endpoint: ep, model: model, key: key, http: client}, nil
}

// Endpoint is the chat completions URL for what the user typed: ""
// is DefaultEndpoint, an API root gets /chat/completions appended.
func Endpoint(raw string) string {
	ep := strings.TrimRight(strings.TrimSpace(raw), "/")
	switch {
	case ep == "":
		return DefaultEndpoint
	case strings.HasSuffix(ep, "/chat/completions"):
		return ep
	}
	return ep + "/chat/completions"
}

// Caps: one upload, no prompt or hints (the API takes none).
func (c *Client) Caps() asr.Caps { return asr.Caps{File: true, MaxBytes: MaxBytes} }

// Model is the model the client asks for.
func (c *Client) Model() string { return c.model }

// Language is the service's language option for a hint: "zh", "en",
// else "auto".
func Language(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	if l == "zh" || l == "en" {
		return l
	}
	return "auto"
}

// Body is the request's JSON body for audio (a WAV file).
func (c *Client) Body(audio []byte, lang string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"model": c.model,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{map[string]any{
				"type":        "input_audio",
				"input_audio": map[string]any{"data": "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(audio)},
			}},
		}},
		"asr_options": map[string]any{"language": Language(lang)},
	})
}

// Transcribe sends req's audio.
func (c *Client) Transcribe(ctx context.Context, req asr.FileRequest) (asr.Result, error) {
	audio := req.Audio
	if req.Format == "pcm" {
		audio = asr.WAV(req.Audio, req.SampleRate)
	}
	if len(audio) > MaxBytes {
		return asr.Result{}, asr.Errorf(ID, asr.KindAudio, "the recording is %d MB, over the 20 MB limit", len(audio)>>20)
	}
	body, err := c.Body(audio, req.Lang)
	if err != nil {
		return asr.Result{}, &asr.Error{Kind: asr.KindProtocol, Provider: ID, Detail: "body", Err: err}
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
	}
	start := time.Now()
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return asr.Result{}, &asr.Error{Kind: asr.KindConfig, Provider: ID, Detail: "bad endpoint", Err: err}
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "application/json")
	hr.Header.Set("Authorization", "Bearer "+c.key)
	hr.Header.Set("User-Agent", "VoxtMini")
	resp, err := c.http.Do(hr)
	if err != nil {
		return asr.Result{}, asr.NetError(ctx, ID, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return asr.Result{}, asr.NetError(ctx, ID, err)
	}
	if resp.StatusCode/100 != 2 {
		code, msg := asr.ErrorBody(raw)
		if msg == "" {
			msg = resp.Status
		}
		return asr.Result{}, asr.HTTPError(ID, resp.StatusCode, code, msg)
	}
	text, err := parse(raw)
	if err != nil {
		return asr.Result{}, err
	}
	return asr.Result{Text: text, Model: c.model, Latency: time.Since(start)}, nil
}

// parse reads the chat completion's first message.
func parse(raw []byte) (string, error) {
	var v struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", &asr.Error{Kind: asr.KindProtocol, Provider: ID, Detail: asr.Clip(string(raw), 200), Err: err}
	}
	if len(v.Choices) == 0 {
		return "", &asr.Error{Kind: asr.KindProtocol, Provider: ID, Detail: "no choices: " + asr.Clip(string(raw), 200)}
	}
	c := v.Choices[0].Message.Content
	var s string
	if json.Unmarshal(c, &s) == nil {
		return strings.TrimSpace(s), nil
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(c, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return strings.TrimSpace(b.String()), nil
	}
	return "", &asr.Error{Kind: asr.KindProtocol, Provider: ID, Detail: "unexpected content: " + asr.Clip(string(c), 200)}
}
