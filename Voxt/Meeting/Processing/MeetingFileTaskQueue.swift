// MeetingFileTaskQueue.swift
// Provides persistent, serial processing for imported meeting files.

import Combine
import Foundation

enum MeetingFileTaskStatus: String, Codable, Hashable, Sendable {
    case queued
    case preparing
    case processing
    case waitingForResources
    case cancelling
    case completed
    case failed
    case cancelled

    var isTerminal: Bool {
        switch self {
        case .completed, .failed, .cancelled:
            return true
        case .queued, .preparing, .processing, .waitingForResources, .cancelling:
            return false
        }
    }
}

struct MeetingFileTask: Identifiable, Codable, Hashable, Sendable {
    let id: UUID
    let fileName: String
    var stagedFileName: String
    // Optional for v1 task migration. Publish only complete canonical WAVs.
    var preparedAudioVersion: Int? = nil
    var legacyStagedFileName: String? = nil
    let enqueuedAt: Date
    var startedAt: Date?
    var completedAt: Date?
    var status: MeetingFileTaskStatus
    var progressStage: MeetingFileAnalysisStage
    var progressFraction: Double
    var mediaDurationSeconds: TimeInterval?
    var processedMediaDurationSeconds: TimeInterval?
    /// Smoothed audio-seconds-per-wall-second measured during transcription.
    var processingSpeedSecondsPerSecond: Double?
    var speedSampleAt: Date?
    var speedSampleProcessedMediaDurationSeconds: TimeInterval?
    /// Conservative total-duration estimate captured while the task is running.
    /// It is intentionally monotonic: later progress observations may lower it,
    /// but never make it larger and cause the UI to oscillate.
    var estimatedTotalSeconds: TimeInterval?
    var errorMessage: String?
    var historyEntryID: UUID?

    var isTerminal: Bool { status.isTerminal }

    func elapsedSeconds(now: Date) -> TimeInterval {
        guard let startedAt else { return 0 }
        let end = completedAt ?? now
        return max(0, end.timeIntervalSince(startedAt))
    }

    func estimatedRemainingSeconds(now: Date) -> TimeInterval? {
        guard status == .processing else { return nil }
        let elapsed = elapsedSeconds(now: now)
        guard elapsed > 0 else { return nil }
        if let estimatedTotalSeconds, estimatedTotalSeconds > elapsed {
            return max(0, estimatedTotalSeconds - elapsed)
        }

        // If an old estimate was too optimistic and has already elapsed,
        // fall back to the current overall progress instead of displaying
        // zero while the task is still processing.
        guard progressFraction > 0 else { return nil }
        return max(0, elapsed * (1 - progressFraction) / progressFraction)
    }

    static func updatedEstimatedTotalSeconds(
        current: TimeInterval?,
        elapsed: TimeInterval,
        progressFraction: Double,
        mediaDurationSeconds: TimeInterval? = nil,
        processedMediaDurationSeconds: TimeInterval? = nil,
        stage: MeetingFileAnalysisStage? = nil,
        processingSpeed: Double? = nil
    ) -> TimeInterval? {
        guard elapsed > 0, progressFraction > 0 else { return current }

        // The first estimate is deliberately conservative. Once established,
        // only a faster observed rate can lower it; a slower phase never makes
        // the remaining-time label jump backwards.
        let observedTotal: TimeInterval
        if stage == nil || stage == .transcribing || stage == .identifyingSpeakers || stage == .saving,
           let mediaDurationSeconds,
           let processedMediaDurationSeconds,
           mediaDurationSeconds > 0,
           processedMediaDurationSeconds > 0 {
            let sampleSpeed = max(
                processingSpeed ?? processedMediaDurationSeconds / elapsed,
                0.001
            )
            let transcriptionTotal = mediaDurationSeconds / sampleSpeed
            // Transcription accounts for 63% of the overall task progress.
            // Include the later speaker-analysis and save stages so finishing
            // the audio pass does not make the task appear to have no time left.
            let transcriptionWeight = 0.63
            observedTotal = max(
                transcriptionTotal / transcriptionWeight,
                elapsed / progressFraction
            )
        } else {
            observedTotal = elapsed / progressFraction
        }
        let conservativeTotal = max(
            observedTotal * 1.35,
            elapsed + 30
        )
        // Treat a legacy or corrupted zero anchor as missing so it can be
        // recovered instead of permanently winning the minimum comparison.
        guard let current, current > 0 else { return conservativeTotal }
        return min(current, conservativeTotal)
    }

