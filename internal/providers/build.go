package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/asr/aliyun"
	"github.com/hehehai/VoxtMini/internal/asr/doubao"
	"github.com/hehehai/VoxtMini/internal/asr/gemini"
	"github.com/hehehai/VoxtMini/internal/asr/mimo"
	"github.com/hehehai/VoxtMini/internal/asr/openai"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// Env is what building a client needs besides the setup: its key and
// how to reach the network.
type Env struct {
	Key string // the key from the Keychain ("" for none)
	// Proxy is the family's network.proxy rule (nil: the environment's).
	Proxy asr.ProxyFunc
	// HTTP replaces the clients built on Proxy (tests).
	HTTP *http.Client
	// Auth signs requests in for a service with an account's sign-in
	// instead of a key (Preset.SignIn: ChatGPT for Codex).
	Auth llm.HeaderSource
}

// NewASR builds the speech client of a setup, with its advanced
// parameters applied to every request (Params).
func NewASR(p settings.Provider, env Env) (asr.Provider, error) {
	pr, ok := PresetOf(ASR, p)
	if !ok {
		return nil, asr.Errorf(p.ID, asr.KindConfig, "unknown speech service %q", p.Preset)
	}
	cfg := asr.Config{Endpoint: p.Endpoint, Model: p.Model, Key: env.Key, AppID: p.AppID, Prompt: p.Prompt, Proxy: env.Proxy, HTTPClient: env.HTTP}
	var (
		c   asr.Provider
		err error
	)
	switch pr.ID {
	case openai.ID:
		c, err = openai.New(cfg)
	case StepFunASR, GLMASR:
		if cfg.Endpoint == "" {
			cfg.Endpoint = pr.Endpoint
		}
		if cfg.Model == "" {
			cfg.Model = pr.Model
		}
		c, err = openai.New(cfg)
	case doubao.ID:
		c, err = doubao.New(cfg)
	case aliyun.ID:
		c, err = aliyun.New(cfg)
	case gemini.ID:
		c, err = gemini.New(cfg)
	case mimo.ID:
		c, err = mimo.New(cfg)
	default:
		return nil, asr.Errorf(p.ID, asr.KindConfig, "unknown speech service %q", pr.ID)
	}
	if err != nil {
		return nil, err
	}
	return withParams(c, p), nil
}

// CheckEndpoint checks the endpoint of a setup before it is saved: the
// scheme its service needs (wss for streams, https otherwise; plain only
// on loopback). "" (the default) is fine, except for a custom LLM.
func CheckEndpoint(kind Kind, p settings.Provider) error {
	pr, ok := PresetOf(kind, p)
	if !ok {
		return asr.Errorf(p.ID, asr.KindConfig, "unknown service %q", p.Preset)
	}
	if kind == LLM {
		if p.Endpoint == "" && !pr.Custom && p.API == "" {
			return nil
		}
		_, err := LLMConfig(p).Resolve()
		return err
	}
	if p.Endpoint == "" {
		return nil
	}
	switch {
	case pr.ID == gemini.ID:
		_, err := asr.CheckEndpoint(pr.ID, gemini.Endpoint(p.Endpoint), "wss")
		return err
	case pr.Mode == ModeStream:
		_, err := asr.CheckEndpoint(pr.ID, p.Endpoint, "wss")
		return err
	case pr.ID == mimo.ID:
		_, err := asr.CheckEndpoint(pr.ID, mimo.Endpoint(p.Endpoint), "https")
		return err
	default:
		_, err := asr.CheckEndpoint(pr.ID, openai.Endpoint(p.Endpoint), "https")
		return err
	}
}

// ErrExtraBody: a setup's extra body is not a JSON object.
var ErrExtraBody = errors.New("the extra body is not a JSON object")

// ExtraBody parses a setup's extra body: nil for "".
func ExtraBody(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m == nil {
		return nil, ErrExtraBody
	}
	return m, nil
}

