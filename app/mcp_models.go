package main

// MCP model invocation tools: transcribe_audio, translate_text,
// rewrite_text, chat_model. Respects AllowRemote and always applies
// VoxtMini's Simplified Chinese normalization (never altering Japanese).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/compose"
	"github.com/hehehai/VoxtMini/internal/dict"
	"github.com/hehehai/VoxtMini/internal/hans"
	"github.com/hehehai/VoxtMini/internal/lang"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/mcp"
	"github.com/hehehai/VoxtMini/internal/settings"
)

func (h mcpHost) TranscribeAudio(ctx context.Context, req mcp.TranscribeRequest) (mcp.TranscribeResult, error) {
	start := time.Now()
	if h.a.local == nil {
		return mcp.TranscribeResult{}, errors.New("no speech model available")
	}
	modelID, err := h.resolveModelID(local.ASR, req.ModelID, req.AllowRemote)
	if err != nil {
		return mcp.TranscribeResult{}, err
	}
	prov, mod, err := h.a.local.ASR(ctx, modelID)
	if err != nil {
		return mcp.TranscribeResult{}, err
	}
	ft, ok := prov.(asr.FileTranscriber)
	if !ok {
		return mcp.TranscribeResult{}, fmt.Errorf("speech model %s does not support file transcription", mod.Name)
	}
	audioBytes, err := os.ReadFile(req.AudioPath)
	if err != nil {
		return mcp.TranscribeResult{}, err
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(req.AudioPath)), ".")
	if format == "" {
		format = "wav"
	}

	s := h.a.settings.Get()
	speechLang := strings.TrimSpace(req.Language)
	if speechLang == "" {
		speechLang = s.General.MainLanguage
	}
	var hints []string
	var glossary []compose.Term
	if s.Dictionary.Enabled && h.a.lib != nil && h.a.lib.store != nil {
		if terms, err := h.a.lib.store.Dictionary().List(ctx, ""); err == nil {
			if s.Dictionary.ASRHints {
				hints = dict.Hints(terms, "")
			}
			for _, t := range terms {
				if t.Enabled {
					glossary = append(glossary, compose.Term{Term: t.Term, Aliases: t.Aliases})
				}
			}
		}
	}

	asrRes, err := ft.Transcribe(ctx, asr.FileRequest{
		Audio:  audioBytes,
		Format: format,
		Lang:   speechLang,
		Others: s.General.OtherLanguages,
		Hints:  hints,
	})
	if err != nil {
		return mcp.TranscribeResult{}, err
	}
	raw := normalizeMCPText(asrRes.Text, speechLang)
	text := raw
	if s.Dictionary.Enabled && h.a.lib != nil && h.a.lib.store != nil {
		if terms, err := h.a.lib.store.Dictionary().List(ctx, ""); err == nil {
			text, _ = dict.ReplaceChanges(text, terms)
		}
	}

	if req.Cleanup && strings.TrimSpace(text) != "" {
		if llmID, lerr := h.resolveModelID(local.LLM, s.Features.Transcribe.LLM, req.AllowRemote); lerr == nil {
			if chat, _, cerr := h.a.local.LLM(ctx, llmID); cerr == nil {
				out, berr := compose.Build(compose.Input{
					Task:           compose.Transcribe,
					Cleanup:        true,
					Structure:      s.Features.Transcribe.Structure,
					TaskPrompt:     s.Features.Transcribe.Prompt,
					Glossary:       glossary,
					MainLanguage:   speechLang,
					OtherLanguages: s.General.OtherLanguages,
					Text:           text,
				})
				if berr == nil {
					if resp, cerr := chat.Chat(ctx, llm.Request{Messages: toLLMMessages(out.Messages())}); cerr == nil && strings.TrimSpace(resp.Text) != "" {
						text = normalizeMCPText(strings.TrimSpace(resp.Text), speechLang)
					}
				}
			}
		}
	}

	return mcp.TranscribeResult{
		Text:      text,
		RawText:   raw,
		Model:     mod.ID,
		Source:    modelSource(mod),
		LatencyMS: time.Since(start).Milliseconds(),
	}, nil
}

func (h mcpHost) TranslateText(ctx context.Context, req mcp.TranslateRequest) (mcp.TranslateResult, error) {
	start := time.Now()
	chat, mod, err := h.openLLM(ctx, req.ModelID, req.AllowRemote)
	if err != nil {
		return mcp.TranslateResult{}, err
	}
	s := h.a.settings.Get()
	glossary := h.mcpGlossary(ctx, s)
	built, err := compose.Build(compose.Input{
		Task:           compose.Translate,
		Selected:       true,
		TaskPrompt:     s.Features.Translate.Prompt,
		Glossary:       glossary,
		MainLanguage:   s.General.MainLanguage,
		OtherLanguages: s.General.OtherLanguages,
		TargetLanguage: req.TargetLanguage,
		Text:           req.Text,
	})
	if err != nil {
		return mcp.TranslateResult{}, err
	}
	resp, err := chat.Chat(ctx, llm.Request{Messages: toLLMMessages(built.Messages())})
	if err != nil {
		return mcp.TranslateResult{}, err
	}
	out := normalizeMCPText(strings.TrimSpace(resp.Text), req.TargetLanguage)
	return mcp.TranslateResult{
		Text:           out,
		TargetLanguage: req.TargetLanguage,
		Model:          mod.ID,
		Source:         modelSource(mod),
		LatencyMS:      time.Since(start).Milliseconds(),
	}, nil
}