    static func queued(
        id: UUID = UUID(),
        fileName: String,
        stagedFileName: String,
        enqueuedAt: Date = Date()
    ) -> MeetingFileTask {
        MeetingFileTask(
            id: id,
            fileName: fileName,
            stagedFileName: stagedFileName,
            enqueuedAt: enqueuedAt,
            startedAt: nil,
            completedAt: nil,
            status: .queued,
            progressStage: .preparing,
            progressFraction: 0,
            mediaDurationSeconds: nil,
            processedMediaDurationSeconds: nil,
            processingSpeedSecondsPerSecond: nil,
            speedSampleAt: nil,
            speedSampleProcessedMediaDurationSeconds: nil,
            estimatedTotalSeconds: nil,
            errorMessage: nil,
            historyEntryID: nil
        )
    }

    func resetForRetry() -> MeetingFileTask {
        var retry = self
        retry.startedAt = nil
        retry.completedAt = nil
        retry.status = .queued
        retry.progressStage = .preparing
        retry.progressFraction = 0
        retry.processedMediaDurationSeconds = nil
        retry.processingSpeedSecondsPerSecond = nil
        retry.speedSampleAt = nil
        retry.speedSampleProcessedMediaDurationSeconds = nil
        retry.estimatedTotalSeconds = nil
        retry.errorMessage = nil
        retry.historyEntryID = nil
        return retry
    }
}

@MainActor
final class MeetingFileTaskQueue: ObservableObject {
    typealias Analyzer = @MainActor @Sendable (
        _ sourceURL: URL,
        _ originalFileName: String,
        _ progress: @escaping @MainActor @Sendable (MeetingFileAnalysisProgress) -> Void
    ) async throws -> TranscriptionHistoryEntry
    typealias Preparer = @Sendable (
        _ sourceURL: URL,
        _ destinationURL: URL,
        _ limits: MeetingFilePreparationLimits,
        _ checkpoint: @escaping @Sendable () async throws -> Void,
        _ progress: @escaping @Sendable (Double) async -> Void
    ) async throws -> MeetingImportedAudioFile
    typealias ActiveAnalysisCanceller = @MainActor @Sendable () async -> Void
    typealias CanStartProvider = @MainActor @Sendable () -> Bool
    typealias AnalysisRollback = @MainActor @Sendable (TranscriptionHistoryEntry) -> Void

    @Published private(set) var tasks: [MeetingFileTask]

    private struct PersistedPayload: Codable, Sendable {
        let version: Int
        let tasks: [MeetingFileTask]
    }

    private let analyzer: Analyzer
    private let preparer: Preparer
    private let cancelActiveAnalysis: ActiveAnalysisCanceller
    private let canStart: CanStartProvider
    private let rollbackAnalysis: AnalysisRollback
    private let fileManager: FileManager
    private let now: () -> Date
    private let storageDirectoryURL: URL
    private let taskFileURL: URL
    private let persistenceCoordinator: AsyncJSONPersistenceCoordinator
    private static let maximumStagedSourceBytes: Int64 = 8 * 1024 * 1024 * 1024
    private static let maximumTaskCount = 64
    private var activePreparationID: UUID?
    private var workerTask: Task<Void, Never>?
    private var tickerTask: Task<Void, Never>?
    private var activeTaskID: UUID?
    private var stagingTaskIDs: Set<UUID> = []
    private var stagingTasks: [UUID: Task<Void, Never>] = [:]
    private var reservedStagingBytes: Int64 = 0
    private var isShuttingDown = false
    private var resourceWaitNotificationToken: NSObjectProtocol?

    init(
        analyzer: @escaping Analyzer,
        cancelActiveAnalysis: @escaping ActiveAnalysisCanceller,
        canStart: @escaping CanStartProvider,
        rollbackAnalysis: @escaping AnalysisRollback = { _ in },
        preparer: @escaping Preparer = { source, destination, limits, checkpoint, progress in
            try await MeetingImportedAudioFile.prepare(
                from: source, to: destination, limits: limits, checkpoint: checkpoint, progress: progress
            )
        },
        fileManager: FileManager = .default,
        storageDirectoryURL: URL? = nil,
        now: @escaping () -> Date = Date.init
    ) {
        self.analyzer = analyzer
        self.preparer = preparer
        self.cancelActiveAnalysis = cancelActiveAnalysis
        self.canStart = canStart
        self.rollbackAnalysis = rollbackAnalysis
        self.fileManager = fileManager
        self.now = now

        let resolvedStorageDirectoryURL = storageDirectoryURL ?? Self.defaultStorageDirectoryURL(fileManager: fileManager)
        self.storageDirectoryURL = resolvedStorageDirectoryURL
        self.taskFileURL = resolvedStorageDirectoryURL.appendingPathComponent("tasks.json")
        self.persistenceCoordinator = AsyncJSONPersistenceCoordinator(
            label: "com.voxt.meeting-file-task-queue.persistence"
        )
        self.tasks = []

        loadPersistedTasks()
        resourceWaitNotificationToken = NotificationCenter.default.addObserver(
            forName: .voxtMeetingFileResourceWaitDidChange,
            object: nil,
            queue: .main
        ) { [weak self] notification in
            Task { @MainActor [weak self] in
                self?.handleResourceWaitNotification(notification)
            }
        }
        removeAbandonedPartialFiles()
        restoreStagingReservations()
    }