// LLMConfig is the llm package's configuration of a setup (its extra
// body merged over the preset's; a bad one is left out: CheckSetup
// refuses it before it is saved).
func LLMConfig(p settings.Provider) llm.ProviderConfig {
	preset := p.Preset
	if preset == "" {
		preset = p.ID
	}
	extra, _ := ExtraBody(p.ExtraBody)
	return llm.ProviderConfig{ID: p.ID, Preset: preset, Name: p.Name, BaseURL: p.Endpoint, Model: p.Model, ExtraBody: extra,
		API: p.API, WebSearch: p.WebSearch}
}

// SearchSupported says whether a language setup can search the web as it
// is set up now (its API; llm.SearchSupported): the dialog's switch.
func SearchSupported(p settings.Provider) bool {
	pr, err := LLMConfig(settings.Provider{ID: p.ID, Preset: p.Preset, Endpoint: p.Endpoint}).Resolve()
	return err == nil && llm.SearchSupported(pr, p.API)
}

// NewLLM builds the language model backend of a setup, with its key.
func NewLLM(p settings.Provider, env Env) (llm.Backend, error) {
	client := env.HTTP
	if client == nil {
		client = asr.NewHTTPClient(env.Proxy, 0)
	}
	cfg := LLMConfig(p)
	cfg.Auth = env.Auth
	pr, err := cfg.Resolve()
	if err != nil {
		return nil, err
	}
	return llm.NewBackend(cfg, client, llm.StaticSecrets{pr.SecretName(): env.Key})
}

// TestResult is the outcome of a connection test.
type TestResult struct {
	Latency time.Duration
	Text    string // what the service answered (an ASR test's text of silence is usually "")
	Err     error  // nil: it works
	At      time.Time
}

// TestTimeout bounds a connection test.
const TestTimeout = 30 * time.Second

// Test checks that a setup works: for speech, a second of silence
// recognised (streamed or uploaded), which proves the key, the endpoint
// and the protocol; for a language model, one tiny request (llm.Probe).
func Test(ctx context.Context, kind Kind, p settings.Provider, env Env) TestResult {
	ctx, cancel := context.WithTimeout(ctx, TestTimeout)
	defer cancel()
	start := time.Now()
	res := TestResult{At: start}
	switch kind {
	case ASR:
		c, err := NewASR(p, env)
		if err != nil {
			res.Err = err
			return res
		}
		r, err := asr.Recognize(ctx, c, asr.Silence(time.Second), "", nil, "", asr.Feed{})
		res.Text, res.Err = r.Text, err
	case LLM:
		b, err := NewLLM(p, env)
		if err != nil {
			res.Err = err
			return res
		}
		_, err = llm.Probe(ctx, b, Model(LLM, p))
		res.Err = err
	default:
		res.Err = errors.New("unknown kind")
	}
	res.Latency = time.Since(start)
	return res
}

// TestLog is the log line of a connection test: the service, its model,
// the outcome, how long it took and, for a failure, the error (its kind
// or code, HTTP status and the service's message, clipped). Never the key
// (removed should an error quote it) nor the text recognised.
func TestLog(kind Kind, p settings.Provider, res TestResult, key string) (level, msg string) {
	what := "speech"
	if kind == LLM {
		what = "language model"
	}
	head := fmt.Sprintf("connection test of %s %s (%s, model %s)", what, p.ID, p.Preset, Model(kind, p))
	if p.Preset == "" || p.Preset == p.ID {
		head = fmt.Sprintf("connection test of %s %s (model %s)", what, p.ID, Model(kind, p))
	}
	took := res.Latency.Round(time.Millisecond)
	if res.Err == nil {
		return "info", fmt.Sprintf("%s: ok in %v", head, took)
	}
	return "warning", fmt.Sprintf("%s: failed in %v: %s", head, took, ErrorLog(res.Err, key))
}

// ErrorLog is err for the log: clipped to about 400 bytes, with key (when
// not empty) blanked out.
func ErrorLog(err error, key string) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if key = strings.TrimSpace(key); len(key) >= 4 {
		s = strings.ReplaceAll(s, key, "[key]")
	}
	return asr.Clip(s, 400)
}
