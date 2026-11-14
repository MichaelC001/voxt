package llm

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
)

// CodexClientVersion is the client_version Codex's model list asks with
// (Voxt's; the backend lists the models that version supports).
const CodexClientVersion = "26.506.31421"

// NewCodex returns the backend of a ChatGPT account (the "codex" preset):
// the Responses API at {base}/responses on Codex's backend
// (https://chatgpt.com/backend-api/codex), signed in by auth's headers
// (the app's ChatGPT sign-in; nil: every call is CodeSignIn). Its rules
// (Voxt's, codex-rs's): always streamed, nothing stored, instructions
// always sent, no temperature, top_p or output limit.
func NewCodex(p Preset, client *http.Client, auth HeaderSource) *Responses {
	p.Headers = maps.Clone(p.Headers)
	if p.Headers == nil {
		p.Headers = map[string]string{}
	}
	if p.Headers["Accept"] == "" {
		p.Headers["Accept"] = "text/event-stream"
	}
	p.Auth = AuthOAuth
	return &Responses{httpTransport: httpTransport{preset: p, client: client, auth: auth}, codex: true}
}

// codexModels lists the account's models (GET {base}/models?client_version=…),
// or the preset's list when the catalogue fails or is empty, as Voxt
// does. A sign-in error is returned: the list would not work either.
func (r *Responses) codexModels(ctx context.Context) ([]ModelInfo, error) {
	resp, err := r.send(ctx, http.MethodGet, r.preset.BaseURL+"/models?client_version="+CodexClientVersion, nil, "", 0)
	if err != nil {
		err = r.signInOn401(err)
		if c := Code(err); c == CodeSignIn || c == CodeCancelled {
			return nil, err
		}
		return staticModels(r.preset), nil
	}
	defer resp.Body.Close()
	var v any
	if json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&v) != nil {
		return staticModels(r.preset), nil
	}
	if ms := ParseCodexModels(v); len(ms) > 0 {
		return ms, nil
	}
	return staticModels(r.preset), nil
}

// ParseCodexModels reads Codex's model catalogue (a decoded JSON value):
// every object with a "slug" (or "id") is a model, found at any depth
// ({"models":[…]}, {"data":[…]}, categories), in order, without
// duplicates or image-only models.
func ParseCodexModels(v any) []ModelInfo {
	var out []ModelInfo
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case []any:
			for _, it := range x {
				walk(it)
			}
		case map[string]any:
			if id := firstString(x, "slug", "id"); id != "" {
				id = strings.TrimSpace(id)
				if id != "" && !seen[id] && !imageOnly(x) {
					seen[id] = true
					out = append(out, ModelInfo{ID: id, Name: strings.TrimSpace(firstString(x, "display_name", "displayName", "title", "name"))})
				}
				return
			}
			// The usual containers first, then the rest in key order.
			keys := slices.Sorted(maps.Keys(x))
			for _, k := range []string{"models", "data", "categories", "chat_models"} {
				if n, ok := x[k]; ok {
					walk(n)
				}
			}
			for _, k := range keys {
				switch k {
				case "models", "data", "categories", "chat_models":
					continue
				}
				walk(x[k])
			}
		}
	}
	walk(v)
	return out
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok {
			return s
		}
	}
	return ""
}

// imageOnly reports a model whose output modalities include images but
// not text.
func imageOnly(m map[string]any) bool {
	var mods []any
	for _, k := range []string{"output_modalities", "outputModalities"} {
		if a, ok := m[k].([]any); ok {
			mods = a
			break
		}
	}
	img, txt := false, false
	for _, x := range mods {
		s, _ := x.(string)
		switch strings.ToLower(s) {
		case "image":
			img = true
		case "text":
			txt = true
		}
	}
	return img && !txt
}
