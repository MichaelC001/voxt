package mcp

// Model invocation tools (require Policy.AllowLocalModels; remote models
// additionally require Policy.AllowRemoteModels).

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const maxAudioBytes = 50 << 20 // 50 MiB

var allowedAudioExts = []string{".wav", ".m4a", ".mp3", ".flac", ".ogg"}

func modelTools() []tool {
	tTranscribe := def("transcribe_audio", "Transcribe a local audio file (.wav/.m4a/.mp3/.flac/.ogg, <=50 MB) using VoxtMini's speech pipeline.", ScopeModels,
		func(c *call, in struct {
			AudioPath string `json:"audio_path" jsonschema:"Absolute path to local audio file"`
			ModelID   string `json:"model_id,omitempty" jsonschema:"Optional model id (default: automatic local)"`
			Cleanup   bool   `json:"cleanup,omitempty" jsonschema:"Run LLM cleanup after recognition"`
			Language  string `json:"language,omitempty" jsonschema:"Optional language hint: en, zh, or ja"`
		}) (any, error) {
			resolved, err := c.srv.validateAudioPath(in.AudioPath)
			if err != nil {
				return nil, err
			}
			if err := c.checkModelAllowed(in.ModelID, "speech"); err != nil {
				return nil, err
			}
			if err := c.srv.gate.confirm(c, AskModel, "Transcribe local audio file", 1, false); err != nil {
				return nil, err
			}
			res, err := c.srv.opt.Host.TranscribeAudio(c.ctx, TranscribeRequest{
				AudioPath:   resolved,
				ModelID:     strings.TrimSpace(in.ModelID),
				Cleanup:     in.Cleanup,
				Language:    strings.TrimSpace(in.Language),
				AllowRemote: c.policy.AllowRemoteModels,
			})
			if err != nil {
				return nil, err
			}
			c.count = 1
			return res, nil
		})
	tTranscribe.readOnly = true

	tTranslate := def("translate_text", "Translate text using VoxtMini's translation pipeline (en, zh/zh-Hans, ja).", ScopeModels,
		func(c *call, in struct {
			Text           string `json:"text" jsonschema:"Source text to translate"`
			TargetLanguage string `json:"target_language" jsonschema:"Target language: en, zh, zh-Hans, or ja"`
			ModelID        string `json:"model_id,omitempty" jsonschema:"Optional model id (default: automatic local)"`
		}) (any, error) {
			if strings.TrimSpace(in.Text) == "" {
				return nil, fail("bad_arguments", "text is required")
			}
			lang, ok := normalizeTargetLang(in.TargetLanguage)
			if !ok {
				return nil, fail("bad_arguments", "target_language must be en, zh, zh-Hans, or ja")
			}
			if err := c.checkModelAllowed(in.ModelID, "language"); err != nil {
				return nil, err
			}
			if err := c.srv.gate.confirm(c, AskModel, "Translate text with VoxtMini model", 1, false); err != nil {
				return nil, err
			}
			res, err := c.srv.opt.Host.TranslateText(c.ctx, TranslateRequest{
				Text:           in.Text,
				TargetLanguage: lang,
				ModelID:        strings.TrimSpace(in.ModelID),
				AllowRemote:    c.policy.AllowRemoteModels,
			})
			if err != nil {
				return nil, err
			}
			c.count = 1
			return res, nil
		})
	tTranslate.readOnly = true

	tRewrite := def("rewrite_text", "Rewrite or generate text following an instruction using VoxtMini's rewrite pipeline.", ScopeModels,
		func(c *call, in struct {
			Instruction string `json:"instruction" jsonschema:"Editing or generation instruction"`
			Text        string `json:"text,omitempty" jsonschema:"Optional source text to rewrite"`
			ModelID     string `json:"model_id,omitempty" jsonschema:"Optional model id (default: automatic local)"`
		}) (any, error) {
			if strings.TrimSpace(in.Instruction) == "" {
				return nil, fail("bad_arguments", "instruction is required")
			}
			if err := c.checkModelAllowed(in.ModelID, "language"); err != nil {
				return nil, err
			}
			if err := c.srv.gate.confirm(c, AskModel, "Rewrite text with VoxtMini model", 1, false); err != nil {
				return nil, err
			}
			res, err := c.srv.opt.Host.RewriteText(c.ctx, RewriteRequest{
				Instruction: in.Instruction,
				Text:        in.Text,
				ModelID:     strings.TrimSpace(in.ModelID),
				AllowRemote: c.policy.AllowRemoteModels,
			})
			if err != nil {
				return nil, err
			}
			c.count = 1
			return res, nil
		})
	tRewrite.readOnly = true

	tChat := def("chat_model", "Run a single-turn completion on a VoxtMini language model.", ScopeModels,
		func(c *call, in struct {
			Prompt  string `json:"prompt" jsonschema:"User prompt"`
			System  string `json:"system,omitempty" jsonschema:"Optional system prompt"`
			ModelID string `json:"model_id,omitempty" jsonschema:"Optional model id (default: automatic local)"`
		}) (any, error) {
			if strings.TrimSpace(in.Prompt) == "" {
				return nil, fail("bad_arguments", "prompt is required")
			}
			if err := c.checkModelAllowed(in.ModelID, "language"); err != nil {
				return nil, err
			}
			if err := c.srv.gate.confirm(c, AskModel, "Call VoxtMini language model", 1, false); err != nil {
				return nil, err
			}
			res, err := c.srv.opt.Host.ChatModel(c.ctx, ChatRequest{
				Prompt:      in.Prompt,
				System:      in.System,
				ModelID:     strings.TrimSpace(in.ModelID),
				AllowRemote: c.policy.AllowRemoteModels,
			})
			if err != nil {
				return nil, err
			}
			c.count = 1
			return res, nil
		})
	tChat.readOnly = true

	return []tool{tTranscribe, tTranslate, tRewrite, tChat}
}

