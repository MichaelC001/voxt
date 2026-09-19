// CustomLLMModelManager.swift
// Provides Custom LLMModel Manager for model catalog and storage support.

import Foundation
import HuggingFace
import Combine
import CoreImage
import MLX
import MLXLLM
import MLXLMCommon
import MLXVLM
import Tokenizers

private struct LocalTokenizerBridge: MLXLMCommon.Tokenizer {
    private let upstream: any Tokenizers.Tokenizer

    init(_ upstream: any Tokenizers.Tokenizer) {
        self.upstream = upstream
    }

    func encode(text: String, addSpecialTokens: Bool) -> [Int] {
        upstream.encode(text: text, addSpecialTokens: addSpecialTokens)
    }

    func decode(tokenIds: [Int], skipSpecialTokens: Bool) -> String {
        upstream.decode(tokens: tokenIds, skipSpecialTokens: skipSpecialTokens)
    }

    func convertTokenToId(_ token: String) -> Int? {
        upstream.convertTokenToId(token)
    }

    func convertIdToToken(_ id: Int) -> String? {
        upstream.convertIdToToken(id)
    }

    var bosToken: String? { upstream.bosToken }
    var eosToken: String? { upstream.eosToken }
    var unknownToken: String? { upstream.unknownToken }

    func applyChatTemplate(
        messages: [[String: any Sendable]],
        tools: [[String: any Sendable]]?,
        additionalContext: [String: any Sendable]?
    ) throws -> [Int] {
        do {
            return try upstream.applyChatTemplate(
                messages: messages,
                tools: tools,
                additionalContext: additionalContext
            )
        } catch Tokenizers.TokenizerError.missingChatTemplate {
            throw MLXLMCommon.TokenizerError.missingChatTemplate
        }
    }
}

private struct LocalTokenizerLoader: MLXLMCommon.TokenizerLoader {
    func load(from directory: URL) async throws -> any MLXLMCommon.Tokenizer {
        let tokenizer = try await Tokenizers.AutoTokenizer.from(modelFolder: directory)
        return LocalTokenizerBridge(tokenizer)
    }
}

@MainActor
class CustomLLMModelManager: ObservableObject {
    private struct TextResultPayload: Decodable {
        let resultText: String
    }

    static let defaultHubBaseURL = URL(string: "https://huggingface.co")!
    static let mirrorHubBaseURL = URL(string: "https://hf-mirror.com")!
    static let hubUserAgent = "Voxt/1.0 (CustomLLM)"

    enum ModelState: Equatable {
        case notDownloaded
        case downloading(
            progress: Double,
            completed: Int64,
            total: Int64,
            currentFile: String?,
            completedFiles: Int,
            totalFiles: Int
        )
        case paused(
            progress: Double,
            completed: Int64,
            total: Int64,
            currentFile: String?,
            completedFiles: Int,
            totalFiles: Int
        )
        case downloaded
        case error(String)
    }

    struct CatalogSnapshot: Equatable {
        let repo: String
        let isDownloaded: Bool
        let hasResumableDownload: Bool
        let state: ModelState
        let pausedStatusMessage: String?
        let hasActiveDownloadTask: Bool

        var isDownloading: Bool {
            if hasActiveDownloadTask {
                return true
            }
            if case .downloading = state {
                return true
            }
            return false
        }

        var isPaused: Bool {
            if case .paused = state {
                return true
            }
            return hasResumableDownload
        }
    }

    private enum DownloadStopAction {
        case pause
        case cancel
    }

    enum ModelSizeState: Equatable {
        case unknown
        case loading
        case ready(bytes: Int64, text: String)
        case error(String)
    }

    typealias ModelOption = CustomLLMModelCatalog.Option

    nonisolated static let defaultModelRepo = CustomLLMModelCatalog.defaultModelRepo
    nonisolated static let availableModels = CustomLLMModelCatalog.availableModels
    nonisolated static let supportedModels = CustomLLMModelCatalog.supportedModels

    @Published private(set) var state: ModelState = .notDownloaded
    @Published private(set) var sizeState: ModelSizeState = .unknown
    @Published private(set) var remoteSizeTextByRepo: [String: String] = [:]
    @Published private(set) var pausedStatusMessage: String?
    @Published private(set) var pausedStatusMessageByRepo: [String: String] = [:]
    @Published private(set) var activeDownloadRepos: Set<String> = []
    @Published private(set) var lastRunDiagnostics: CustomLLMRunDiagnostics?
    @Published var generationTuning = CustomLLMGenerationTuning.default

    private(set) var stateByRepo: [String: ModelState] = [:]
    private var downloadedStateByRepo: [String: Bool] = [:]
    private let installationCache = ModelInstallationCache()
    private var deletingRepos: Set<String> = []
    private var storageRevision = UUID()
    private var storageRoots: [URL] = []
    private var downloadStorageRevisions: [String: UUID] = [:]
    @Published private(set) var installationRevision: UInt64 = 0
    private var localSizeTextByRepo: [String: String] = [:]
    private var modelRepo: String
    private var hubBaseURL: URL
    private var downloadTasksByRepo: [String: Task<Void, Never>] = [:]
    private var downloadProgressTasksByRepo: [String: Task<Void, Never>] = [:]
    private var sizeTask: Task<Void, Never>?
    private var prefetchTask: Task<Void, Never>?
    private var idleUnloadTask: Task<Void, Never>?
    private var downloadStopActionsByRepo: [String: DownloadStopAction] = [:]
    private var inferenceContainer: ModelContainer? {
        didSet {
            // Observe the state transition so every non-termination release path
            // schedules the same delayed reclamation.
            guard ModelUnloadReclamationNotificationPolicy.shouldNotify(
                wasLoaded: oldValue != nil,
                isLoaded: inferenceContainer != nil,
                isApplicationTerminating: isShuttingDownForApplicationTermination
            ) else { return }
            onModelUnloaded?()
        }
    }
    private var inferenceModelRepo: String?
    private let inferenceLoadCoordinator = SharedModelLoadCoordinator<ModelContainer>()
    private var lastLoggedModelPresence: (repo: String, downloaded: Bool)?
    private var lastInvalidRepoLogged: String?
    private var activeInferenceCount = 0
    private var activeInferenceWaiters: [CheckedContinuation<Void, Never>] = []
    private var isShuttingDownForApplicationTermination = false
    var onModelUnloaded: (() -> Void)?
    private var resolvedIdleUnloadDelay: Duration {
        .seconds(AppPreferenceKey.resolvedLocalModelIdleUnloadDelaySeconds())
    }
    private func resolvedGenerationSettings(for repo: String) -> LLMGenerationSettings {
        CustomLLMGenerationSettingsStore.resolvedSettings(
            for: repo,
            rawByRepo: UserDefaults.standard.string(forKey: AppPreferenceKey.customLLMGenerationSettingsByRepo),
            legacyRaw: UserDefaults.standard.string(forKey: AppPreferenceKey.customLLMGenerationSettings)
        )
    }

    init(modelRepo: String, hubBaseURL: URL = URL(string: "https://huggingface.co")!) {
        let repoSelection = Self.resolveModelRepo(modelRepo)
        let repoWasSupported = Self.isSupportedModelRepo(modelRepo)
        self.modelRepo = repoSelection.effectiveRepo
        self.hubBaseURL = hubBaseURL
        self.remoteSizeTextByRepo = CustomLLMModelStorageSupport.loadPersistedRemoteSizeCache()
        if !repoWasSupported {
            VoxtLog.modelWarning("Unsupported custom LLM repo '\(modelRepo)' found in settings. Falling back to \(repoSelection.effectiveRepo).")
        } else if repoSelection.effectiveRepo != modelRepo {
            VoxtLog.modelInfo("Canonicalized custom LLM repo '\(modelRepo)' -> '\(repoSelection.effectiveRepo)'")
        }
        VoxtLog.modelInfo("Custom LLM manager initialized. repo=\(repoSelection.effectiveRepo), hub=\(hubBaseURL.absoluteString)")
        storageRoots = [writeRootURL()] + readableRootURLs()
        installationCache.onChange = { [weak self] repo, snapshot in
            guard let self else { return }
            self.downloadedStateByRepo[repo] = snapshot.isInstalled
            self.localSizeTextByRepo[repo] = snapshot.allocatedBytes > 0
                ? MLXModelStorageSupport.formatByteCount(snapshot.allocatedBytes) : nil
            if self.downloadTasksByRepo[repo] == nil {
                self.applyInstallationState(snapshot, repo: repo)
            }
            self.installationRevision &+= 1
        }
        checkExistingModel()
    }

    var currentModelRepo: String { modelRepo }
    var hasLoadedInferenceModel: Bool { inferenceContainer != nil }
    var hasPendingModelLoad: Bool { inferenceLoadCoordinator.hasPendingLoad }
    var hasActiveInference: Bool { activeInferenceCount > 0 }

    func refreshMemoryOptimizationPolicy() {
        guard inferenceContainer != nil else {
            cancelIdleUnloadTask()
            return
        }
        guard activeInferenceCount == 0 else { return }
        scheduleIdleUnloadIfNeeded()
    }