    var hasActiveTasks: Bool {
        tasks.contains { !$0.isTerminal }
    }

    var hasFinishedTasks: Bool {
        tasks.contains(where: \.isTerminal)
    }

    func task(id: UUID) -> MeetingFileTask? {
        tasks.first { $0.id == id }
    }

    func enqueue(urls: [URL]) {
        guard !isShuttingDown else { return }

        for sourceURL in urls {
            guard MeetingFileImportSupport.isSupportedImportFile(at: sourceURL) else { continue }

            guard tasks.count < Self.maximumTaskCount else {
                NotificationCenter.default.post(
                    name: .voxtFeatureSettingsToastRequested,
                    object: nil,
                    userInfo: ["message": AppLocalization.localizedString("The file task queue is full. Clear finished tasks before adding more files.")]
                )
                break
            }
            let taskID = UUID()
            let fileName = sourceURL.lastPathComponent
            let stagedFileName = taskID.uuidString + "-" + fileName + ".prepared.wav"
            tasks.append(
                .queued(
                    id: taskID,
                    fileName: fileName,
                    stagedFileName: stagedFileName,
                    enqueuedAt: now()
                )
            )
            MeetingFileTrace.event("enqueued", taskID: taskID, "queueCount=\(tasks.count)")
            stagingTaskIDs.insert(taskID)
            stage(sourceURL: sourceURL, taskID: taskID, stagedFileName: stagedFileName)
        }

        persist()
        startIfNeeded()
    }

    func cancel(taskID: UUID) {
        guard let index = tasks.firstIndex(where: { $0.id == taskID }) else { return }
        guard !tasks[index].isTerminal else { return }
        MeetingFileTrace.event("cancel-requested", taskID: taskID, diagnosticContext(for: tasks[index]))

        if activeTaskID == taskID,
           tasks[index].status == .processing || tasks[index].status == .waitingForResources {
            tasks[index].status = .cancelling
            persist()
            Task { @MainActor [weak self] in
                guard let self, self.activeTaskID == taskID,
                      self.task(id: taskID)?.status == .cancelling else { return }
                await self.cancelActiveAnalysis()
            }
            return
        }

        if stagingTaskIDs.contains(taskID) {
            stagingTasks[taskID]?.cancel()
            if activePreparationID == taskID {
                tasks[index].status = .cancelling
                persist()
                return
            }
        }
        tasks[index].status = .cancelled
        tasks[index].completedAt = now()
        persist()
        startIfNeeded()
    }

    /// Moves a queued task ahead of the other queued tasks while keeping any
    /// currently processing task in place.
    func prioritize(taskID: UUID) {
        guard let taskIndex = tasks.firstIndex(where: { $0.id == taskID }),
              tasks[taskIndex].status == .queued,
              let firstQueuedIndex = tasks.firstIndex(where: { $0.status == .queued }),
              taskIndex != firstQueuedIndex
        else { return }

        let task = tasks.remove(at: taskIndex)
        let insertionIndex = tasks.firstIndex(where: { $0.status == .queued }) ?? tasks.endIndex
        tasks.insert(task, at: insertionIndex)
        persist()
        startIfNeeded()
    }

    func retry(taskID: UUID) {
        guard let index = tasks.firstIndex(where: { $0.id == taskID }) else { return }
        guard tasks[index].status == .failed || tasks[index].status == .cancelled else { return }
        guard !stagingTaskIDs.contains(taskID) else { return }
        guard fileManager.fileExists(atPath: stagedURL(for: tasks[index]).path) else {
            tasks[index].status = .failed
            tasks[index].errorMessage = AppLocalization.localizedString("The staged source file is no longer available.")
            persist()
            return
        }

        tasks[index] = tasks[index].resetForRetry()
        persist()
        startIfNeeded()
    }

    func clearFinishedTasks() {
        let finishedTasks = tasks.filter(\.isTerminal)
        tasks.removeAll(where: \.isTerminal)
        for task in finishedTasks {
            Task { await MeetingFileAnalysisCheckpointStore.shared.clear(taskID: task.id) }
            try? fileManager.removeItem(at: stagedURL(for: task))
            if let legacyName = task.legacyStagedFileName {
                try? fileManager.removeItem(at: storageDirectoryURL.appendingPathComponent(legacyName))
            }
        }
        restoreStagingReservations()
        persist()
    }

