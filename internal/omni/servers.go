package omni

// The native servers (sglang_omni_mlx/native): one binary per family of
// models, all beside qwen3_asr_server in the runtime's bin/ (scripts/
// build-omni.sh stages them), all with qwen3_asr_server's launch protocol
// and transcription API; only qwen3_asr_server has the realtime API and
// the live preview's flags. What each family's request carries is here.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The servers' binaries (Spec.Server).
const (
	ServerQwen3ASR = "qwen3_asr_server" // Qwen3-ASR, Silero VAD (--model-kind silero_vad)
	ServerWhisper  = "whisper_server"   // Whisper (MLX checkpoints + openai/whisper-* tokenizers)
)

// ErrNoServer: the runtime lacks the model's server (a runtime built
// before it was staged).
var ErrNoServer = errors.New("omni: the speech runtime has no server for this model")

// binary is spec's server: Config.Binary for qwen3_asr_server, else
// Spec.Server beside it.
func (rt *Runtime) binary(spec Spec) string {
	if spec.Server == "" || spec.Server == ServerQwen3ASR {
		return rt.cfg.Binary
	}
	return filepath.Join(filepath.Dir(rt.cfg.Binary), spec.Server)
}

// checkServer fails with ErrNoServer when spec's server is not there.
func (rt *Runtime) checkServer(spec Spec) error {
	bin := rt.binary(spec)
	if fi, err := os.Stat(bin); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: no %s beside %s (build the runtime again: scripts/build-omni.sh)", ErrNoServer, filepath.Base(bin), rt.cfg.Binary)
	}
	return nil
}

// fields are a Final request's form fields beside the file, in order:
// each family's own (the server rejects nothing it does not read, but
// sending only what it reads keeps the request what was measured).
func (r finalRequest) fields(model string) [][2]string {
	out := [][2]string{{"model", model}, {"stream", "true"}}
	add := func(k, v string) { out = append(out, [2]string{k, v}) }
	switch r.kind {
	case kindWhisper:
		// whisper_server: language (no token when it is not one of the
		// checkpoint's: it detects), max_new_tokens (per 30 s window,
		// within the text context), temperature (0: greedy, Voxt's
		// default). No prompt: the native port has no initial prompt.
		if r.language != "" {
			add("language", r.language)
		}
		if r.maxTokens > 0 {
			add("max_new_tokens", itoa(r.maxTokens))
		}
		add("temperature", ftoa(r.temperature))
		add("include_generation_metadata", "true")
	default:
		// qwen3_asr_server: Voxt's OmniMultipartBody for Qwen3-ASR.
		add("response_format", "json")
		if r.language != "" {
			add("language", r.language)
		}
		if r.context != "" {
			add("prompt", r.context)
		}
		if r.maxTokens > 0 {
			add("max_new_tokens", itoa(r.maxTokens))
		}
		add("stop_at_end_of_text", "true")
		add("stop_on_token_loop", "true")
		add("include_generation_metadata", "true")
		if r.layout != "" {
			add("audio_layout", r.layout)
		}
	}
	return out
}
