# App

Application orchestration layer for launch, menu bar ownership, recording entry points, and feature routing.

## Responsibilities

- Connects hotkeys, menu actions, recording sessions, transcription flows, translation, notes, and settings windows.
- Owns app-level startup policies, runtime synchronization, warmup, and development-only seeding.
- Uses AppDelegate flow extensions for coordination; substantial shared session state still lives in AppDelegate and is a refactoring boundary, not an isolated module.

## Output delivery

- `SessionOutputPreparation.swift`: normalization, dictionary correction and the prepared delivery snapshot.
- `SessionTextIO.swift`: committing, delivery destinations, history/evidence updates and answer-overlay interaction.
- `SessionTimingLogging.swift`: timing snapshots and diagnostics.
- `Recording/SessionEndFlow.swift`: session-end orchestration.

The obsolete finalize-stage runner has been removed. Preparation precedes delivery; history and dictionary evidence are updated after delivery completes.