    func startIfNeeded() {
        guard !isShuttingDown else { return }
        // Lazily upgrade raw files from v1 queues through the same serial lane.
        if stagingTaskIDs.isEmpty,
           let task = tasks.first(where: { !$0.isTerminal }),
           task.status == .queued, task.preparedAudioVersion != 1 {
            stagingTaskIDs.insert(task.id)
            stage(sourceURL: stagedURL(for: task), taskID: task.id, stagedFileName: task.stagedFileName)
        }
        guard workerTask == nil, tasks.contains(where: { !$0.isTerminal }) else { return }
        workerTask = Task { @MainActor [weak self] in
            await self?.runWorker()
        }
        if tickerTask == nil {
            tickerTask = Task { @MainActor [weak self] in
                await self?.runTicker()
            }
        }
    }

    func shutdown() async {
        MeetingFileTrace.event("shutdown-requested", taskID: activeTaskID ?? activePreparationID)
        if let resourceWaitNotificationToken {
            NotificationCenter.default.removeObserver(resourceWaitNotificationToken)
            self.resourceWaitNotificationToken = nil
        }
        isShuttingDown = true
        tickerTask?.cancel()
        tickerTask = nil
        if activeTaskID != nil {
            await cancelActiveAnalysis()
        }
        let stagingTaskIDsToCancel = Array(stagingTasks.keys)
        for taskID in stagingTaskIDsToCancel {
            if let index = tasks.firstIndex(where: { $0.id == taskID }), !tasks[index].isTerminal {
                // Do not allow Clear Finished Tasks to race a decoder/writer.
                tasks[index].status = .cancelling
            }
            stagingTasks[taskID]?.cancel()
        }
        persist()
        if let workerTask {
            await workerTask.value
        }
        workerTask = nil
        let tasksToFinish = Array(stagingTasks.values)
        for stagingTask in tasksToFinish {
            await stagingTask.value
        }
        stagingTasks.removeAll()
        persistenceCoordinator.flushWrite(
            PersistedPayload(version: 2, tasks: tasks),
            to: taskFileURL
        )
    }

    private func runWorker() async {
        defer { workerTask = nil }

        while !Task.isCancelled, !isShuttingDown {
            startIfNeeded()
            guard let index = nextRunnableTaskIndex() else {
                guard tasks.contains(where: { !$0.isTerminal }) else { return }
                try? await Task.sleep(for: .milliseconds(200))
                continue
            }
            let taskID = tasks[index].id

            while (!canStart() || activePreparationID != nil), !Task.isCancelled, !isShuttingDown {
                try? await Task.sleep(for: .seconds(1))
            }
            guard !Task.isCancelled, !isShuttingDown else { return }

            guard let queuedTask = task(id: taskID), queuedTask.status == .queued else { continue }
            let preparedURL = stagedURL(for: queuedTask)
            do {
                _ = try await Task.detached(priority: .utility) {
                    try MeetingImportedAudioFile.validatedPreparedFile(at: preparedURL)
                }.value
            } catch {
                if !isShuttingDown, task(id: taskID)?.status == .queued {
                    markFailed(taskID: taskID, error: error)
                    persist()
                }
                continue
            }
            guard !isShuttingDown, !Task.isCancelled else { return }
            guard canStart(), activePreparationID == nil,
                  tasks.first(where: { !$0.isTerminal })?.id == taskID else { continue }
            guard let currentIndex = tasks.firstIndex(where: { $0.id == taskID }),
                  tasks[currentIndex].status == .queued
            else { continue }

            let startDate = now()
            tasks[currentIndex].status = .processing
            tasks[currentIndex].startedAt = tasks[currentIndex].startedAt ?? startDate
            tasks[currentIndex].completedAt = nil
            tasks[currentIndex].progressStage = .transcribing
            tasks[currentIndex].status = .processing
            tasks[currentIndex].progressFraction = 0.15
            tasks[currentIndex].processedMediaDurationSeconds = nil
            tasks[currentIndex].processingSpeedSecondsPerSecond = nil
            tasks[currentIndex].speedSampleAt = nil
            tasks[currentIndex].speedSampleProcessedMediaDurationSeconds = nil
            tasks[currentIndex].estimatedTotalSeconds = nil
            tasks[currentIndex].errorMessage = nil
            activeTaskID = taskID
            VoxtLog.meeting("File task analysis started. \(diagnosticContext(for: tasks[currentIndex]))")
            persist()

            do {
                let task = tasks[currentIndex]
                let entry = try await MeetingFileTrace.$taskID.withValue(taskID) {
                    try await analyzer(stagedURL(for: task), task.fileName) { [weak self] progress in
                        guard let self else { return }
                        self.apply(progress: progress, to: taskID)
                    }
                }
                guard let finishedIndex = tasks.firstIndex(where: { $0.id == taskID }) else { continue }
                let shouldRollback = isShuttingDown || tasks[finishedIndex].status == .cancelling
                if shouldRollback {
                    rollbackAnalysis(entry)
                }
                if isShuttingDown {
                    markInterrupted(taskID: taskID)
                } else if tasks[finishedIndex].status == .cancelling {
                    tasks[finishedIndex].status = .cancelled
                    tasks[finishedIndex].completedAt = now()
                } else {
                    await MeetingFileAnalysisCheckpointStore.shared.clear(taskID: taskID)
                    tasks[finishedIndex].status = .completed
                    tasks[finishedIndex].completedAt = now()
                    tasks[finishedIndex].progressStage = .saving
                    tasks[finishedIndex].progressFraction = 1
                    tasks[finishedIndex].historyEntryID = entry.id
                    VoxtLog.meeting("File task completed. \(diagnosticContext(for: tasks[finishedIndex]))")
                    MeetingFileTrace.event("task-completed", taskID: taskID, diagnosticContext(for: tasks[finishedIndex]))
                    SystemNotificationSupport.post(
                        title: AppLocalization.localizedString("File conversion completed"),
                        body: AppLocalization.format("%@ has been converted successfully.", tasks[finishedIndex].fileName),
                        userInfo: [
                            "fileTaskID": taskID.uuidString,
                            "historyEntryID": entry.id.uuidString
                        ]
                    )
                }
            } catch is CancellationError {
                if isShuttingDown {
                    markInterrupted(taskID: taskID)
                } else {
                    markCancelled(taskID: taskID)
                }
            } catch {
                markFailed(taskID: taskID, error: error)
            }

            activeTaskID = nil
            persist()
        }
    }

