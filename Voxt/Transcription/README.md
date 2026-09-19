# Transcription

Transcription engine adapters, local model managers, model artifacts, and shared transcriber contracts.

## Responsibilities

- Implements MLX, Speech, remote ASR, and legacy Whisper migration integration points.
- Manages local ASR model discovery, downloads, repository state, and artifact validation.
- Provides common transcriber protocols, support types, and post-processing for transcript text.

## MLX boundaries

`MLXTranscriber` owns recording tasks, revision checks, model leases and capture-buffer instances. Pure policy is in `MLXTranscriptionPlanning`, `MLXTranscriptMerging`, and `MLXLiveTextPreview`. Shared values, buffers, detached inference and structured segment conversion live in their correspondingly named files.

Keep `nonisolated` inference, cancellation propagation and the existing buffer locks intact when moving code. The source split does not introduce another model/session owner. See the [phase record](../../docs/RefactoringProgress.zh-CN.md) before changing runtime ownership.
