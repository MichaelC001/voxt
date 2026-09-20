import Foundation

/// Resource owner for one file import. It never mutates the live meeting's
/// transcriber, engine selection or model-use fields.
@MainActor
final class MeetingImportedFilePipeline: MeetingImportedFileAnalyzing {
    private let modelManager: MLXModelManager
    private let engineContext: MeetingASREngineContext
    private let sourceIsPreparedAudio: Bool
    private var transcriber: (any MeetingSegmentTranscribing)?
    private var holdsModelUse = false
    private var preparedAudioURL: URL?

    init(modelManager: MLXModelManager, engineContext: MeetingASREngineContext, sourceIsPreparedAudio: Bool = false) {
        self.modelManager = modelManager
        self.engineContext = engineContext
        self.sourceIsPreparedAudio = sourceIsPreparedAudio
    }

    func analyze(
        at sourceURL: URL,
        progress: @escaping @MainActor @Sendable (MeetingFileAnalysisProgress) -> Void
    ) async throws -> MeetingSessionResult {
        try Task.checkCancellation()
        progress(MeetingFileAnalysisProgress(stage: .preparing, stageFraction: sourceIsPreparedAudio ? 1 : 0))
        let startedAt = ContinuousClock.now
        var stage = "archive-preparation"
        MeetingFileTrace.event("pipeline-started", "preparedInput=\(sourceIsPreparedAudio), engine=\(engineContext.engine.rawValue)")
        do {
            let sourceIsPreparedAudio = sourceIsPreparedAudio
            let traceTaskID = MeetingFileTrace.taskID
            let preparationTask = Task.detached(priority: .utility) {
                try await MeetingFileTrace.$taskID.withValue(traceTaskID) {
                    if sourceIsPreparedAudio {
                        return try await MeetingImportedAudioFile.copyPreparedForAnalysis(from: sourceURL)
                    }
                    return try await MeetingImportedAudioFile.prepare(from: sourceURL) { fraction in
                        await progress(
                            MeetingFileAnalysisProgress(
                                stage: .preparing,
                                stageFraction: fraction
                            )
                        )
                    }
                }
            }
            let importedAudio = try await withTaskCancellationHandler {
                try await preparationTask.value
            } onCancel: {
                preparationTask.cancel()
            }
            preparedAudioURL = importedAudio.standardizedAudioURL
            try Task.checkCancellation()

            progress(
                MeetingFileAnalysisProgress(
                    stage: .preparing,
                    stageFraction: 1,
                    mediaDurationSeconds: importedAudio.durationSeconds
                )
            )

            stage = "transcribing"
            MeetingFileTrace.event("transcription-started", "durationSeconds=\(importedAudio.durationSeconds), windows=\(importedAudio.assetDescriptors.count)")
            progress(MeetingFileAnalysisProgress(stage: .transcribing))
            let importedTranscriber = try makeTranscriber()
            transcriber = importedTranscriber
            try Task.checkCancellation()

            let transcriptSegments = try await MeetingFinalTranscriptionPass.transcribe(
                descriptors: importedAudio.assetDescriptors,
                loadAsset: { descriptor in
                    importedAudio.loadAsset(descriptor)
                },
                transcriber: importedTranscriber,
                requiresCompleteTranscription: true,
                processedDurationProgress: { fraction, processedDuration in
                    await progress(
                        MeetingFileAnalysisProgress(
                            stage: .transcribing,
                            stageFraction: fraction,
                            mediaDurationSeconds: importedAudio.durationSeconds,
                            processedMediaDurationSeconds: processedDuration
                        )
                    )
                }
            )
            try Task.checkCancellation()
            MeetingFileTrace.event("transcription-completed", "segments=\(transcriptSegments.count), elapsed=\(startedAt.duration(to: .now))")
            guard !MeetingTranscriptFormatter.meaningfulSegments(for: transcriptSegments).isEmpty else {
                throw MeetingFileAnalysisError.noTranscript
            }

            stage = "identifyingSpeakers"
            MeetingFileTrace.event("speaker-analysis-requested", "segments=\(transcriptSegments.count)")
            progress(MeetingFileAnalysisProgress(stage: .identifyingSpeakers))
            let finalSegments: [MeetingTranscriptSegment]
            do {
                finalSegments = try await MeetingLocalInferenceCoordinator.shared.withPermit(.speakerAnalysis) {
                    MeetingFileTrace.event("speaker-analysis-admitted")
                    return await MeetingSpeakerAnalysisPipeline.analyzedSegments(
                        from: transcriptSegments,
                        descriptors: importedAudio.assetDescriptors,
                        loadAsset: { descriptor in
                            importedAudio.loadAsset(descriptor)
                        },
                        continuousAudioURL: importedAudio.standardizedAudioURL,
                        options: MeetingSpeakerDiarizationOptions.fromPreferences(),
                        progress: { fraction in
                            await progress(
                                MeetingFileAnalysisProgress(
                                    stage: .identifyingSpeakers,
                                    stageFraction: fraction
                                )
                            )
                        }
                    )
                }
            } catch {
                VoxtLog.meetingWarning(
                    "Imported meeting speaker analysis skipped by device safety policy: \(error.localizedDescription)"
                )
                MeetingFileTrace.event("speaker-analysis-fallback", MeetingFileTaskDiagnostics.errorSummary(error))
                finalSegments = MeetingTranscriptPostProcessor.process(transcriptSegments)
            }
            try Task.checkCancellation()

            MeetingFileTrace.event("speaker-analysis-returned", "segments=\(finalSegments.count)")
            stage = "saving"
            progress(MeetingFileAnalysisProgress(stage: .saving))
            let result = MeetingSessionResult(
                captureMode: .meeting,
                transcriptionEngine: engineContext.engine,
                transcriptionModelDescription: engineContext.historyModelDescription,
                segments: finalSegments,
                visibleSnapshotSegments: finalSegments,
                audioDurationSeconds: importedAudio.durationSeconds,
                archivedAudioURL: importedAudio.standardizedAudioURL
            )
            MeetingFileTrace.event("pipeline-result-ready", "elapsed=\(startedAt.duration(to: .now)), segments=\(finalSegments.count)")
            return result
        } catch {
            MeetingFileTrace.event("pipeline-stopped", "stage=\(stage), elapsed=\(startedAt.duration(to: .now)), \(MeetingFileTaskDiagnostics.errorSummary(error))")
            if let preparedAudioURL {
                try? FileManager.default.removeItem(at: preparedAudioURL)
            }
            throw error
        }
    }