    private func runTicker() async {
        defer { tickerTask = nil }
        var lastTrace = ContinuousClock.now
        while !Task.isCancelled, !isShuttingDown {
            try? await Task.sleep(for: .seconds(1))
            guard !Task.isCancelled else { return }
            guard hasActiveTasks else { return }
            if lastTrace.duration(to: .now) >= .seconds(15),
               let current = tasks.first(where: { !$0.isTerminal }) {
                MeetingFileTrace.event("heartbeat", taskID: current.id,
                    "status=\(current.status.rawValue), canStart=\(canStart()), \(diagnosticContext(for: current))")
                lastTrace = .now
            }
            objectWillChange.send()
        }
    }

    private func nextRunnableTaskIndex() -> Int? {
        guard let index = tasks.firstIndex(where: { !$0.isTerminal }) else { return nil }
        let task = tasks[index]
        guard task.status == .queued, task.preparedAudioVersion == 1,
              !stagingTaskIDs.contains(task.id) else { return nil }
        guard fileManager.fileExists(atPath: stagedURL(for: task).path) else {
            markFailed(
                taskID: task.id,
                error: NSError(
                    domain: "Voxt.MeetingFileTaskQueue",
                    code: 1,
                    userInfo: [NSLocalizedDescriptionKey: AppLocalization.localizedString("The staged source file is no longer available.")]
                )
            )
            return nextRunnableTaskIndex()
        }
        return index
    }

    private func apply(progress: MeetingFileAnalysisProgress, to taskID: UUID) {
        guard let index = tasks.firstIndex(where: { $0.id == taskID }),
              tasks[index].status == .processing || tasks[index].status == .preparing
        else { return }
        let sampleDate = now()
        let previousStage = tasks[index].progressStage
        let previousProcessedSeconds = tasks[index].processedMediaDurationSeconds ?? 0
        let previousFraction = tasks[index].progressFraction
        tasks[index].progressStage = progress.stage
        tasks[index].progressFraction = min(max(progress.fractionCompleted, 0), 1)
        if let mediaDurationSeconds = progress.mediaDurationSeconds {
            tasks[index].mediaDurationSeconds = mediaDurationSeconds
        }
        if let processedMediaDurationSeconds = progress.processedMediaDurationSeconds {
            tasks[index].processedMediaDurationSeconds = processedMediaDurationSeconds
        }
        let crossedAudioMinute = floor((tasks[index].processedMediaDurationSeconds ?? 0) / 60) > floor(previousProcessedSeconds / 60)
        let crossedProgressStep = floor(tasks[index].progressFraction * 20) > floor(previousFraction * 20)
        if previousStage != progress.stage || crossedAudioMinute || crossedProgressStep {
            VoxtLog.meeting("File task progress. \(diagnosticContext(for: tasks[index]))")
        }
        updateProcessingSpeed(for: &tasks[index], at: sampleDate)
        tasks[index].estimatedTotalSeconds = MeetingFileTask.updatedEstimatedTotalSeconds(
            current: tasks[index].estimatedTotalSeconds,
            elapsed: tasks[index].elapsedSeconds(now: now()),
            progressFraction: tasks[index].progressFraction,
            mediaDurationSeconds: tasks[index].mediaDurationSeconds,
            processedMediaDurationSeconds: tasks[index].processedMediaDurationSeconds,
            stage: progress.stage,
            processingSpeed: tasks[index].processingSpeedSecondsPerSecond
        )
        persist()
    }

