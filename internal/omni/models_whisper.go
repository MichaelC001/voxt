package omni

import "time"

// Whisper on whisper_server: the request policy of Voxt's Whisper
// (MLXInferenceConfiguration for the whisper family, MLXModelCatalog's
// capability), on the native port of Voxt's Swift Whisper. No realtime
// API: the session transcribes the recording after Stop (Voxt's
// batchPreview re-decoded the tail while recording; VoxtMini shows no
// live text for it, docs/behavior.md 3.36).

// whisperTuning is Voxt's Whisper policy:
//   - the Final's token budget is Qwen3-ASR's (Voxt's postStopFinalMaxTokens
//     for every family but Cohere); whisper_server spends it per 30 s
//     window, each within the text context (448 tokens);
//   - temperature 0 (Voxt's whisperTemperature default: greedy);
//   - the language hint only when the user speaks one language (Voxt's
//     routing for models that need no explicit language: forced, Whisper
//     writes other languages' speech in it);
//   - no context: the native server has no initial prompt, so the
//     dictionary reaches Whisper only through its aliases and the clean-up;
//   - Voxt sent the whole recording (the model cut it every 30 s, wherever
//     that fell); VoxtMini cuts it itself into requests of at most 30 s,
//     at the quietest 100 ms of their last 5 s, so that no window starts
//     or ends inside a word, and each request is one of the model's windows.
func whisperTuning() *ASRTuning {
	return &ASRTuning{
		HintAlone:       true,
		TokensPerSecond: 28, ExtraTokens: 64, MinTokens: 256, MaxTokens: 8192,
		ChunkSeconds: 30, LimitSeconds: 30, MinChunkSeconds: 1, CutSearch: 5 * time.Second, CutWindow: 100 * time.Millisecond,
	}
}