    func isModelLoaded(repo: String) -> Bool {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        return inferenceContainer != nil && inferenceModelRepo == canonicalRepo
    }

    func prewarmModel(repo: String) async throws {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        try await withActiveInference {
            let installation = try await refreshInstallation(repo: canonicalRepo)
            guard installation.isInstalled else {
                throw NSError(
                    domain: "Voxt.CustomLLM",
                    code: 404,
                    userInfo: [NSLocalizedDescriptionKey: "Custom LLM model is not installed locally."]
                )
            }
            _ = try await container(for: canonicalRepo)
        }
    }

    func enhance(_ rawText: String, systemPrompt: String) async throws -> String {
        try await enhance(rawText, systemPrompt: systemPrompt, modelRepo: modelRepo)
    }

    func enhance(_ rawText: String, systemPrompt: String, modelRepo: String) async throws -> String {
        let input = rawText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !input.isEmpty else { return rawText }
        let request = CustomLLMRequestPlanBuilder.enhancement(
            input: input,
            systemPrompt: systemPrompt,
            repo: modelRepo,
            resultFallback: rawText,
            structuredOutputPrompt: structuredOutputPrompt(taskInstruction:input:)
        )
        return try await runLocalPromptRequest(request)
    }

    func enhance(userPrompt: String) async throws -> String {
        try await enhance(userPrompt: userPrompt, repo: modelRepo)
    }

    func enhance(userPrompt: String, repo: String) async throws -> String {
        let prompt = userPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !prompt.isEmpty else { return "" }
        let request = CustomLLMRequestPlanBuilder.userPromptEnhancement(
            prompt: prompt,
            repo: repo
        )
        return try await runLocalPromptRequest(request)
    }

    func dictionaryHistoryScanTerms(userPrompt: String, repo: String) async throws -> [String] {
        let prompt = userPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !prompt.isEmpty else { return [] }
        let request = CustomLLMRequestPlanBuilder.dictionaryHistoryScan(
            prompt: prompt,
            repo: repo,
            structuredOutputPrompt: dictionaryHistoryScanStructuredOutputPrompt(_:)
        )
        let rawOutput = try await runLocalPromptRequest(request)
        return try DictionaryHistoryScanResponseParser.parseTerms(from: rawOutput)
    }

    func executeCompiledRequest(
        _ request: LLMCompiledRequest,
        repo: String,
        onPartialText: (@Sendable (String) -> Void)? = nil
    ) async throws -> String {
        let prompt = request.prompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !prompt.isEmpty else { return request.fallbackText }
        let compiledPlan = CustomLLMRequestPlanBuilder.compiled(request: request, repo: repo)
        let result = try await runLocalPromptRequest(compiledPlan, onPartialText: onPartialText)
        return result.isEmpty ? request.fallbackText : result
    }

    func translate(
        _ text: String,
        targetLanguage: TranslationTargetLanguage,
        systemPrompt: String,
        modelRepo: String
    ) async throws -> String {
        let input = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !input.isEmpty else { return text }
        _ = targetLanguage
        let translated = try await runTranslationPrompt(
            input,
            instructions: systemPrompt,
            modelRepo: modelRepo
        )
        return translated.isEmpty ? text : translated
    }

    func translate(
        userPrompt: String,
        fallbackText: String,
        modelRepo: String
    ) async throws -> String {
        let prompt = userPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !prompt.isEmpty else { return fallbackText }
        let request = CustomLLMRequestPlanBuilder.userPromptTranslation(
            prompt: prompt,
            repo: modelRepo,
            resultFallback: fallbackText
        )
        let translated = try await runLocalPromptRequest(request)
        return translated.isEmpty ? fallbackText : translated
    }

    func rewrite(
        sourceText: String,
        dictatedPrompt: String,
        systemPrompt: String,
        modelRepo: String,
        onPartialText: (@Sendable (String) -> Void)? = nil
    ) async throws -> String {
        let instruction = dictatedPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
        let source = sourceText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !instruction.isEmpty || !source.isEmpty else { return sourceText }
        let result = try await runRewritePrompt(
            sourceText: source,
            dictatedPrompt: instruction,
            instructions: systemPrompt,
            modelRepo: modelRepo,
            onPartialText: onPartialText
        )
        return result.isEmpty ? sourceText : result
    }

    func rewrite(
        userPrompt: String,
        fallbackText: String,
        modelRepo: String,
        onPartialText: (@Sendable (String) -> Void)? = nil
    ) async throws -> String {
        let prompt = userPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !prompt.isEmpty else { return fallbackText }
        let request = CustomLLMRequestPlanBuilder.userPromptRewrite(
            prompt: prompt,
            repo: modelRepo,
            resultFallback: fallbackText
        )
        let result = try await runLocalPromptRequest(request, onPartialText: onPartialText)
        return result.isEmpty ? fallbackText : result
    }

    private func runTranslationPrompt(
        _ text: String,
        instructions: String,
        modelRepo: String
    ) async throws -> String {
        let request = CustomLLMRequestPlanBuilder.translation(
            text: text,
            instructions: instructions,
            repo: modelRepo,
            structuredOutputPrompt: structuredOutputPrompt(taskInstruction:input:)
        )
        return try await runLocalPromptRequest(request)
    }

    private func runRewritePrompt(
        sourceText: String,
        dictatedPrompt: String,
        instructions: String,
        modelRepo: String,
        onPartialText: (@Sendable (String) -> Void)? = nil
    ) async throws -> String {
        let request = CustomLLMRequestPlanBuilder.rewrite(
            sourceText: sourceText,
            dictatedPrompt: dictatedPrompt,
            instructions: instructions,
            repo: modelRepo,
            structuredOutputPrompt: structuredOutputPrompt(taskInstruction:input:)
        )
        return try await runLocalPromptRequest(request, onPartialText: onPartialText)
    }

    private func generationParameters(
        for request: CustomLLMRequestPlan,
        behavior: CustomLLMModelBehavior,
        settings: LLMGenerationSettings
    ) -> GenerateParameters {
        let safeInput = max(1, request.inputCharacterCount)
        let estimated = Int(Double(safeInput) * request.kind.tokenBudgetMultiplier)
        let totalPromptCharacters = request.instructions.count + request.prompt.count
        let budget: Int?
        if let override = generationTuning.maxTokensOverride {
            budget = max(1, override)
        } else if let override = settings.maxOutputTokens {
            budget = max(1, override)
        } else if let override = request.maxTokensOverride {
            budget = max(1, override)
        } else {
            budget = defaultOutputTokenBudget(for: request.kind, estimated: estimated)
        }

        let prefillStepSize: Int
        if let override = generationTuning.prefillStepSizeOverride {
            prefillStepSize = override
        } else {
            switch totalPromptCharacters {
            case ..<1000:
                prefillStepSize = 256
            case ..<3000:
                prefillStepSize = 512
            default:
                prefillStepSize = 768
            }
        }

        let repetitionPenalty: Float? =
            settings.repetitionPenalty.map(Float.init) ?? (behavior.family == .qwen3 ? 1.05 : nil)

        return GenerateParameters(
            maxTokens: budget,
            temperature: settings.temperature.map(Float.init) ?? 0,
            topP: settings.topP.map(Float.init) ?? 1.0,
            topK: settings.topK ?? 0,
            minP: settings.minP.map(Float.init) ?? 0,
            repetitionPenalty: repetitionPenalty,
            repetitionContextSize: 32,
            prefill: .init(stepSize: prefillStepSize, chunking: .remainder)
        )
    }

    private func defaultOutputTokenBudget(for kind: CustomLLMTaskKind, estimated: Int) -> Int {
        switch kind {
        case .enhancement:
            return max(128, min(estimated + 128, 1024))
        case .translation:
            return max(128, min(estimated + 160, 1024))
        case .rewrite:
            return max(256, min(estimated + 192, 1536))
        case .dictionaryHistoryScan:
            return max(256, min(estimated + 96, 2048))
        }
    }