func (h mcpHost) RewriteText(ctx context.Context, req mcp.RewriteRequest) (mcp.RewriteResult, error) {
	start := time.Now()
	chat, mod, err := h.openLLM(ctx, req.ModelID, req.AllowRemote)
	if err != nil {
		return mcp.RewriteResult{}, err
	}
	s := h.a.settings.Get()
	glossary := h.mcpGlossary(ctx, s)
	built, err := compose.Build(compose.Input{
		Task:           compose.Rewrite,
		Selected:       strings.TrimSpace(req.Text) != "",
		TaskPrompt:     s.Features.Rewrite.Prompt,
		Glossary:       glossary,
		MainLanguage:   s.General.MainLanguage,
		OtherLanguages: s.General.OtherLanguages,
		Text:           req.Text,
		Instruction:    req.Instruction,
	})
	if err != nil {
		return mcp.RewriteResult{}, err
	}
	resp, err := chat.Chat(ctx, llm.Request{Messages: toLLMMessages(built.Messages())})
	if err != nil {
		return mcp.RewriteResult{}, err
	}
	out := strings.TrimSpace(resp.Text)
	if ans, ok := compose.ParseAnswer(out); ok && strings.TrimSpace(ans.Content) != "" {
		out = strings.TrimSpace(ans.Content)
	}
	out = normalizeMCPText(out, s.General.MainLanguage)
	return mcp.RewriteResult{
		Text:      out,
		Model:     mod.ID,
		Source:    modelSource(mod),
		LatencyMS: time.Since(start).Milliseconds(),
	}, nil
}

func (h mcpHost) ChatModel(ctx context.Context, req mcp.ChatRequest) (mcp.ChatResult, error) {
	start := time.Now()
	chat, mod, err := h.openLLM(ctx, req.ModelID, req.AllowRemote)
	if err != nil {
		return mcp.ChatResult{}, err
	}
	var msgs []llm.Message
	if strings.TrimSpace(req.System) != "" {
		msgs = append(msgs, llm.Message{Role: llm.System, Content: req.System})
	}
	msgs = append(msgs, llm.Message{Role: llm.User, Content: req.Prompt})
	resp, err := chat.Chat(ctx, llm.Request{Messages: msgs})
	if err != nil {
		return mcp.ChatResult{}, err
	}
	s := h.a.settings.Get()
	out := normalizeMCPText(strings.TrimSpace(resp.Text), s.General.MainLanguage)
	return mcp.ChatResult{
		Text:      out,
		Model:     mod.ID,
		Source:    modelSource(mod),
		LatencyMS: time.Since(start).Milliseconds(),
	}, nil
}

func (h mcpHost) openLLM(ctx context.Context, wantID string, allowRemote bool) (local.Chat, local.Model, error) {
	if h.a.local == nil {
		return nil, local.Model{}, errors.New("no language model available")
	}
	id, err := h.resolveModelID(local.LLM, wantID, allowRemote)
	if err != nil {
		return nil, local.Model{}, err
	}
	return h.a.local.LLM(ctx, id)
}

func (h mcpHost) resolveModelID(kind local.Kind, wantID string, allowRemote bool) (string, error) {
	wantID = strings.TrimSpace(wantID)
	if wantID != "" {
		if _, isRem := settings.IsRemote(wantID); isRem && !allowRemote {
			return "", mcp.ErrRemoteOff
		}
		return wantID, nil
	}
	// Automatic: always pick a local model first.
	for _, m := range h.a.local.Installed(kind) {
		if !m.Remote {
			return m.ID, nil
		}
	}
	return "", local.NotInstalledError(kind, "")
}

func (h mcpHost) mcpGlossary(ctx context.Context, s settings.Settings) []compose.Term {
	if !s.Dictionary.Enabled || h.a.lib == nil || h.a.lib.store == nil {
		return nil
	}
	terms, err := h.a.lib.store.Dictionary().List(ctx, "")
	if err != nil {
		return nil
	}
	out := make([]compose.Term, 0, len(terms))
	for _, t := range terms {
		if t.Enabled {
			out = append(out, compose.Term{Term: t.Term, Aliases: t.Aliases})
		}
	}
	return out
}

func toLLMMessages(in []compose.Message) []llm.Message {
	out := make([]llm.Message, len(in))
	for i, m := range in {
		out[i] = llm.Message{Role: llm.Role(m.Role), Content: m.Content}
	}
	return out
}

func normalizeMCPText(text, langCode string) string {
	if lang.Canonical(langCode) == lang.Japanese {
		return text
	}
	return hans.Simplify(text)
}