    private func updateProcessingSpeed(for task: inout MeetingFileTask, at sampleDate: Date) {
        guard task.progressStage == .transcribing,
              let processedDuration = task.processedMediaDurationSeconds,
              processedDuration > 0
        else { return }

        if let previousDate = task.speedSampleAt,
           let previousProcessedDuration = task.speedSampleProcessedMediaDurationSeconds {
            let wallDuration = sampleDate.timeIntervalSince(previousDate)
            let audioDuration = processedDuration - previousProcessedDuration
            if wallDuration > 0, audioDuration > 0 {
                let instantaneousSpeed = audioDuration / wallDuration
                if let existingSpeed = task.processingSpeedSecondsPerSecond {
                    task.processingSpeedSecondsPerSecond = existingSpeed * 0.7 + instantaneousSpeed * 0.3
                } else {
                    task.processingSpeedSecondsPerSecond = instantaneousSpeed
                }
            }
        }

        task.speedSampleAt = sampleDate
        task.speedSampleProcessedMediaDurationSeconds = processedDuration
    }

    private func handleResourceWaitNotification(_ notification: Notification) {
        guard let rawTaskID = notification.userInfo?["taskID"] as? String,
              let taskID = UUID(uuidString: rawTaskID),
              let index = tasks.firstIndex(where: { $0.id == taskID }),
              activeTaskID == taskID
        else { return }
        let isWaiting = notification.userInfo?["isWaiting"] as? Bool ?? false
        guard !tasks[index].isTerminal else { return }
        if isWaiting, tasks[index].status == .processing {
            tasks[index].status = .waitingForResources
            persist()
        } else if !isWaiting, tasks[index].status == .waitingForResources {
            tasks[index].status = .processing
            persist()
        }
    }

    private func markCancelled(taskID: UUID) {
        guard let index = tasks.firstIndex(where: { $0.id == taskID }) else { return }
        tasks[index].status = .cancelled
        tasks[index].completedAt = now()
        VoxtLog.meeting("File task cancelled. \(diagnosticContext(for: tasks[index]))")
        MeetingFileTrace.event("task-cancelled", taskID: taskID, diagnosticContext(for: tasks[index]))
    }

    private func markInterrupted(taskID: UUID) {
        guard let index = tasks.firstIndex(where: { $0.id == taskID }) else { return }
        var task = tasks[index].resetForRetry()
        task.errorMessage = AppLocalization.localizedString("The task was interrupted and has been queued again.")
        tasks[index] = task
    }

    private func markFailed(taskID: UUID, error: Error) {
        guard let index = tasks.firstIndex(where: { $0.id == taskID }) else { return }
        tasks[index].status = .failed
        tasks[index].completedAt = now()
        tasks[index].errorMessage = error.localizedDescription
        VoxtLog.meetingError(
            "File task failed. \(diagnosticContext(for: tasks[index])), \(MeetingFileTaskDiagnostics.errorSummary(error))"
        )
        MeetingFileTrace.event("task-failed", taskID: taskID,
            "\(diagnosticContext(for: tasks[index])), \(MeetingFileTaskDiagnostics.errorSummary(error))")
        SystemNotificationSupport.post(
            title: AppLocalization.localizedString("File conversion failed"),
            body: AppLocalization.format("%@: %@", tasks[index].fileName, error.localizedDescription)
        )
    }

    private func diagnosticContext(for task: MeetingFileTask) -> String {
        let elapsed = String(format: "%.2f", task.elapsedSeconds(now: now()))
        let processed = task.processedMediaDurationSeconds.map { String(format: "%.2f", $0) } ?? "unknown"
        let duration = task.mediaDurationSeconds.map { String(format: "%.2f", $0) } ?? "unknown"
        let fraction = String(format: "%.4f", task.progressFraction)
        return "taskID=\(task.id), stage=\(task.progressStage.diagnosticName), elapsedSeconds=\(elapsed), processedAudioSeconds=\(processed), totalAudioSeconds=\(duration), progress=\(fraction), thermalState=\(ProcessInfo.processInfo.thermalState.rawValue)"
    }