    private func runLocalPromptRequest(
        _ request: CustomLLMRequestPlan,
        onPartialText: (@Sendable (String) -> Void)? = nil
    ) async throws -> String {
        return try await withActiveInference {
            try Task.checkCancellation()
            let installation = try await refreshInstallation(repo: request.repo)
            guard installation.isInstalled else {
                throw NSError(
                    domain: "Voxt.CustomLLM",
                    code: 404,
                    userInfo: [NSLocalizedDescriptionKey: "Custom LLM model is not installed locally."]
                )
            }

            let overallStartedAt = Date()
            let containerSnapshot = try await profiledContainer(for: request.repo)
            let container = containerSnapshot.container
            let behavior = CustomLLMModelBehaviorResolver.behavior(for: request.repo)
            let settings = resolvedGenerationSettings(for: request.repo)
            let session = makeChatSession(
                container: container,
                instructions: request.instructions,
                conversationHistory: request.conversationHistory,
                repo: request.repo,
                behavior: behavior,
                settings: settings
            )
            let params = generationParameters(for: request, behavior: behavior, settings: settings)
            session.generateParameters = params
            let inputImages = userInputImages(from: request.attachments)

            let modelStartedAt = Date()
            let setupMs = Int(modelStartedAt.timeIntervalSince(overallStartedAt) * 1000) - containerSnapshot.elapsedMs
            VoxtLog.llmDebug(startLogMessage(for: request, params: params, behavior: behavior))
            VoxtLog.llm(contentLogMessage(for: request))

            var aggregated = ""
            var firstChunkLatencyMs: Int?
            var completionInfo: GenerateCompletionInfo?
            let repetitionGuard = LLMOutputRepetitionGuard()
            var partialDelivery = LocalLLMPartialDelivery()
            var lastPublishedPreview = ""
            for try await event in session.streamDetails(
                to: request.prompt,
                images: inputImages,
                videos: []
            ) {
                try Task.checkCancellation()
                switch event {
                case .chunk(let chunk):
                    if firstChunkLatencyMs == nil, !chunk.isEmpty {
                        firstChunkLatencyMs = Int(Date().timeIntervalSince(modelStartedAt) * 1000)
                    }
                    aggregated += chunk
                    if let repetition = repetitionGuard.repeatedSuffix(in: aggregated) {
                        aggregated = repetition.truncatedText
                        VoxtLog.modelWarning(
                            "Custom LLM \(request.kind.logLabel) repetition guard stopped generation. repo=\(request.repo), repeatedUnitChars=\(repetition.repeatedUnit.count), repetitions=\(repetition.repetitionCount), outputChars=\(aggregated.count)"
                        )
                        break
                    }
                    if let onPartialText,
                       partialDelivery.shouldPublish(at: ProcessInfo.processInfo.systemUptime) {
                        let preview = CustomLLMOutputSanitizer.normalizeResultText(aggregated)
                        if !preview.isEmpty, preview != lastPublishedPreview {
                            lastPublishedPreview = preview
                            onPartialText(preview)
                        }
                    }
                case .info(let info):
                    completionInfo = info
                case .toolCall:
                    continue
                }
            }
            let response = aggregated
            let modelElapsedMs = Int(Date().timeIntervalSince(modelStartedAt) * 1000)
            let totalElapsedMs = Int(Date().timeIntervalSince(overallStartedAt) * 1000)
            // Flush even if the last chunk arrived inside the preview interval.
            if let onPartialText {
                let preview = CustomLLMOutputSanitizer.normalizeResultText(aggregated)
                if !preview.isEmpty, preview != lastPublishedPreview {
                    onPartialText(preview)
                }
            }
            let cleaned: String
            switch request.responseExtractionMode {
            case .textResultPayloadOrNormalizedText:
                cleaned = extractResultText(response)
            case .normalizedRawText:
                cleaned = sanitizeModelOutput(response)
            }

            VoxtLog.llmDebug(
                "Custom LLM \(request.kind.logLabel) completed. repo=\(request.repo), outputChars=\(cleaned.count), elapsedMs=\(modelElapsedMs), totalElapsedMs=\(totalElapsedMs)"
            )
            var diagnostics = CustomLLMRunDiagnostics(
                repo: request.repo,
                taskLabel: request.kind.logLabel,
                containerLoadSource: containerSnapshot.source,
                containerLoadMs: containerSnapshot.elapsedMs,
                setupMs: max(0, setupMs),
                modelElapsedMs: modelElapsedMs,
                totalElapsedMs: totalElapsedMs,
                firstChunkMs: firstChunkLatencyMs,
                overallFirstChunkMs: firstChunkLatencyMs.map { max(0, containerSnapshot.elapsedMs + max(0, setupMs) + $0) },
                promptTokens: nil,
                completionTokens: nil,
                prefillMs: nil,
                generationMs: nil,
                modelOverheadMs: nil,
                totalOverheadMs: nil
            )
            if let completionInfo {
                let firstChunkText = firstChunkLatencyMs.map(String.init) ?? "n/a"
                let promptTPS = String(format: "%.1f", completionInfo.promptTokensPerSecond)
                let generationTPS = String(format: "%.1f", completionInfo.tokensPerSecond)
                let prefillMs = Int((completionInfo.promptTime * 1000).rounded())
                let generationMs = Int((completionInfo.generateTime * 1000).rounded())
                let modelOverheadMs = max(0, modelElapsedMs - prefillMs - generationMs)
                let totalOverheadMs = max(0, totalElapsedMs - prefillMs - generationMs)
                diagnostics = CustomLLMRunDiagnostics(
                    repo: request.repo,
                    taskLabel: request.kind.logLabel,
                    containerLoadSource: containerSnapshot.source,
                    containerLoadMs: containerSnapshot.elapsedMs,
                    setupMs: max(0, setupMs),
                    modelElapsedMs: modelElapsedMs,
                    totalElapsedMs: totalElapsedMs,
                    firstChunkMs: firstChunkLatencyMs,
                    overallFirstChunkMs: firstChunkLatencyMs.map { max(0, containerSnapshot.elapsedMs + max(0, setupMs) + $0) },
                    promptTokens: completionInfo.promptTokenCount,
                    completionTokens: completionInfo.generationTokenCount,
                    prefillMs: prefillMs,
                    generationMs: generationMs,
                    modelOverheadMs: modelOverheadMs,
                    totalOverheadMs: totalOverheadMs
                )
                VoxtLog.llmDebug(
                    "Custom LLM \(request.kind.logLabel) metrics. repo=\(request.repo), containerSource=\(containerSnapshot.source.rawValue), containerLoadMs=\(containerSnapshot.elapsedMs), setupMs=\(max(0, setupMs)), firstChunkMs=\(firstChunkText), overallFirstChunkMs=\(diagnostics.overallFirstChunkMs.map(String.init) ?? "n/a"), promptTokens=\(completionInfo.promptTokenCount), generationTokens=\(completionInfo.generationTokenCount), prefillMs=\(prefillMs), generationMs=\(generationMs), modelOverheadMs=\(modelOverheadMs), totalOverheadMs=\(totalOverheadMs), promptTPS=\(promptTPS), generationTPS=\(generationTPS), stopReason=\(completionInfo.stopReason)"
                )
            }
            lastRunDiagnostics = diagnostics
            VoxtLog.llm(
                """
                Custom LLM \(request.kind.logLabel) output. repo=\(request.repo)
                [output]
                \(VoxtLog.llmPreview(cleaned))
                """
            )
            return cleaned.isEmpty ? request.resultFallback : cleaned
        }
    }

    private func startLogMessage(
        for request: CustomLLMRequestPlan,
        params: GenerateParameters,
        behavior: CustomLLMModelBehavior
    ) -> String {
        var suffix = ""
        if let mode = request.logMode {
            suffix = ", mode=\(mode)"
        }
        let maxTokens = params.maxTokens.map(String.init) ?? "0"
        let prefillStepSize = params.prefill.stepSize.map(String.init) ?? "0"
        return "Custom LLM \(request.kind.logLabel) started. repo=\(request.repo), inputChars=\(request.inputCharacterCount), maxTokens=\(maxTokens), temperature=\(params.temperature), topP=\(params.topP), prefillStep=\(prefillStepSize)\(suffix), family=\(behavior.family.logLabel), thinkingDisabled=\(behavior.disablesThinking)"
    }

    private func contentLogMessage(for request: CustomLLMRequestPlan) -> String {
        var lines = ["Custom LLM \(request.kind.logLabel) content. repo=\(request.repo)"]
        for section in request.contentLogSections {
            lines.append("[\(section.label)]")
            lines.append(VoxtLog.llmPreview(section.content))
        }
        return lines.joined(separator: "\n")
    }

    private func profiledContainer(for repo: String) async throws -> (
        container: ModelContainer,
        source: CustomLLMContainerLoadSource,
        elapsedMs: Int
    ) {
        let repo = Self.canonicalModelRepo(repo)
        let startedAt = Date()
        if let cached = inferenceContainer, inferenceModelRepo == repo {
            return (
                cached,
                .reusedLoaded,
                Int(Date().timeIntervalSince(startedAt) * 1000)
            )
        }

        let container = try await container(for: repo)
        return (
            container,
            .loadedFromDisk,
            Int(Date().timeIntervalSince(startedAt) * 1000)
        )
    }