    private func makeTranscriber() throws -> any MeetingSegmentTranscribing {
        switch engineContext.engine {
        case .mlxAudio:
            modelManager.beginActiveUse()
            holdsModelUse = true
            return MeetingMLXSegmentTranscriber(modelManager: modelManager, strictInferenceWorkClass: .fileASR)
        case .remote:
            return MeetingRemoteASRSegmentTranscriber()
        case .dictation:
            throw NSError(domain: "Voxt.Meeting", code: -1, userInfo: [NSLocalizedDescriptionKey: "Direct Dictation is not supported for Meeting Notes."])
        }
    }

    func cancel() async {
        await transcriber?.cancelPendingWork()
    }

    func finish(keepingResult: Bool) async {
        MeetingFileTrace.event("pipeline-cleanup-started", "keepingResult=\(keepingResult)")
        defer { MeetingFileTrace.event("pipeline-cleanup-completed", "keepingResult=\(keepingResult), cancelled=\(Task.isCancelled)") }
        await transcriber?.cancelPendingWork()
        transcriber = nil
        if holdsModelUse {
            holdsModelUse = false
            modelManager.endActiveUse()
        }
        if !keepingResult || Task.isCancelled, let preparedAudioURL {
            try? FileManager.default.removeItem(at: preparedAudioURL)
            self.preparedAudioURL = nil
        }
    }
}