    private func stage(sourceURL: URL, taskID: UUID, stagedFileName: String) {
        // Retain the provider URL and scope, but read no audio until admission.
        let didStartAccessing = sourceURL.startAccessingSecurityScopedResource()
        let preparedName = stagedFileName.hasSuffix(".prepared.wav")
            ? stagedFileName : stagedFileName + ".prepared.wav"
        let destinationURL = storageDirectoryURL.appendingPathComponent(preparedName)
        let preparer = self.preparer
        let fileManager = self.fileManager

        let stagingTask = Task { @MainActor [weak self] in
            var createdDestination = false
            defer {
                if didStartAccessing { sourceURL.stopAccessingSecurityScopedResource() }
                if let self {
                    self.stagingTasks.removeValue(forKey: taskID)
                    self.stagingTaskIDs.remove(taskID)
                    if self.activePreparationID == taskID { self.activePreparationID = nil }
                    self.restoreStagingReservations()
                    self.persist()
                    self.startIfNeeded()
                }
            }
            do {
                guard let self else { throw CancellationError() }
                while true {
                    try Task.checkCancellation()
                    guard !self.isShuttingDown, let task = self.task(id: taskID), !task.isTerminal else {
                        throw CancellationError()
                    }
                    if self.activePreparationID == nil, self.activeTaskID == nil,
                       self.tasks.first(where: { !$0.isTerminal })?.id == taskID,
                       self.canStart() { break }
                    try await Task.sleep(for: .milliseconds(200))
                }
                self.activePreparationID = taskID
                guard let index = self.tasks.firstIndex(where: { $0.id == taskID }) else { throw CancellationError() }
                self.tasks[index].status = .preparing
                self.tasks[index].startedAt = self.now()
                self.tasks[index].progressStage = .preparing
                self.tasks[index].progressFraction = 0
                VoxtLog.meeting("File task preparation started. \(self.diagnosticContext(for: self.tasks[index]))")
                self.persist()
                try fileManager.createDirectory(at: self.storageDirectoryURL, withIntermediateDirectories: true)
                self.restoreStagingReservations()
                let remainingBytes = max(0, Self.maximumStagedSourceBytes - self.reservedStagingBytes)
                var limits = MeetingFilePreparationLimits()
                limits.maximumOutputBytes = min(limits.maximumOutputBytes, remainingBytes)
                let preparationLimits = limits
                createdDestination = !fileManager.fileExists(atPath: destinationURL.path)
                let shouldPrepare = createdDestination
                let preparationTask = Task.detached(priority: .utility) { [weak self] in
                    try await MeetingFileTrace.$taskID.withValue(taskID) {
                        MeetingFileTrace.event("preparation-admitted", "cacheReuse=\(!shouldPrepare), outputBudgetBytes=\(preparationLimits.maximumOutputBytes)")
                        if !shouldPrepare {
                            // Recover a renamed cache even if metadata wasn't flushed.
                            return try MeetingImportedAudioFile.validatedPreparedFile(at: destinationURL)
                        }
                        let before = try fileManager.attributesOfItem(atPath: sourceURL.path)
                        guard before[.type] as? FileAttributeType == .typeRegular else {
                            throw MeetingFileTaskStagingError.sourceUnavailable
                        }
                        let audio = try await preparer(sourceURL, destinationURL, preparationLimits, {
                            guard let self else { throw CancellationError() }
                            try await self.waitForPreparationAvailability()
                        }, { fraction in
                            await self?.apply(
                                progress: MeetingFileAnalysisProgress(stage: .preparing, stageFraction: fraction),
                                to: taskID
                            )
                        })
                        let after = try fileManager.attributesOfItem(atPath: sourceURL.path)
                        guard (before[.size] as? NSNumber) == (after[.size] as? NSNumber),
                              (before[.modificationDate] as? Date) == (after[.modificationDate] as? Date),
                              (before[.systemFileNumber] as? NSNumber) == (after[.systemFileNumber] as? NSNumber),
                              (before[.systemNumber] as? NSNumber) == (after[.systemNumber] as? NSNumber) else {
                            throw MeetingFileTaskStagingError.sourceUnavailable
                        }
                        MeetingFileTrace.event("source-validation-completed", "samples=\(audio.sampleCount)")
                        return audio
                    }
                }
                let audio = try await withTaskCancellationHandler {
                    try await preparationTask.value
                } onCancel: { preparationTask.cancel() }
                try Task.checkCancellation()
                guard let updatedIndex = self.tasks.firstIndex(where: { $0.id == taskID }) else { throw CancellationError() }
                if preparedName != stagedFileName {
                    self.tasks[updatedIndex].legacyStagedFileName = stagedFileName
                }
                self.tasks[updatedIndex].stagedFileName = preparedName
                self.tasks[updatedIndex].preparedAudioVersion = 1
                self.tasks[updatedIndex].mediaDurationSeconds = audio.durationSeconds
                self.tasks[updatedIndex].progressFraction = 0.15
                self.tasks[updatedIndex].status = .queued
                VoxtLog.meeting("File task preparation ready. \(self.diagnosticContext(for: self.tasks[updatedIndex]))")
            } catch {
                MeetingFileTrace.event("preparation-stopped", taskID: taskID, MeetingFileTaskDiagnostics.errorSummary(error))
                if createdDestination {
                    try? fileManager.removeItem(at: destinationURL)
                    try? fileManager.removeItem(at: destinationURL.appendingPathExtension("partial"))
                }
                if let self, self.isShuttingDown,
                   let index = self.tasks.firstIndex(where: { $0.id == taskID }) {
                    self.tasks[index].status = .failed
                    self.tasks[index].completedAt = self.now()
                    self.tasks[index].errorMessage = AppLocalization.localizedString("The meeting file could not be staged before the app closed.")
                } else if error is CancellationError {
                    self?.markCancelled(taskID: taskID)
                } else {
                    self?.markFailed(taskID: taskID, error: error)
                }
            }
        }
        stagingTasks[taskID] = stagingTask
    }