    private func container(for repo: String) async throws -> ModelContainer {
        let repo = Self.canonicalModelRepo(repo)
        let revision = storageRevision
        guard !deletingRepos.contains(repo) else { throw CancellationError() }
        guard Self.availableModels.contains(where: { $0.id == repo }) else {
            throw NSError(
                domain: "Voxt.CustomLLM",
                code: -10,
                userInfo: [NSLocalizedDescriptionKey: "Unsupported local LLM model: \(repo)"]
            )
        }
        if let cached = inferenceContainer, inferenceModelRepo == repo {
            return cached
        }

        let installation = try await refreshInstallation(repo: repo)
        guard let directory = installation.directory else {
            throw NSError(
                domain: "Voxt.CustomLLM",
                code: -10,
                userInfo: [NSLocalizedDescriptionKey: "Invalid local model path."]
            )
        }
        let token = ProcessInfo.processInfo.environment["HF_TOKEN"]
            ?? Bundle.main.object(forInfoDictionaryKey: "HF_TOKEN") as? String
        if shouldRepairModelDirectory(directory, for: repo) {
            await CustomLLMModelDownloadSupport.repairMissingChatTemplateIfNeeded(
                repo: repo,
                directory: directory,
                preferredBaseURL: hubBaseURL,
                mirrorBaseURL: Self.mirrorHubBaseURL,
                userAgent: Self.hubUserAgent,
                token: token
            )
        }
        try Task.checkCancellation()
        guard revision == storageRevision else { throw CancellationError() }
        if CustomLLMModelCatalog.supportsImageInput(repo: repo) {
            _ = MLXVLM.TrampolineModelFactory.modelFactory()
        }
        let supportsVision = CustomLLMModelCatalog.supportsImageInput(repo: repo)
        let container = try await inferenceLoadCoordinator.value(for: repo) {
            try await MemoryEfficientModelContainerLoader.load(
                from: directory,
                using: LocalTokenizerLoader(),
                supportsVision: supportsVision
            )
        }
        try Task.checkCancellation()
        guard revision == storageRevision, !deletingRepos.contains(repo) else { throw CancellationError() }
        inferenceContainer = container
        inferenceModelRepo = repo
        return container
    }

    private func userInputImages(from attachments: [LLMInputAttachment]) -> [UserInput.Image] {
        attachments.compactMap { attachment in
            switch attachment {
            case .image(let imageAttachment):
                return userInputImage(from: imageAttachment)
            }
        }
    }

    private func userInputImage(from attachment: LLMImageAttachment) -> UserInput.Image? {
        guard let image = CIImage(data: attachment.data, options: [.applyOrientationProperty: true]) else {
            VoxtLog.modelWarning(
                "Custom LLM could not decode image attachment '\(attachment.filename)' for local VLM input."
            )
            return nil
        }
        return .ciImage(image)
    }

    func displayTitle(for repo: String) -> String {
        CustomLLMModelCatalog.displayTitle(for: repo)
    }

    func description(for repo: String) -> String? {
        CustomLLMModelCatalog.description(for: repo)
    }

    nonisolated static func ratingText(for repo: String) -> String {
        CustomLLMModelCatalog.ratingText(for: repo)
    }

    nonisolated static func catalogTagKeys(for repo: String) -> [String] {
        CustomLLMModelCatalog.catalogTagKeys(for: repo)
    }

    nonisolated static func fallbackRemoteSizeText(repo: String) -> String? {
        CustomLLMModelCatalog.fallbackRemoteSizeText(repo: repo)
    }

    nonisolated static func canonicalModelRepo(_ repo: String) -> String {
        CustomLLMModelCatalog.canonicalModelRepo(repo)
    }

    nonisolated static func displayModels(including repo: String? = nil) -> [ModelOption] {
        CustomLLMModelCatalog.displayModels(including: repo)
    }

    nonisolated static func displayModels(includingInstalled repos: Set<String>) -> [ModelOption] {
        CustomLLMModelCatalog.displayModels(includingInstalled: repos)
    }

    func displayModelsIncludingInstalled() -> [ModelOption] {
        // Catalog membership no longer depends on disk state.
        Self.availableModels
    }

    func updateModel(repo: String) {
        let repoSelection = Self.resolveModelRepo(repo)
        let repoWasSupported = Self.isSupportedModelRepo(repo)
        guard repoSelection.effectiveRepo != modelRepo else { return }
        if !repoWasSupported {
            VoxtLog.modelWarning("Unsupported custom LLM repo '\(repo)' requested. Falling back to \(repoSelection.effectiveRepo).")
        } else if repoSelection.effectiveRepo != repo {
            VoxtLog.modelInfo("Canonicalized custom LLM repo '\(repo)' -> '\(repoSelection.effectiveRepo)'")
        }
            VoxtLog.modelInfo("Custom LLM model changed: \(modelRepo) -> \(repoSelection.effectiveRepo)")
        modelRepo = repoSelection.effectiveRepo
        releaseInferenceResources()
        lastLoggedModelPresence = nil
        lastInvalidRepoLogged = nil
        checkExistingModel()
        fetchRemoteSize()
    }

    static func isSupportedModelRepo(_ repo: String) -> Bool {
        CustomLLMModelCatalog.isSupportedModelRepo(repo)
    }

    private nonisolated static func resolveModelRepo(_ requestedRepo: String) -> CustomLLMRepoSelection {
        guard CustomLLMModelCatalog.isSupportedModelRepo(requestedRepo) else {
            return CustomLLMRepoSelection(
                requestedRepo: requestedRepo,
                effectiveRepo: defaultModelRepo
            )
        }
        return CustomLLMRepoSelection(
            requestedRepo: requestedRepo,
            effectiveRepo: CustomLLMModelCatalog.canonicalModelRepo(requestedRepo)
        )
    }

    func updateHubBaseURL(_ url: URL) {
        guard url != hubBaseURL else { return }
        VoxtLog.modelInfo("Custom LLM hub base URL changed: \(hubBaseURL.absoluteString) -> \(url.absoluteString)")
        hubBaseURL = url
        fetchRemoteSize()
    }

    private func downloadSourceTargetKey(for repo: String) -> String {
        ModelDownloadSourceSelectionStore.targetKey(namespace: "custom-llm", identifier: repo)
    }

    private func clearSelectedDownloadSource(for repo: String) {
        ModelDownloadSourceSelectionStore.clearSourceID(for: downloadSourceTargetKey(for: repo))
    }

    private func downloadSourceCandidates() -> [ModelDownloadSourceCandidate] {
        [
            ModelDownloadSourceCandidate(
                id: "huggingface",
                displayName: "Hugging Face",
                url: Self.defaultHubBaseURL
            ),
            ModelDownloadSourceCandidate(
                id: "hf-mirror",
                displayName: "HF Mirror",
                url: Self.mirrorHubBaseURL
            ),
        ]
    }

    func isModelDownloaded(repo: String) -> Bool {
        let repo = Self.canonicalModelRepo(repo)
        guard Self.availableModels.contains(where: { $0.id == repo }) else { return false }
        requestInstallation(repo)
        return downloadedStateByRepo[repo] ?? false
    }

    /// Preflight must not reject an installed model while its first background
    /// scan is pending. The async inference entry point waits for validation.
    func canAttemptInference(repo: String) -> Bool {
        isModelDownloaded(repo: repo) || isCheckingInstallation(repo: repo)
    }

    func isCheckingInstallation(repo: String) -> Bool {
        let repo = Self.canonicalModelRepo(repo)
        guard Self.availableModels.contains(where: { $0.id == repo }) else { return false }
        requestInstallation(repo)
        return downloadedStateByRepo[repo] == nil
    }

    @discardableResult
    func refreshInstallation(repo: String) async throws -> ModelInstallationSnapshot {
        let repo = Self.canonicalModelRepo(repo)
        guard !isShuttingDownForApplicationTermination, !deletingRepos.contains(repo),
              Self.availableModels.contains(where: { $0.id == repo }) else { throw CancellationError() }
        let request = installationRequest(repo)
        return try await installationCache.value(repo) { request.scan() }
    }

    func hasResumableDownload(repo: String) -> Bool {
        let repo = Self.canonicalModelRepo(repo)
        requestInstallation(repo)
        return installationCache.peek(repo)?.hasPartialDownload ?? false
    }

    private func shouldReuseSavedDownloadSource(for repo: String) -> Bool {
        if case .paused = state(for: repo) {
            return true
        }
        return hasResumableDownload(repo: repo)
    }

    func modelSizeOnDisk(repo: String) -> String {
        let repo = Self.canonicalModelRepo(repo)
        requestInstallation(repo)
        return localSizeTextByRepo[repo] ?? ""
    }

    func cachedModelSizeText(repo: String) -> String? {
        localSizeTextByRepo[Self.canonicalModelRepo(repo)]
    }

    func modelDirectoryURL(repo: String) -> URL? {
        let repo = Self.canonicalModelRepo(repo)
        requestInstallation(repo)
        let snapshot = installationCache.peek(repo)
        return snapshot?.directory ?? snapshot?.existingDirectory
    }

    func remoteSizeText(repo: String) -> String {
        Self.fallbackRemoteSizeText(repo: repo) ?? "Unknown"
    }

    func ensureRemoteSizeLoaded(repo: String) {
        _ = repo
    }

    func checkExistingModel(refresh: Bool = false) {
        guard !isShuttingDownForApplicationTermination else { return }
        if refresh, !installationCache.hasPendingRequest(modelRepo), downloadTasksByRepo[modelRepo] == nil {
            invalidateLocalCache(for: modelRepo)
        }
        guard Self.availableModels.contains(where: { $0.id == modelRepo }) else {
            setState(.notDownloaded, for: modelRepo)
            return
        }
        requestInstallation(modelRepo)
        guard let snapshot = installationCache.peek(modelRepo) else {
            return
        }
        guard downloadTasksByRepo[modelRepo] == nil else { return }
        applyInstallationState(snapshot, repo: modelRepo)
    }

