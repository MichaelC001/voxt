# Meeting Processing

Post-capture meeting processing for ASR, translation, transcript assembly, and final summaries.

## Responsibilities

- Transcribes meeting segments and combines them into coherent final transcript output.
- Smooths speaker turns, assembles speaker-aware transcript text, and prepares final meeting records.
- Applies translation and summary support after audio capture is complete.

## File import and finalization

`MeetingImportedFileAnalyzer` registers import work before awaiting the previous live-session cleanup. Cancellation captures that operation rather than looking up a potentially newer task later. It remains busy through cleanup and cancels abandoned operations.

`MeetingFileTaskQueue` admits bounded pending imports and serializes preprocessing with file analysis. It streams directly from the authorized source into a versioned canonical WAV cache instead of first copying an entire video. The cache is published only after validation and atomic rename; retries reuse it and v1 raw staged tasks are upgraded lazily. The original 4 GiB source limit remains until large-container stress tests justify raising it.

`MeetingFilePreparationLimits` bounds decoded duration/output, conversion buffers and disk usage. `MeetingFilePreparationResources` waits at checkpoints under memory/thermal pressure. AVFoundation's internal allocations are not covered by the application buffer ceiling.

`MeetingFileAnalysisCheckpointStore` is the intentionally small P2 recovery slice: after each imported-audio descriptor, it atomically stores the committed ASR segments, completed descriptor count, prepared-audio sample count and model fingerprint. A retry/restart resumes at the next descriptor only when input and model identity match; otherwise the checkpoint is discarded. It does not persist model tensors, token streams or speaker-analysis state.

File speaker analysis now uses an operation-owned Sortformer engine with bounded, frame-aligned feeds (at most five seconds and no more than the configured AOSC retirement budget). It preserves streaming identity, validates FIFO/cache lengths after each feed, and obtains a low-priority file permit per feed rather than holding one for the whole recording. Errors propagate so the completed ASR checkpoint is retained instead of silently saving a transcript-only success. The file page can preview/copy the completed checkpoint without opening a history model or starting summary generation. See [long-file repair and pending validation](../../../docs/SortformerLongFileRepair.zh-CN.md).

`MeetingImportedFilePipeline` owns one import's transcriber, temporary audio and model use; it does not mutate live coordinator engine/transcriber fields. Prepared queue inputs use a bounded independent archive copy, not another decode, because history storage moves its input. Failure/cancellation discards only that operation's temporary output, never the queue's reusable cache; success transfers the temporary archive to history persistence.

Implementation details, remaining limitations and pending macOS acceptance: [File preprocessing resource safety](../../../docs/FilePreprocessingResourceSafety.zh-CN.md).

`MeetingFinalizationContext` snapshots stop-time identity, engine/model metadata, duration and visible segments for all recovery checkpoints. The live coordinator remains occupied until final checkpoint cleanup finishes, and repeated stop calls return the same task.
