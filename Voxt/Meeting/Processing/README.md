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

`MeetingImportedFilePipeline` owns one import's transcriber, temporary audio and model use; it does not mutate live coordinator engine/transcriber fields. Prepared queue inputs use a bounded independent archive copy, not another decode, because history storage moves its input. Failure/cancellation discards only that operation's temporary output, never the queue's reusable cache; success transfers the temporary archive to history persistence.

Implementation details, remaining limitations and pending macOS acceptance: [File preprocessing resource safety](../../../docs/FilePreprocessingResourceSafety.zh-CN.md).

`MeetingFinalizationContext` snapshots stop-time identity, engine/model metadata, duration and visible segments for all recovery checkpoints. The live coordinator remains occupied until final checkpoint cleanup finishes, and repeated stop calls return the same task.