    private func applyInstallationState(_ snapshot: ModelInstallationSnapshot, repo: String) {
        if snapshot.isInstalled {
            setState(.downloaded, for: repo)
        } else if snapshot.hasPartialDownload {
            setPausedState(progress: 0, completed: 0, total: 0, currentFile: nil, completedFiles: 0, totalFiles: 0, for: repo)
        } else {
            setState(.notDownloaded, for: repo)
        }
    }

    func state(for repo: String) -> ModelState {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        return catalogSnapshot(for: canonicalRepo).state
    }

    func catalogSnapshot(for repo: String) -> CatalogSnapshot {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        let isDownloaded = isModelDownloaded(repo: canonicalRepo)
        let hasResumableDownload = hasResumableDownload(repo: canonicalRepo)
        let resolvedState = MLXModelPerRepoStateSupport.resolvedState(
            for: canonicalRepo,
            currentRepo: modelRepo,
            currentState: state,
            storedStates: stateByRepo,
            isDownloaded: { _ in isDownloaded },
            hasResumableDownload: { _ in hasResumableDownload }
        )
        return CatalogSnapshot(
            repo: canonicalRepo,
            isDownloaded: isDownloaded,
            hasResumableDownload: hasResumableDownload,
            state: resolvedState,
            pausedStatusMessage: pausedStatusMessage(for: canonicalRepo),
            hasActiveDownloadTask: downloadTasksByRepo[canonicalRepo] != nil
        )
    }

    func pausedStatusMessage(for repo: String) -> String? {
        MLXModelPerRepoStateSupport.pausedStatusMessage(
            for: repo,
            storedMessages: pausedStatusMessageByRepo,
            canonicalize: Self.canonicalModelRepo(_:)
        )
    }

    func isDownloading(repo: String) -> Bool {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        if downloadTasksByRepo[canonicalRepo] != nil { return true }
        if case .downloading = state(for: canonicalRepo) { return true }
        return false
    }

    func isPaused(repo: String) -> Bool {
        if case .paused = state(for: repo) { return true }
        return false
    }

    func downloadModel() async {
        guard !isShuttingDownForApplicationTermination else { return }
        await performDownload(forRepo: modelRepo)
    }

    private func performDownload(forRepo repo: String) async {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        if downloadTasksByRepo[canonicalRepo] != nil || deletingRepos.contains(canonicalRepo) { return }
        do {
            let installation = try await refreshInstallation(repo: canonicalRepo)
            if installation.isInstalled { return }
        } catch { return }
        guard downloadTasksByRepo[canonicalRepo] == nil, !isShuttingDownForApplicationTermination,
              !deletingRepos.contains(canonicalRepo) else { return }
        installationCache.invalidate(canonicalRepo)

        SystemNotificationSupport.requestAuthorizationIfNeeded()
        let revision = storageRevision
        downloadStorageRevisions[canonicalRepo] = revision
        let task = Task { [weak self] in
            guard let self else { return }
            defer {
                cancelDownloadProgressTask(for: canonicalRepo)
                downloadTasksByRepo[canonicalRepo] = nil
                downloadStopActionsByRepo[canonicalRepo] = nil
                activeDownloadRepos.remove(canonicalRepo)
                downloadStorageRevisions.removeValue(forKey: canonicalRepo)
                if revision != storageRevision {
                    invalidateLocalCache(for: canonicalRepo)
                    clearPerRepoState(for: canonicalRepo)
                    requestInstallation(canonicalRepo)
                }
            }
            guard revision == storageRevision, !Task.isCancelled else { return }
            activeDownloadRepos.insert(canonicalRepo)
            if let pausedState = pausedDownloadSnapshot(for: canonicalRepo) {
                setDownloadingState(
                    progress: pausedState.progress,
                    completed: pausedState.completed,
                    total: pausedState.total,
                    currentFile: pausedState.currentFile,
                    completedFiles: pausedState.completedFiles,
                    totalFiles: pausedState.totalFiles,
                    for: canonicalRepo
                )
            } else {
                setDownloadingState(
                    progress: 0,
                    completed: 0,
                    total: 0,
                    currentFile: nil,
                    completedFiles: 0,
                    totalFiles: 0,
                    for: canonicalRepo
                )
            }

            do {
                setPausedStatusMessage(nil, for: canonicalRepo)
                let modelDir = try await performDownloadWithFallback(for: canonicalRepo)
                let valid = await Task.detached(priority: .utility) {
                    CustomLLMModelStorageSupport.isModelDirectoryValid(modelDir)
                }.value
                try Task.checkCancellation()
                guard revision == storageRevision else { throw CancellationError() }
                guard valid else {
                    setPausedStatusMessage(nil, for: canonicalRepo)
                    let message = "Downloaded files are incomplete."
                    setState(.error(message), for: canonicalRepo)
                    SystemNotificationSupport.postModelDownloadFailed(
                        modelName: displayTitle(for: canonicalRepo),
                        message: message
                    )
                    VoxtLog.modelError("Custom LLM download produced incomplete files: \(canonicalRepo)")
                    return
                }
                try Task.checkCancellation()
                guard revision == storageRevision else { throw CancellationError() }
                markDownloadCompleted(for: canonicalRepo)
                SystemNotificationSupport.postModelDownloadSucceeded(
                    modelName: displayTitle(for: canonicalRepo)
                )
                VoxtLog.modelInfo("Custom LLM download completed: \(canonicalRepo)")
            } catch is CancellationError {
                guard revision == storageRevision else { return }
                cancelDownloadProgressTask(for: canonicalRepo)
                switch downloadStopActionsByRepo[canonicalRepo] {
                case .pause:
                    setPausedStatusMessage(nil, for: canonicalRepo)
                    VoxtLog.modelInfo("Custom LLM download paused: \(canonicalRepo)")
                case .cancel, .none:
                    setPausedStatusMessage(nil, for: canonicalRepo)
                    await cleanupPartialDownload(for: canonicalRepo)
                    guard revision == storageRevision else { return }
                    clearSelectedDownloadSource(for: canonicalRepo)
                    markCancelledDownloadUnavailable(for: canonicalRepo)
                    VoxtLog.modelWarning("Custom LLM download cancelled: \(canonicalRepo)")
                }
            } catch {
                guard revision == storageRevision else { return }
                cancelDownloadProgressTask(for: canonicalRepo)
                if pauseDownloadIfNetworkIssue(error, repo: canonicalRepo) {
                    return
                }
                setPausedStatusMessage(nil, for: canonicalRepo)
                await clearHubCache(for: canonicalRepo)
                guard revision == storageRevision else { return }
                let message = "Download failed: \(error.localizedDescription)"
                setState(.error(message), for: canonicalRepo)
                SystemNotificationSupport.postModelDownloadFailed(
                    modelName: displayTitle(for: canonicalRepo),
                    message: message
                )
                VoxtLog.modelError("Custom LLM download failed: \(canonicalRepo), error=\(error.localizedDescription)")
            }
        }
        downloadTasksByRepo[canonicalRepo] = task
        await task.value
    }

    func downloadModel(repo: String) async {
        guard !isShuttingDownForApplicationTermination else { return }
        await performDownload(forRepo: repo)
    }

    func cancelDownload(repo: String) {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        if let task = downloadTasksByRepo[canonicalRepo] {
            downloadStopActionsByRepo[canonicalRepo] = .cancel
            setPausedStatusMessage(nil, for: canonicalRepo)
            setState(.notDownloaded, for: canonicalRepo)
            clearSelectedDownloadSource(for: canonicalRepo)
            task.cancel()
            return
        }

        schedulePartialCleanup(for: canonicalRepo)
    }

    func cancelDownloadAndWait(repo: String) async {
        cancelDownload(repo: repo)
        await downloadTasksByRepo[Self.canonicalModelRepo(repo)]?.value
    }

    private func schedulePartialCleanup(for repo: String) {
        guard !isModelDownloaded(repo: repo), !deletingRepos.contains(repo) else { return }
        let directories = [writeCacheDirectory(for: repo)].compactMap { $0 }
        let root = writeRootURL()
        let revision = storageRevision
        invalidateLocalCache(for: repo)
        clearSelectedDownloadSource(for: repo)
        setPausedStatusMessage(nil, for: repo)
        setState(.notDownloaded, for: repo)
        activeDownloadRepos.insert(repo)
        downloadStopActionsByRepo[repo] = .cancel
        let task = Task { [weak self] in
            guard let self else { return }
            defer {
                downloadTasksByRepo[repo] = nil
                downloadStopActionsByRepo[repo] = nil
                activeDownloadRepos.remove(repo)
                invalidateLocalCache(for: repo)
                requestInstallation(repo)
            }
            do {
                try await ModelDiskOperations.remove(directories)
                try await ModelDiskOperations.perform {
                    if let id = Repo.ID(rawValue: repo) {
                        CustomLLMModelStorageSupport.clearHubCache(for: id, rootDirectory: root)
                    }
                }
                guard revision == storageRevision else { return }
                setState(.notDownloaded, for: repo)
            } catch {
                guard revision == storageRevision else { return }
                setState(.error("Couldn't remove partial model files: \(error.localizedDescription)"), for: repo)
            }
        }
        downloadTasksByRepo[repo] = task
    }

