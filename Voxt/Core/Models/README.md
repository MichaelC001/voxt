# Models

Model metadata, storage, download, debug, and custom model support shared by transcription and settings.

## Responsibilities

- Tracks local model locations, remote model configuration, and model-specific metadata.
- Supports custom LLM model downloads, validation, and installation-state reporting.
- Provides debug helpers and storage directory rules used by model management UI.

`SharedModelLoadCoordinator<Value>` coalesces current waiters with typed results. Invalidated/cancelled loads remain outstanding until their tasks exit; subsequent `cancelAll()` calls still return their completion barriers. `hasPendingLoad` describes shareable entries, while `hasOutstandingLoad` also covers retiring work and is the idle-reclamation guard.

ASR and Custom LLM managers share one complete shutdown task per instance, so repeated shutdown callers wait for the same download/load/use cleanup. Swift task completion is not a proof of native Metal worker quiescence.
