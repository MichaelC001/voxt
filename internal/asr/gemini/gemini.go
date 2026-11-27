// Package gemini is Google's Gemini speech recognition: both the Live
// API WebSocket streamer (for live models such as
// gemini-3.5-transcribe-live) and non-streaming generateContent audio
// transcription (for multimodal models such as gemini-4-hydrogen and
// gemini-4-neon).
//
// Live WebSocket (wire.go, stream.go):
//
//	→ {"setup":{"model":"models/…","generationConfig":{"responseModalities":["TEXT"]},
//	   "inputAudioTranscription":{"languageCodes":[…],"mode":"SMART","customVocabulary":[…]}}}
//	← {"setupComplete":{}}
//	→ {"realtimeInput":{"audio":{"data":"<base64 PCM>","mimeType":"audio/pcm;rate=16000"}}}…
//	← {"serverContent":{"interimInputTranscription":{"text":…}}}   the draft of a segment
//	← {"serverContent":{"inputTranscription":{"text":…}}}          a settled segment
//	← {"serverContent":{"turnComplete":true}}                       a pause (keeps going)
//	→ {"realtimeInput":{"audioStreamEnd":true}}
//	← turnComplete or goAway, or the socket closes: the final text
//	← {"error":{"code":…,"message":…,"status":…}} at any point
//
// Non-streaming generateContent (file.go):
//
//	POST …/v1beta/models/{model}:generateContent (x-goog-api-key header)
//	WAV audio as inlineData alongside a transcription-only system instruction.
//
// NewProvider routes automatically between Live and generateContent based
// on the model name and endpoint (UseLive).
//
// Imports: asr.
package gemini

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/hehehai/VoxtMini/internal/asr"
)

// ID is the provider's preset id.
const ID = "gemini"

// DefaultEndpoint is the Live API's WebSocket (v1beta).
const DefaultEndpoint = "wss://generativelanguage.googleapis.com" + defaultPath

const defaultPath = "/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent"

// DefaultModel is the model of a new setup.
const DefaultModel = "gemini-3.5-transcribe-live"

// Models are the models offered in the model menu (any id may be typed).
var Models = []string{
	DefaultModel,
	"gemini-4-hydrogen",
	"gemini-4-neon",
}

// UseLive reports whether model and endpoint select the Gemini Live
// WebSocket API rather than non-streaming generateContent:
//   - an explicit wss://, ws://, or BidiGenerateContent endpoint uses Live;
//   - an explicit generateContent endpoint (other than BidiGenerateContent)
//     uses generateContent;
//   - otherwise, a model containing "live" (case-insensitive; empty
//     defaults to DefaultModel) uses Live, and any other model uses
//     generateContent.
func UseLive(model, endpoint string) bool {
	ep := strings.ToLower(strings.TrimSpace(endpoint))
	if ep != "" {
		if strings.Contains(ep, "bidigeneratecontent") {
			return true
		}
		if strings.Contains(ep, "generatecontent") {
			return false
		}
		if strings.HasPrefix(ep, "wss://") || strings.HasPrefix(ep, "ws://") {
			return true
		}
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultModel
	}
	return strings.Contains(strings.ToLower(model), "live")
}

// NewProvider returns a Live streamer (*Client) or a non-streaming
// generateContent transcriber (*FileClient) according to UseLive.
func NewProvider(cfg asr.Config) (asr.Provider, error) {
	if UseLive(cfg.Model, cfg.Endpoint) {
		return New(cfg)
	}
	return NewFile(cfg)
}

// Endpoint is the WebSocket URL for what the user typed, without the key:
// "" is DefaultEndpoint; https:// and http:// become wss:// and ws://; an
// empty path gets the default one; a key query parameter is dropped (the
// key is added at connect time only).
func Endpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultEndpoint
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
	if u.Path == "" || u.Path == "/" {
		u.Path, u.RawPath = defaultPath, ""
	}
	q := u.Query()
	if q.Has("key") {
		q.Del("key")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// Client is a configured streamer. Goroutine-safe.
type Client struct {
	endpoint string // without the key
	model    string
	key      string
	http     *http.Client
}

var _ asr.Streamer = (*Client)(nil)

// New checks cfg and returns a client: cfg.Key (a Gemini API key) is
// required, cfg.Model defaults to DefaultModel, cfg.Endpoint (see
// Endpoint) must be wss (ws only on loopback).
func New(cfg asr.Config) (*Client, error) {
	key := strings.TrimSpace(cfg.Key)
	if key == "" {
		return nil, asr.Errorf(ID, asr.KindConfig, "no API key")
	}
	name := strings.TrimSpace(cfg.Model)
	if name == "" {
		name = DefaultModel
	}
	ep := Endpoint(cfg.Endpoint)
	if _, err := asr.CheckEndpoint(ID, ep, "wss"); err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = asr.WSClient(cfg.Proxy)
	}
	return &Client{endpoint: ep, model: name, key: key, http: client}, nil
}

// Model is the model the client uses.
func (c *Client) Model() string { return c.model }

// Endpoint is the client's WebSocket URL, without the key.
func (c *Client) Endpoint() string { return c.endpoint }

// Caps reports a streamer that takes hot words (customVocabulary).
func (c *Client) Caps() asr.Caps { return asr.Caps{Stream: true, Hints: true} }

// Open connects, sends the setup, and returns once the service answered
// setupComplete, so that audio sent afterwards needs no buffering (the
// helper keeps recording meanwhile; Voxt buffers the first audio instead
// and sends it after the handshake).
func (c *Client) Open(ctx context.Context, req asr.StreamRequest) (asr.Stream, error) {
	if req.SampleRate <= 0 {
		req.SampleRate = asr.SampleRate
	}
	s, err := open(ctx, c, req)
	if err != nil {
		return nil, err // not a nil *stream in an interface
	}
	return s, nil
}

// connectURL is the endpoint with the key.
func (c *Client) connectURL() string {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return c.endpoint
	}
	q := u.Query()
	q.Set("key", c.key)
	u.RawQuery = q.Encode()
	return u.String()
}