    func refreshStorageRoot() {
        let nextRoots = [writeRootURL()] + readableRootURLs()
        if nextRoots != storageRoots {
            storageRoots = nextRoots
            storageRevision = UUID()
            // Old work may finish after cancellation. Its captured revision must
            // not publish into, clean up, or start a fallback in the new root.
            for (repo, task) in downloadTasksByRepo {
                downloadStopActionsByRepo[repo] = .pause
                task.cancel()
                cancelDownloadProgressTask(for: repo)
            }
            releaseInferenceResources()
        }
        downloadedStateByRepo.removeAll()
        installationCache.invalidateAll()
        installationRevision &+= 1
        localSizeTextByRepo.removeAll()
        MLXModelPerRepoStateSupport.resetCustomLLMStorageRootState(
            currentPausedStatusMessage: &pausedStatusMessage,
            storedStates: &stateByRepo,
            storedMessages: &pausedStatusMessageByRepo
        )
        checkExistingModel()
    }

    func pauseDownload() {
        pauseDownload(repo: modelRepo)
    }

    func pauseDownload(repo: String) {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        guard let task = downloadTasksByRepo[canonicalRepo] else { return }
        downloadStopActionsByRepo[canonicalRepo] = .pause
        setPausedStatusMessage(nil, for: canonicalRepo)
        if let snapshot = downloadingSnapshot(for: canonicalRepo) {
            setPausedState(
                progress: snapshot.progress,
                completed: snapshot.completed,
                total: snapshot.total,
                currentFile: snapshot.currentFile,
                completedFiles: snapshot.completedFiles,
                totalFiles: snapshot.totalFiles,
                for: canonicalRepo
            )
        }
        task.cancel()
        cancelDownloadProgressTask(for: canonicalRepo)
    }

    func cancelDownload() {
        cancelDownload(repo: modelRepo)
    }

    private func cancelDownloadProgressTask(for repo: String) {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        downloadProgressTasksByRepo[canonicalRepo]?.cancel()
        downloadProgressTasksByRepo[canonicalRepo] = nil
    }

    private func performDownloadWithFallback(for repo: String) async throws -> URL {
        _ = try ModelStorageDirectoryManager.requireWriteRootURL()
        let canonicalRepo = Self.canonicalModelRepo(repo)
        let token = ProcessInfo.processInfo.environment["HF_TOKEN"]
            ?? Bundle.main.object(forInfoDictionaryKey: "HF_TOKEN") as? String
        let selection = try await ModelDownloadSourceSelector.select(
            candidates: downloadSourceCandidates(),
            targetKey: downloadSourceTargetKey(for: canonicalRepo),
            reuseSavedSource: shouldReuseSavedDownloadSource(for: canonicalRepo)
        ) { candidate in
            let startedAt = Date()
            let context = try await CustomLLMModelDownloadSupport.makeDownloadContext(
                repo: canonicalRepo,
                baseURL: candidate.url,
                userAgent: Self.hubUserAgent,
                token: token
            )
            let elapsed = max(Date().timeIntervalSince(startedAt), 0.001)
            return (elapsed, context.totalBytes)
        }

        VoxtLog.modelInfo(
            "Selected Custom LLM download source. repo=\(canonicalRepo), source=\(selection.candidate.displayName), url=\(selection.candidate.url.absoluteString), reusedSavedSource=\(selection.reusedSavedSource), probes=\(ModelDownloadSourceSelector.logSummary(for: selection))"
        )

        var lastError: Error?
        for candidate in downloadAttemptCandidates(from: selection) {
            try Task.checkCancellation()
            do {
                return try await performDownload(using: candidate.url, repo: canonicalRepo)
            } catch is CancellationError {
                throw CancellationError()
            } catch {
                try Task.checkCancellation()
                lastError = error
                VoxtLog.modelWarning(
                    "Custom LLM download source failed. repo=\(canonicalRepo), source=\(candidate.displayName), error=\(error.localizedDescription)"
                )
                await clearHubCache(for: canonicalRepo)
            }
        }

        clearSelectedDownloadSource(for: canonicalRepo)
        throw lastError ?? NSError(
            domain: "CustomLLMModelManager",
            code: 1004,
            userInfo: [NSLocalizedDescriptionKey: "All Custom LLM download sources failed."]
        )
    }

    private func downloadAttemptCandidates(
        from selection: ModelDownloadSourceSelection
    ) -> [ModelDownloadSourceCandidate] {
        guard !selection.reusedSavedSource, !selection.probeResults.isEmpty else {
            return [selection.candidate]
        }

        let candidates = selection.probeResults
            .filter(\.isReachable)
            .sorted(by: { $0.elapsed < $1.elapsed })
            .map(\.candidate)
        return candidates.isEmpty ? [selection.candidate] : candidates
    }