    private func waitForPreparationAvailability() async throws {
        var waited = false
        defer { if waited { MeetingFileTrace.event("preparation-recording-wait-ended", "cancelled=\(Task.isCancelled)") } }
        while !canStart() {
            if !waited {
                MeetingFileTrace.event("preparation-recording-wait-started")
                waited = true
            }
            try Task.checkCancellation()
            guard !isShuttingDown else { throw CancellationError() }
            try await Task.sleep(for: .milliseconds(250))
        }
        try Task.checkCancellation()
        guard !isShuttingDown else { throw CancellationError() }
    }

    private func stagedURL(for task: MeetingFileTask) -> URL {
        storageDirectoryURL.appendingPathComponent(task.stagedFileName)
    }

    private func restoreStagingReservations() {
        // Include orphaned complete caches and legacy originals in the quota;
        // a lost metadata write must not make their disk usage disappear.
        reservedStagingBytes = 0
        let urls: [URL]
        do {
            urls = try fileManager.contentsOfDirectory(
                at: storageDirectoryURL, includingPropertiesForKeys: [.isRegularFileKey, .fileSizeKey]
            )
        } catch {
            // Fail closed if an existing directory's usage cannot be counted.
            reservedStagingBytes = fileManager.fileExists(atPath: storageDirectoryURL.path) ? .max : 0
            return
        }
        for url in urls where url.lastPathComponent != taskFileURL.lastPathComponent {
            guard let values = try? url.resourceValues(forKeys: [.isRegularFileKey, .fileSizeKey]) else {
                reservedStagingBytes = .max
                return
            }
            guard values.isRegularFile == true else { continue }
            guard let size = values.fileSize, size >= 0 else {
                reservedStagingBytes = .max
                return
            }
            let (sum, overflow) = reservedStagingBytes.addingReportingOverflow(Int64(size))
            reservedStagingBytes = overflow ? Int64.max : sum
        }
    }

    private func removeAbandonedPartialFiles() {
        let urls = (try? fileManager.contentsOfDirectory(at: storageDirectoryURL, includingPropertiesForKeys: nil)) ?? []
        for url in urls {
            let name = url.lastPathComponent
            guard name.hasSuffix(".prepared.wav.partial"),
                  UUID(uuidString: String(name.prefix(36))) != nil else { continue }
            try? fileManager.removeItem(at: url)
        }
    }

    private func loadPersistedTasks() {
        do {
            guard fileManager.fileExists(atPath: taskFileURL.path) else { return }
            let data = try Data(contentsOf: taskFileURL)
            let payload = try JSONDecoder().decode(PersistedPayload.self, from: data)
            guard payload.version == 1 || payload.version == 2 else { return }
            tasks = payload.tasks.filter { task in
                // Persisted paths must stay inside this queue's storage directory.
                [task.stagedFileName, task.legacyStagedFileName].compactMap { $0 }.allSatisfy {
                    !$0.isEmpty && $0 != "." && $0 != ".." && URL(fileURLWithPath: $0).lastPathComponent == $0
                }
            }.map { task in
                let destinationName = task.stagedFileName.hasSuffix(".prepared.wav")
                    ? task.stagedFileName : task.stagedFileName + ".prepared.wav"
                try? fileManager.removeItem(at: storageDirectoryURL.appendingPathComponent(destinationName).appendingPathExtension("partial"))
                guard task.status == .processing || task.status == .waitingForResources || task.status == .cancelling || task.status == .preparing else { return task }
                var restored = task.resetForRetry()
                restored.errorMessage = AppLocalization.localizedString("The task was interrupted and has been queued again.")
                return restored
            }
            restoreStagingReservations()
            persist()
        } catch {
            tasks = []
        }
    }

    private func persist() {
        let payload = PersistedPayload(version: 2, tasks: tasks)
        persistenceCoordinator.scheduleWrite(payload, to: taskFileURL)
    }

    private static func defaultStorageDirectoryURL(fileManager: FileManager) -> URL {
        let appSupport = fileManager.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? fileManager.temporaryDirectory
        return appSupport
            .appendingPathComponent("Voxt", isDirectory: true)
            .appendingPathComponent("meeting-file-tasks", isDirectory: true)
    }
}