// checkModelAllowed enforces Policy.AllowRemoteModels if modelID refers to
// a remote model known to Host.ListModels.
func (c *call) checkModelAllowed(modelID, kind string) error {
	id := strings.TrimSpace(modelID)
	if id == "" || c.policy.AllowRemoteModels {
		return nil
	}
	models, err := c.srv.opt.Host.ListModels(c.ctx, kind)
	if err != nil {
		return nil
	}
	for _, m := range models {
		if m.ID == id && strings.EqualFold(m.Source, "remote") {
			return fail("remote_off", ErrRemoteOff.Error())
		}
	}
	return nil
}

// validateAudioPath ensures rawPath points to an existing regular audio file
// (<=50 MiB, allowed extension) strictly inside AllowedAudioRoots.
func (s *Server) validateAudioPath(rawPath string) (string, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" || !filepath.IsAbs(p) {
		return "", fail("invalid_path", "audio_path must be an absolute path")
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fail("not_found", "audio file not found")
		}
		return "", fail("invalid_path", "audio_path cannot be resolved")
	}
	if !s.underAllowedRoots(resolved) {
		return "", fail("invalid_path", "audio_path must be inside an allowed folder (home or temp directory)")
	}
	ext := strings.ToLower(filepath.Ext(resolved))
	if !slices.Contains(allowedAudioExts, ext) {
		return "", fail("invalid_path", "unsupported audio format; allowed: .wav, .m4a, .mp3, .flac, .ogg")
	}
	fi, err := os.Stat(resolved)
	if err != nil {
		return "", fail("invalid_path", "audio file cannot be read")
	}
	if !fi.Mode().IsRegular() {
		return "", fail("invalid_path", "audio_path must be a regular file")
	}
	if fi.Size() == 0 {
		return "", fail("invalid_path", "audio file is empty")
	}
	if fi.Size() > maxAudioBytes {
		return "", fail("too_large", "audio file exceeds 50 MB limit")
	}
	return resolved, nil
}

func (s *Server) underAllowedRoots(resolved string) bool {
	sep := string(filepath.Separator)
	for _, root := range s.opt.AllowedAudioRoots {
		if root == "" {
			continue
		}
		rr, err := filepath.EvalSymlinks(root)
		if err != nil {
			rr = filepath.Clean(root)
		}
		prefix := rr
		if !strings.HasSuffix(prefix, sep) {
			prefix += sep
		}
		if strings.HasPrefix(resolved, prefix) {
			return true
		}
	}
	return false
}

func normalizeTargetLang(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "en", "en-us", "en-gb":
		return "en", true
	case "zh", "zh-hans", "zh-cn":
		return "zh-Hans", true
	case "ja", "ja-jp":
		return "ja", true
	default:
		return "", false
	}
}
