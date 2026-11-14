package providers

// The advanced parameters of a setup (settings.Provider), applied to each
// request a session makes.

import (
	"context"
	"strings"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// LanguageAuto is settings.Provider.Language's "no hint": the service
// detects the language.
const LanguageAuto = "auto"

// speechParams adjusts a speech request: the language hint (the setup's
// over the session's; "auto" none) and the dictionary's hints (none with
// NoHints).
func speechParams(p settings.Provider, lang string, hints []string) (string, []string) {
	switch l := strings.TrimSpace(p.Language); l {
	case "":
	case LanguageAuto:
		lang = ""
	default:
		lang = l
	}
	if p.NoHints {
		hints = nil
	}
	return lang, hints
}

type fileParams struct {
	asr.FileTranscriber
	p settings.Provider
}

func (f fileParams) Transcribe(ctx context.Context, req asr.FileRequest) (asr.Result, error) {
	req.Lang, req.Hints = speechParams(f.p, req.Lang, req.Hints)
	return f.FileTranscriber.Transcribe(ctx, req)
}

type streamParams struct {
	asr.Streamer
	p settings.Provider
}

func (s streamParams) Open(ctx context.Context, req asr.StreamRequest) (asr.Stream, error) {
	req.Lang, req.Hints = speechParams(s.p, req.Lang, req.Hints)
	return s.Streamer.Open(ctx, req)
}

// withParams wraps a speech client so that its requests get the setup's
// parameters. A client is a file transcriber or a streamer.
func withParams(c asr.Provider, p settings.Provider) asr.Provider {
	if strings.TrimSpace(p.Language) == "" && !p.NoHints {
		return c
	}
	switch v := c.(type) {
	case asr.Streamer:
		return streamParams{Streamer: v, p: p}
	case asr.FileTranscriber:
		return fileParams{FileTranscriber: v, p: p}
	}
	return c
}

// ChatParams sets a language request's parameters from the setup: its
// model, and its temperature, top-p and output budget over the
// session's. The extra body is the backend's (LLMConfig).
func ChatParams(p settings.Provider, req *llm.Request) {
	req.Model = Model(LLM, p)
	if p.Temperature != nil {
		req.Temperature = llm.F64(*p.Temperature)
	}
	if p.TopP != nil {
		req.TopP = llm.F64(*p.TopP)
	}
	if p.MaxTokens > 0 {
		req.MaxTokens = p.MaxTokens
	}
}
