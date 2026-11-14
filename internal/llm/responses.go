package llm

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Responses is the OpenAI Responses API transport (POST {base}/responses,
// GET {base}/models), for a service that offers it (Preset.Responses:
// OpenAI, xAI, Volcengine Ark, DashScope, a custom server) and for a
// ChatGPT account's Codex backend (NewCodex). System messages become
// instructions, the others input; nothing is stored (store:false); the
// API has no stop sequences or seed, so Request.Stop and Seed are not sent.
type Responses struct {
	httpTransport
	codex bool
}

// minOutputTokens is OpenAI's smallest max_output_tokens (a lower value is
// refused; llm.Probe asks for 8).
const minOutputTokens = 16

// codexInstructions is what Codex gets when a request has no system
// message: its backend requires instructions (Voxt's).
const codexInstructions = "Process the input and return only the final answer."

// NewResponses returns a Responses API backend for p (a resolved preset).
// client nil means http.DefaultClient; secrets may be nil for a keyless
// local server.
func NewResponses(p Preset, client *http.Client, secrets Secrets) *Responses {
	return &Responses{httpTransport: httpTransport{preset: p, client: client, secrets: secrets}}
}

func (r *Responses) Info() BackendInfo {
	t := TransportResponses
	if r.codex {
		t = TransportCodex
	}
	return BackendInfo{ID: r.preset.ID, Transport: t, Local: r.preset.Local, JSONMode: r.preset.JSONMode, ThirdParty: !r.preset.Local}
}

// respOpts are the parts of a request body a refusal may relax.
type respOpts struct {
	stream, extra, store, sampling bool
}

func (r *Responses) body(req Request, model string, o respOpts) map[string]any {
	msgs := req.Messages
	if req.Format == FormatJSON {
		msgs = withJSONInstruction(msgs, req.Schema, r.preset.JSONMode != JSONSchemaMode || req.Schema == nil)
	}
	var system []string
	input := make([]oaMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == System {
			system = append(system, m.Content)
			continue
		}
		input = append(input, oaMessage{Role: string(m.Role), Content: m.Content})
	}
	b := map[string]any{}
	if o.extra {
		for k, v := range r.preset.ExtraBody {
			b[k] = v
		}
	}
	b["model"], b["input"], b["stream"] = model, input, o.stream
	if o.store {
		b["store"] = false
	}
	if instr := strings.TrimSpace(strings.Join(system, "\n\n")); instr != "" {
		b["instructions"] = instr
	} else if r.codex {
		b["instructions"] = codexInstructions
	}
	// Codex's backend takes no sampling parameters nor an output limit
	// (Voxt's capabilities for it).
	if !r.codex {
		if o.sampling && req.Temperature != nil {
			b["temperature"] = *req.Temperature
		}
		if o.sampling && req.TopP != nil {
			b["top_p"] = *req.TopP
		}
		if req.MaxTokens > 0 {
			b["max_output_tokens"] = max(req.MaxTokens, minOutputTokens)
		}
	}
	if req.Format == FormatJSON {
		switch {
		case r.preset.JSONMode == JSONSchemaMode && req.Schema != nil:
			name := req.Schema.Name
			if name == "" {
				name = "result"
			}
			b["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": name, "schema": req.Schema, "strict": false}}
		case r.preset.JSONMode == JSONSchemaMode, r.preset.JSONMode == JSONObjectMode:
			b["text"] = map[string]any{"format": map[string]any{"type": "json_object"}}
		}
	}
	if searches(r.search, req) {
		b["tools"] = []any{map[string]any{"type": "web_search"}}
	}
	return b
}

// Chat makes one attempt.
func (r *Responses) Chat(ctx context.Context, req Request) (Response, error) {
	model := req.Model
	if model == "" {
		model = r.preset.DefaultModel
	}
	if model == "" {
		return Response{}, newErr(CodeConfig, "provider", r.preset.ID, "field", "model")
	}
	start := time.Now()
	// Codex's backend only streams.
	o := respOpts{stream: req.OnDelta != nil || r.codex, extra: len(r.preset.ExtraBody) > 0, store: true, sampling: true}
	url := r.preset.BaseURL + "/responses"
	resp, err := r.send(ctx, http.MethodPost, url, r.body(req, model, o), model, req.Timeout)
	// Relax what a server rejects, once each (as OpenAI.Chat): temperature
	// / top_p (reasoning models), store (a compatible server without it)
	// and the preset's ExtraBody.
relax:
	for range 3 {
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeBadRequest {
			break
		}
		detail := strings.ToLower(e.Params["detail"])
		switch {
		case o.sampling && (req.Temperature != nil || req.TopP != nil) && (strings.Contains(detail, "temperature") || strings.Contains(detail, "top_p")):
			o.sampling = false
		case o.store && strings.Contains(detail, "store"):
			o.store = false
		case o.extra && mentionsExtra(r.preset, detail):
			o.extra = false
		default:
			break relax
		}
		resp, err = r.send(ctx, http.MethodPost, url, r.body(req, model, o), model, req.Timeout)
	}
	if err != nil {
		return Response{}, r.signInOn401(err)
	}
	defer resp.Body.Close()
	var out Response
	if o.stream {
		onDelta := req.OnDelta
		if onDelta == nil {
			onDelta = func(string) {}
		}
		out, err = r.readStream(ctx, resp.Body, onDelta, req.Timeout)
	} else {
		out, err = r.readOne(ctx, resp.Body, req.Timeout)
	}
	if err != nil {
		return Response{}, r.signInOn401(err)
	}
	if out.Model == "" {
		out.Model = model
	}
	out.Provider = r.preset.ID
	out.Latency = time.Since(start)
	if out.FinishReason == FinishFilter && out.Text == "" {
		return out, newErr(CodeFiltered, "provider", r.preset.ID)
	}
	return out, nil
}

// signInOn401 turns a refused account (a ChatGPT sign-in that expired)
// into CodeSignIn; a key's 401 stays CodeAuth.
func (r *Responses) signInOn401(err error) error {
	var e *Error
	if r.preset.Auth == AuthOAuth && errors.As(err, &e) && e.Code == CodeAuth && e.Status == http.StatusUnauthorized {
		x := newErr(CodeSignIn, "provider", r.preset.ID)
		x.Status, x.Err = e.Status, e
		return x
	}
	return err
}

// Models lists the service's models (GET {base}/models, as OpenAI's), or
// Codex's (codexModels).
func (r *Responses) Models(ctx context.Context) ([]ModelInfo, error) {
	if r.codex {
		return r.codexModels(ctx)
	}
	return (&OpenAI{httpTransport: r.httpTransport}).Models(ctx)
}