    private func performDownload(using baseURL: URL, repo: String) async throws -> URL {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        let token = ProcessInfo.processInfo.environment["HF_TOKEN"]
            ?? Bundle.main.object(forInfoDictionaryKey: "HF_TOKEN") as? String
        let context = try await CustomLLMModelDownloadSupport.makeDownloadContext(
            repo: canonicalRepo,
            baseURL: baseURL,
            userAgent: Self.hubUserAgent,
            token: token
        )
        VoxtLog.modelInfo("Custom LLM download started: repo=\(context.repoID.description), files=\(context.entries.count), baseURL=\(baseURL.absoluteString)")

        let totalBytes = context.totalBytes
        let totalFiles = context.entries.count
        var completedBytes: Int64 = 0

        guard let modelDir = writeCacheDirectory(for: canonicalRepo) else {
            throw NSError(
                domain: "Voxt.CustomLLM",
                code: 1002,
                userInfo: [NSLocalizedDescriptionKey: "Invalid model cache directory."]
            )
        }
        try FileManager.default.createDirectory(at: modelDir, withIntermediateDirectories: true)

        for (index, entry) in context.entries.enumerated() {
            let expectedFileBytes = max(entry.size ?? 0, 0)
            let progress = Progress(totalUnitCount: max(expectedFileBytes, 1))
            let fileBaseCompleted = completedBytes
            setDownloadingState(
                progress: min(1, Double(completedBytes) / Double(totalBytes)),
                completed: min(completedBytes, totalBytes),
                total: totalBytes,
                currentFile: entry.path,
                completedFiles: index,
                totalFiles: totalFiles,
                for: canonicalRepo
            )

            let destination = try CustomLLMModelStorageSupport.destinationFileURL(
                for: entry.path,
                under: modelDir
            )
            if MLXModelDownloadSupport.canReuseExistingDownload(
                at: destination,
                expectedSize: entry.size,
                fileManager: .default
            ) {
                let delta = max(expectedFileBytes, Int64((try? destination.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0))
                completedBytes += max(delta, 0)
                setDownloadingState(
                    progress: min(1, Double(completedBytes) / Double(totalBytes)),
                    completed: min(completedBytes, totalBytes),
                    total: totalBytes,
                    currentFile: nil,
                    completedFiles: index + 1,
                    totalFiles: totalFiles,
                    for: canonicalRepo
                )
                VoxtLog.modelInfo("Custom LLM download resume reused existing file: \(entry.path)", verbose: true)
                continue
            }
            cancelDownloadProgressTask(for: canonicalRepo)
            downloadProgressTasksByRepo[canonicalRepo] = Task { [weak self] in
                let startTime = Date()
                while !Task.isCancelled {
                    await MainActor.run {
                        guard !Task.isCancelled, let self else { return }
                        let effectiveCurrentFileCompleted = CustomLLMModelDownloadSupport.inFlightBytes(
                            progress: progress,
                            expectedFileBytes: expectedFileBytes,
                            startTime: startTime
                        )
                        let aggregateCompleted = min(
                            fileBaseCompleted + effectiveCurrentFileCompleted,
                            totalBytes
                        )
                        self.setDownloadingState(
                            progress: min(1, Double(aggregateCompleted) / Double(totalBytes)),
                            completed: aggregateCompleted,
                            total: totalBytes,
                            currentFile: entry.path,
                            completedFiles: index,
                            totalFiles: totalFiles,
                            for: canonicalRepo
                        )
                    }
                    try? await Task.sleep(for: .milliseconds(200))
                }
            }

            try await downloadEntryWithRetry(
                context: context,
                entryPath: entry.path,
                destination: destination,
                progress: progress,
                baseURL: baseURL,
                bearerToken: token
            )
            cancelDownloadProgressTask(for: canonicalRepo)

            let delta = max(expectedFileBytes, max(progress.completedUnitCount, 0))
            completedBytes += max(delta, 0)
            setDownloadingState(
                progress: min(1, Double(completedBytes) / Double(totalBytes)),
                completed: min(completedBytes, totalBytes),
                total: totalBytes,
                currentFile: nil,
                completedFiles: index + 1,
                totalFiles: totalFiles,
                for: canonicalRepo
            )
        }

        return modelDir
    }

    private func downloadEntryWithRetry(
        context: CustomLLMModelDownloadSupport.DownloadContext,
        entryPath: String,
        destination: URL,
        progress: Progress,
        baseURL: URL,
        bearerToken: String?
    ) async throws {
        let remoteURL = try MLXModelDownloadSupport.fileResolveURL(
            baseURL: baseURL,
            repo: context.repoID.description,
            path: entryPath
        )
        _ = try await ResumableModelDownloadSupport.download(
            ResumableDownloadDescriptor(
                sourceURL: remoteURL,
                destinationURL: destination,
                relativePath: entryPath,
                expectedSize: progress.totalUnitCount > 1 ? progress.totalUnitCount : nil,
                userAgent: Self.hubUserAgent,
                bearerToken: bearerToken,
                disableProxy: MLXModelDownloadSupport.isMirrorHost(baseURL)
            ),
            progress: progress
        )
    }

    @discardableResult
    func deleteModel() async -> Result<Void, Error> {
        await deleteModel(repo: modelRepo)
    }

    @discardableResult
    func deleteModel(repo: String) async -> Result<Void, Error> {
        let repo = Self.canonicalModelRepo(repo)
        guard !hasActiveInference, !deletingRepos.contains(repo) else {
            return .failure(NSError(domain: "Voxt.CustomLLM", code: 1005,
                userInfo: [NSLocalizedDescriptionKey: "The model is currently in use. Try again when generation finishes."]))
        }
        deletingRepos.insert(repo)
        defer { deletingRepos.remove(repo) }
        let roots = readableRootURLs()
        let directories = roots.compactMap { CustomLLMModelStorageSupport.cacheDirectory(for: repo, rootDirectory: $0) }
        invalidateLocalCache(for: repo)
        let download = downloadTasksByRepo[repo]
        if download != nil { cancelDownload(repo: repo) }
        let loads = inferenceLoadCoordinator.cancelAll()
        await download?.value
        for task in loads { await task.waitForCompletion() }
        if inferenceModelRepo == repo { releaseInferenceResources() }
        do {
            try await ModelDiskOperations.remove(directories)
            await Task.detached(priority: .utility) {
                if let id = Repo.ID(rawValue: repo) {
                    for root in roots { CustomLLMModelStorageSupport.clearHubCache(for: id, rootDirectory: root) }
                }
            }.value
            clearSelectedDownloadSource(for: repo)
            invalidateLocalCache(for: repo)
            clearPerRepoState(for: repo)
            setState(.notDownloaded, for: repo)
            requestInstallation(repo)
            return .success(())
        } catch {
            invalidateLocalCache(for: repo)
            setState(.error("Couldn't uninstall local LLM: \(error.localizedDescription)"), for: repo)
            return .failure(error)
        }
    }

    private func invalidateLocalCache(for repo: String) {
        installationCache.invalidate(repo)
        installationRevision &+= 1
        downloadedStateByRepo.removeValue(forKey: repo)
        localSizeTextByRepo.removeValue(forKey: repo)
    }

    private func markDownloadCompleted(for repo: String) {
        installationCache.invalidate(repo)
        requestInstallation(repo)
        downloadedStateByRepo[repo] = true
        localSizeTextByRepo.removeValue(forKey: repo)
        if repo == modelRepo {
            checkExistingModel()
        } else {
            setState(.downloaded, for: repo)
        }
    }

    private func markCancelledDownloadUnavailable(for repo: String) {
        invalidateLocalCache(for: repo)
        if repo == modelRepo {
            checkExistingModel()
        } else {
            setState(.notDownloaded, for: repo)
        }
    }

    private func cleanupPartialDownload(for repo: String) async {
        let directories = [writeCacheDirectory(for: repo)].compactMap { $0 }
        // This compensating cleanup is requested BY cancellation, so it must not
        // inherit the cancelled download task's flag. Paths are captured first.
        let result = await Task.detached(priority: .utility) {
            try await ModelDiskOperations.remove(directories)
        }.result
        if case .failure(let error) = result {
            VoxtLog.modelWarning("Partial model cleanup failed. repo=\(repo), error=\(error.localizedDescription)")
        }
    }

    private func installationRequest(_ repo: String) -> ModelInstallationRequest {
        let directories = readableRootURLs().compactMap { CustomLLMModelStorageSupport.cacheDirectory(for: repo, rootDirectory: $0) }
        return ModelInstallationRequest(
            directories: directories,
            partialDirectories: [writeCacheDirectory(for: repo)].compactMap { $0 },
            validate: { CustomLLMModelStorageSupport.isModelDirectoryValid($0) }
        )
    }

    private func requestInstallation(_ repo: String) {
        guard !isShuttingDownForApplicationTermination, !deletingRepos.contains(repo), installationCache.needsRequest(repo) else { return }
        guard Self.availableModels.contains(where: { $0.id == repo }) else { return }
        let request = installationRequest(repo)
        installationCache.request(repo) { request.scan() }
    }

    private func fetchRemoteSize() {
        sizeTask?.cancel()
        let repo = modelRepo
        if let fallback = CustomLLMModelCatalog.fallbackRemoteSizeInfo(repo: repo) {
            sizeState = .ready(bytes: fallback.bytes, text: fallback.text)
        } else {
            sizeState = .error("Size unavailable")
        }
    }

    func prefetchAllModelSizes() {
        prefetchTask?.cancel()
        prefetchTask = nil
    }

    private func setDownloadingState(
        progress: Double,
        completed: Int64,
        total: Int64,
        currentFile: String?,
        completedFiles: Int,
        totalFiles: Int,
        for repo: String
    ) {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        guard downloadTasksByRepo[canonicalRepo] != nil,
              downloadStorageRevisions[canonicalRepo] == storageRevision,
              downloadStopActionsByRepo[canonicalRepo] == nil else { return }
        let nextState = ModelState.downloading(
            progress: progress,
            completed: completed,
            total: total,
            currentFile: currentFile,
            completedFiles: completedFiles,
            totalFiles: totalFiles
        )
        setState(nextState, for: canonicalRepo)
    }

    private func setPausedState(
        progress: Double,
        completed: Int64,
        total: Int64,
        currentFile: String?,
        completedFiles: Int,
        totalFiles: Int,
        for repo: String
    ) {
        let nextState = ModelState.paused(
            progress: progress,
            completed: completed,
            total: total,
            currentFile: currentFile,
            completedFiles: completedFiles,
            totalFiles: totalFiles
        )
        setState(nextState, for: repo)
    }

    private func setState(_ nextState: ModelState, for repo: String) {
        MLXModelPerRepoStateSupport.applyState(
            nextState,
            for: repo,
            currentRepo: modelRepo,
            currentState: &state,
            storedStates: &stateByRepo
        )
    }

    private func setPausedStatusMessage(_ message: String?, for repo: String) {
        MLXModelPerRepoStateSupport.applyPausedStatusMessage(
            message,
            for: repo,
            currentRepo: modelRepo,
            currentMessage: &pausedStatusMessage,
            storedMessages: &pausedStatusMessageByRepo,
            canonicalize: Self.canonicalModelRepo(_:)
        )
    }

    private func clearPerRepoState(for repo: String) {
        MLXModelPerRepoStateSupport.clearCustomLLMState(
            for: repo,
            currentRepo: modelRepo,
            currentPausedStatusMessage: &pausedStatusMessage,
            storedStates: &stateByRepo,
            storedMessages: &pausedStatusMessageByRepo
        )
    }

    private func pauseDownloadIfNetworkIssue(_ error: Error, repo: String) -> Bool {
        let canonicalRepo = Self.canonicalModelRepo(repo)
        guard let message = MLXModelDownloadSupport.pauseMessageForInterruptedDownload(error) else {
            return false
        }
        let snapshot = downloadingSnapshot(for: canonicalRepo) ?? pausedDownloadSnapshot(for: canonicalRepo)
        setPausedStatusMessage(message, for: canonicalRepo)
        if let snapshot {
            setPausedState(
                progress: snapshot.progress,
                completed: snapshot.completed,
                total: snapshot.total,
                currentFile: snapshot.currentFile,
                completedFiles: snapshot.completedFiles,
                totalFiles: snapshot.totalFiles,
                for: canonicalRepo
            )
        } else {
            setPausedState(
                progress: 0,
                completed: 0,
                total: 0,
                currentFile: nil,
                completedFiles: 0,
                totalFiles: 0,
                for: canonicalRepo
            )
        }
        VoxtLog.modelWarning("Custom LLM download auto-paused after network issue. repo=\(canonicalRepo), error=\(error.localizedDescription)")
        return true
    }

    private func downloadingSnapshot(for repo: String) -> (
        progress: Double,
        completed: Int64,
        total: Int64,
        currentFile: String?,
        completedFiles: Int,
        totalFiles: Int
    )? {
        guard case .downloading(
            let progress,
            let completed,
            let total,
            let currentFile,
            let completedFiles,
            let totalFiles
        ) = state(for: repo) else {
            return nil
        }
        return (progress, completed, total, currentFile, completedFiles, totalFiles)
    }

    private func pausedDownloadSnapshot(for repo: String) -> (
        progress: Double,
        completed: Int64,
        total: Int64,
        currentFile: String?,
        completedFiles: Int,
        totalFiles: Int
    )? {
        guard case .paused(
            let progress,
            let completed,
            let total,
            let currentFile,
            let completedFiles,
            let totalFiles
        ) = state(for: repo) else {
            return nil
        }
        return (progress, completed, total, currentFile, completedFiles, totalFiles)
    }

    private func makeChatSession(
        container: ModelContainer,
        instructions: String,
        conversationHistory: [RewriteConversationPromptTurn],
        repo: String,
        behavior: CustomLLMModelBehavior,
        settings: LLMGenerationSettings
    ) -> ChatSession {
        let additionalContext = localThinkingAdditionalContext(
            behavior: behavior,
            settings: settings
        )
        let history = conversationHistory.flatMap { turn -> [Chat.Message] in
            var messages: [Chat.Message] = []
            let userMessage = turn.modelUserMessage
            if !userMessage.isEmpty {
                messages.append(.user(userMessage))
            }
            let assistantMessage = turn.resultContent.trimmingCharacters(in: .whitespacesAndNewlines)
            if !assistantMessage.isEmpty {
                messages.append(.assistant(assistantMessage))
            }
            return messages
        }
        let session = ChatSession(
            container,
            instructions: instructions,
            history: history,
            additionalContext: additionalContext
        )
        if additionalContext?["enable_thinking"] as? Bool == false {
            VoxtLog.llmDebug("Custom LLM thinking disabled for repo=\(repo) using chat-template additionalContext.")
        } else if additionalContext?["enable_thinking"] as? Bool == true {
            VoxtLog.llmDebug("Custom LLM thinking enabled for repo=\(repo) using chat-template additionalContext.")
        }
        return session
    }

    private func localThinkingAdditionalContext(
        behavior: CustomLLMModelBehavior,
        settings: LLMGenerationSettings
    ) -> [String: any Sendable]? {
        switch settings.thinking.mode {
        case .providerDefault:
            return CustomLLMModelBehavior.thinkingOffAdditionalContext
        case .off:
            return CustomLLMModelBehavior.thinkingOffAdditionalContext
        case .on:
            return CustomLLMModelBehavior.thinkingOnAdditionalContext
        case .effort, .budget:
            return CustomLLMModelBehavior.thinkingOffAdditionalContext
        }
    }

    private func structuredOutputPrompt(taskInstruction: String, input: String) -> String {
        """
        \(taskInstruction)

        Return only valid JSON with exactly one key:
        {"resultText":"..."}

        Input:
        \(input)
        """
    }

    private func dictionaryHistoryScanStructuredOutputPrompt(_ prompt: String) -> String {
        """
        Analyze the following task and return only valid JSON.

        Final answer requirements:
        - Return only a JSON array.
        - Every item must be an object with exactly one key: "term".
        - Example: [{"term":"OpenAI"},{"term":"MCP"}]
        - If no term qualifies, return [].
        - Do not wrap the array in another object.
        - Do not return prose, markdown, code fences, or explanations.

        Task:
        \(prompt)
        """
    }

    private func extractResultText(_ output: String) -> String {
        let normalized = sanitizeModelOutput(output)
        if let parsed = decodeStructuredResultText(from: normalized) {
            return parsed
        }
        return normalized
    }

    private func decodeStructuredResultText(from output: String) -> String? {
        for candidate in jsonCandidates(from: output) {
            guard let data = candidate.data(using: .utf8),
                  let decoded = try? JSONDecoder().decode(TextResultPayload.self, from: data) else {
                continue
            }
            let text = CustomLLMOutputSanitizer.normalizeResultText(decoded.resultText)
            if !text.isEmpty {
                return text
            }
        }
        return nil
    }

    private func jsonCandidates(from output: String) -> [String] {
        let normalized = output.trimmingCharacters(in: .whitespacesAndNewlines)
        var candidates: [String] = [normalized]

        let unfenced = CustomLLMOutputSanitizer.unwrapCodeFenceIfNeeded(normalized)
        if unfenced != normalized {
            candidates.append(unfenced)
        }

        if let jsonObject = Self.extractFirstJSONObject(in: unfenced),
           !candidates.contains(jsonObject) {
            candidates.append(jsonObject)
        }

        return candidates
    }

    private static func extractFirstJSONObject(in text: String) -> String? {
        guard let start = text.firstIndex(of: "{"),
              let end = text.lastIndex(of: "}"),
              start <= end else {
            return nil
        }
        return String(text[start...end])
    }

    private func sanitizeModelOutput(_ output: String) -> String {
        let cleaned = CustomLLMOutputSanitizer.normalizeResultText(output)
        if cleaned != output.trimmingCharacters(in: .whitespacesAndNewlines) {
            VoxtLog.llm(
                """
                Custom LLM output sanitized.
                [raw]
                \(VoxtLog.llmPreview(output))
                [cleaned]
                \(VoxtLog.llmPreview(cleaned))
                """
            )
        }
        return cleaned
    }

    private func writeCacheDirectory(for repo: String) -> URL? {
        CustomLLMModelStorageSupport.cacheDirectory(
            for: repo,
            rootDirectory: writeRootURL()
        )
    }

    private func clearHubCache(for repo: String) async {
        let root = writeRootURL()
        await Task.detached(priority: .utility) {
            try? await ModelDiskOperations.perform {
                if let id = Repo.ID(rawValue: repo) {
                    CustomLLMModelStorageSupport.clearHubCache(for: id, rootDirectory: root)
                }
            }
        }.value
    }

    private func shouldRepairModelDirectory(_ directory: URL, for repo: String) -> Bool {
        guard let writableDirectory = writeCacheDirectory(for: repo) else { return false }
        return writableDirectory.standardizedFileURL.path == directory.standardizedFileURL.path
    }

    private func writeRootURL() -> URL {
        ModelStorageDirectoryManager.resolvedWriteRootURL()
    }

    private func readableRootURLs() -> [URL] {
        ModelStorageDirectoryManager.resolvedReadableRootURLs()
    }

    private func withActiveInference<T>(
        _ operation: () async throws -> T
    ) async throws -> T {
        guard !isShuttingDownForApplicationTermination else { throw CancellationError() }
        beginActiveInference()
        defer { endActiveInference() }
        return try await operation()
    }

    private func beginActiveInference() {
        activeInferenceCount += 1
        cancelIdleUnloadTask()
    }

    private func endActiveInference() {
        activeInferenceCount = max(0, activeInferenceCount - 1)
        guard activeInferenceCount == 0 else { return }
        resumeActiveInferenceWaiters()
        Memory.clearCache()
        scheduleIdleUnloadIfNeeded()
    }

    func shutdownForApplicationTermination() async {
        guard !isShuttingDownForApplicationTermination else {
            await waitForActiveInferencesToFinish()
            return
        }
        isShuttingDownForApplicationTermination = true
        installationCache.invalidateAll()

        let downloadTasks = Array(downloadTasksByRepo.values)
        for repo in Array(downloadTasksByRepo.keys) {
            pauseDownload(repo: repo)
        }
        let pendingSizeTask = sizeTask
        sizeTask?.cancel()
        sizeTask = nil
        let pendingPrefetchTask = prefetchTask
        prefetchTask?.cancel()
        prefetchTask = nil
        let pendingInferenceLoadTasks = inferenceLoadCoordinator.cancelAll()
        cancelIdleUnloadTask()

        for task in downloadTasks {
            await task.value
        }
        await pendingSizeTask?.value
        await pendingPrefetchTask?.value
        for task in pendingInferenceLoadTasks {
            await task.waitForCompletion()
        }
        await waitForActiveInferencesToFinish()

        releaseInferenceResources()
        VoxtLog.modelInfo("Custom LLM model released for application termination.", verbose: true)
    }

    private func waitForActiveInferencesToFinish() async {
        guard activeInferenceCount > 0 else { return }
        await withCheckedContinuation { continuation in
            activeInferenceWaiters.append(continuation)
        }
    }

    private func resumeActiveInferenceWaiters() {
        let waiters = activeInferenceWaiters
        activeInferenceWaiters.removeAll()
        for waiter in waiters {
            waiter.resume()
        }
    }

    private func scheduleIdleUnloadIfNeeded() {
        guard inferenceContainer != nil else { return }
        idleUnloadTask?.cancel()
        let expectedRepo = inferenceModelRepo
        let delay = resolvedIdleUnloadDelay
        idleUnloadTask = Task { [weak self] in
            do {
                try await Task.sleep(for: delay)
            } catch {
                return
            }
            guard let self else { return }
            await MainActor.run {
                self.unloadInferenceContainerIfIdle(expectedRepo: expectedRepo, reason: "idle-timeout")
            }
        }
    }

    private func cancelIdleUnloadTask() {
        idleUnloadTask?.cancel()
        idleUnloadTask = nil
    }

    private func releaseInferenceResources() {
        cancelIdleUnloadTask()
        inferenceLoadCoordinator.cancelAll()
        inferenceContainer = nil
        inferenceModelRepo = nil
        Memory.clearCache()
    }

    private func unloadInferenceContainerIfIdle(expectedRepo: String?, reason: String) {
        guard activeInferenceCount == 0 else { return }
        guard inferenceContainer != nil, inferenceModelRepo == expectedRepo else { return }

        releaseInferenceResources()
        VoxtLog.modelInfo(
            "Custom LLM model released. repo=\(expectedRepo ?? "unknown"), reason=\(reason)"
        )
    }
}
