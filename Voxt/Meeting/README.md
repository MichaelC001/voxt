# Meeting

Meeting-mode runtime for live capture, transcription, diarization, translation, and session coordination.

## Responsibilities

- Coordinates meeting sessions from capture startup through live transcript updates and final output.
- Bridges microphone/system audio, remote provider live sessions, speaker labels, and meeting models.
- Keeps compatibility aliases and shared meeting contracts stable for callers and tests.

## Remote live sessions

`MeetingRemoteProviderLiveSession.swift` now contains the factory only. Base, Doubao, Aliyun Fun and Aliyun Qwen implementations have dedicated files. `BaseMeetingRemoteLiveSession` owns buffering, drain, deadline and exactly-once terminal cleanup; provider classes retain their wire-specific send/receive behavior.

A session is single-use. Finish drains queued audio before its finish signal, and its 1.8-second deadline starts at the stop request. Cancellation drops pending output; failure preserves a recoverable partial before notifying the coordinator. Terminal sessions ignore late provider events and release socket, receiver, keepalive and finish waiters once.

`MeetingRemoteAudioSupport` projects provider results into meeting segments, using the shared `Transcription/RemoteASR/DoubaoPacketCodec` for framing/decompression. Do not merge meeting utterances and dictation snapshots into one text policy.
