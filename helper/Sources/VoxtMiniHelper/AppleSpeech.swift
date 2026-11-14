@preconcurrency import AVFoundation
import Foundation
@preconcurrency import Speech
import VoxtMiniHelperKit

/// A speech recogniser fed the app's audio (mono PCM16 at 16 kHz).
protocol SpeechEngine: AnyObject, Sendable {
    var name: String { get }
    var locale: String { get }
    func append(_ pcm: Data) throws
    /// No more audio: the final text.
    func finish() async -> String
    func cancel() async
}

/// The 16 kHz mono float format the app's audio becomes.
let appleInputFormat = AVAudioFormat(commonFormat: .pcmFormatFloat32, sampleRate: 16000, channels: 1, interleaved: false)!

/// pcm as a buffer of appleInputFormat.
func floatBuffer(_ pcm: Data) -> AVAudioPCMBuffer? {
    let samples = PCM16.floats(pcm)
    guard !samples.isEmpty, let buf = AVAudioPCMBuffer(pcmFormat: appleInputFormat, frameCapacity: AVAudioFrameCount(samples.count)),
          let dst = buf.floatChannelData?[0] else { return nil }
    buf.frameLength = AVAudioFrameCount(samples.count)
    samples.withUnsafeBufferPointer { src in dst.update(from: src.baseAddress!, count: samples.count) }
    return buf
}

/// Which engine runs a locale, the locales' states, Apple's downloads.
/// macOS 26's SpeechAnalyzer where it knows the language, else
/// SFSpeechRecognizer on-device (never Apple's servers). The macOS 26
/// parts build with its SDK only (canImport(FoundationModels) stands for
/// it).
enum SpeechEngines {
    static func status(_ locales: [String]) async -> AppleSpeechStatus {
        #if canImport(FoundationModels)
        if #available(macOS 26.0, *), SpeechTranscriber.isAvailable {
            var out: [AppleLocale] = []
            for l in locales {
                var st = await AnalyzerEngine.status(l)
                if st == LocaleStatus.unsupported, RecognizerEngine.supports(l) {
                    st = LocaleStatus.installed // the older recogniser has it
                }
                out.append(AppleLocale(locale: l, status: st))
            }
            return AppleSpeechStatus(engine: AppleEngine.analyzer, locales: out)
        }
        #endif
        let out = locales.map { AppleLocale(locale: $0, status: RecognizerEngine.supports($0) ? LocaleStatus.installed : LocaleStatus.unsupported) }
        return AppleSpeechStatus(engine: AppleEngine.recognizer, locales: out)
    }

    static func make(locale: String, hints: [String], onPartial: @escaping @Sendable (String) -> Void) async throws -> SpeechEngine {
        #if canImport(FoundationModels)
        if #available(macOS 26.0, *), SpeechTranscriber.isAvailable, await AnalyzerEngine.status(locale) == LocaleStatus.installed {
            return try await AnalyzerEngine.start(locale: locale, hints: hints, onPartial: onPartial)
        }
        #endif
        return try RecognizerEngine(locale: locale, hints: hints, onPartial: onPartial)
    }

    /// Downloads the locales Apple can (SpeechAnalyzer's assets); the
    /// recogniser's languages come with macOS's dictation.
    static func install(_ locales: [String], progress: @escaping @Sendable (Double) -> Void) async throws -> AppleInstallResult {
        #if canImport(FoundationModels)
        if #available(macOS 26.0, *), SpeechTranscriber.isAvailable {
            for (i, l) in locales.enumerated() {
                try await AnalyzerEngine.install(l) { f in progress((Double(i) + f) / Double(locales.count)) }
            }
        }
        #endif
        let st = await status(locales)
        return AppleInstallResult(locales: st.locales)
    }
}

// MARK: - SFSpeechRecognizer (macOS 15+)

/// SFSpeechRecognizer on-device only, as Voxt's direct dictation without
/// its server mode: partial results, punctuation, the dictionary's terms
/// as contextual strings.
final class RecognizerEngine: SpeechEngine, @unchecked Sendable {
    let name = AppleEngine.recognizer
    let locale: String
    private let request = SFSpeechAudioBufferRecognitionRequest()
    private var task: SFSpeechRecognitionTask?
    private let lock = NSLock()
    private var text = SpeechText()
    private var done = false
    private var lastResult = monotonicMs()

    static func supports(_ locale: String) -> Bool {
        guard let r = SFSpeechRecognizer(locale: Locale(identifier: locale)) else { return false }
        return r.supportsOnDeviceRecognition
    }

    init(locale: String, hints: [String], onPartial: @escaping @Sendable (String) -> Void) throws {
        self.locale = locale
        guard let r = SFSpeechRecognizer(locale: Locale(identifier: locale)), r.supportsOnDeviceRecognition else {
            throw RPCError.unsupported(Capability.locale, "Speech recognition does not support \(locale) on this Mac")
        }
        guard r.isAvailable else {
            throw RPCError(Code.busy, "Speech recognition is not available right now")
        }
        let queue = OperationQueue()
        queue.maxConcurrentOperationCount = 1
        r.queue = queue
        request.requiresOnDeviceRecognition = true
        request.shouldReportPartialResults = true
        request.taskHint = .dictation
        request.addsPunctuation = true
        request.contextualStrings = hints
        task = r.recognitionTask(with: request) { [weak self] result, error in
            guard let self else { return }
            if let result {
                let s = result.bestTranscription.formattedString
                lock.withLock {
                    text.whole(s)
                    lastResult = monotonicMs()
                    if result.isFinal { done = true }
                }
                onPartial(s)
            }
            if let error {
                let e = error as NSError
                lock.withLock {
                    if e.domain == "kAFAssistantErrorDomain", e.code == 1110 { text.whole("") } // no speech
                    done = true
                }
                if !(e.domain == "kAFAssistantErrorDomain" && [216, 1110].contains(e.code)) {
                    log("speech recognition (\(locale)): \(e.domain) \(e.code) \(e.localizedDescription)")
                }
            }
        }
    }

    func append(_ pcm: Data) throws {
        if let buf = floatBuffer(pcm) { request.append(buf) }
    }

    /// The final result, or the last one 900 ms after it came (Voxt's
    /// wait), at most 10 s.
    func finish() async -> String {
        request.endAudio()
        let start = monotonicMs()
        while true {
            let (isDone, last, t) = lock.withLock { (done, lastResult, text.text) }
            let now = monotonicMs()
            if isDone || now - max(last, start) > 900 || now - start > 10_000 {
                task?.cancel()
                return t
            }
            try? await Task.sleep(nanoseconds: 50_000_000)
        }
    }

    func cancel() async {
        request.endAudio()
        task?.cancel()
    }
}

// MARK: - SpeechAnalyzer (macOS 26)

#if canImport(FoundationModels)
/// SpeechAnalyzer with SpeechTranscriber: volatile results as the live
/// text, the dictionary's terms in its context.
@available(macOS 26.0, *)
final class AnalyzerEngine: SpeechEngine, @unchecked Sendable {
    let name = AppleEngine.analyzer
    let locale: String
    private let analyzer: SpeechAnalyzer
    private let input: AsyncStream<AnalyzerInput>.Continuation
    private let converter: AVAudioConverter?
    private let format: AVAudioFormat
    private var results: Task<Void, Never>?
    private let lock = NSLock()
    private var text = SpeechText()

    private init(locale: String, analyzer: SpeechAnalyzer, input: AsyncStream<AnalyzerInput>.Continuation, format: AVAudioFormat) {
        self.locale = locale
        self.analyzer = analyzer
        self.input = input
        self.format = format
        converter = format == appleInputFormat ? nil : AVAudioConverter(from: appleInputFormat, to: format)
    }

    /// The transcriber of locale's equivalent, nil when it has none.
    static func transcriber(_ locale: String) async -> SpeechTranscriber? {
        guard let loc = await SpeechTranscriber.supportedLocale(equivalentTo: Locale(identifier: locale)) else { return nil }
        return SpeechTranscriber(locale: loc, transcriptionOptions: [], reportingOptions: [.volatileResults], attributeOptions: [])
    }

    static func status(_ locale: String) async -> String {
        guard let t = await transcriber(locale) else { return LocaleStatus.unsupported }
        switch await AssetInventory.status(forModules: [t]) {
        case .installed: return LocaleStatus.installed
        case .downloading: return LocaleStatus.downloading
        case .supported: return LocaleStatus.available
        default: return LocaleStatus.unsupported
        }
    }

    static func install(_ locale: String, progress: @escaping @Sendable (Double) -> Void) async throws {
        guard let t = await transcriber(locale) else { return }
        guard let request = try await AssetInventory.assetInstallationRequest(supporting: [t]) else { return } // installed
        let watch = Task {
            while !Task.isCancelled {
                progress(request.progress.fractionCompleted)
                try? await Task.sleep(nanoseconds: 250_000_000)
            }
        }
        defer { watch.cancel() }
        try await request.downloadAndInstall()
        progress(1)
    }

    static func start(locale: String, hints: [String], onPartial: @escaping @Sendable (String) -> Void) async throws -> AnalyzerEngine {
        guard let t = await transcriber(locale) else {
            throw RPCError.unsupported(Capability.locale, "Speech recognition does not support \(locale) on this Mac")
        }
        let analyzer = SpeechAnalyzer(modules: [t])
        if !hints.isEmpty {
            let context = AnalysisContext()
            context.contextualStrings[.general] = hints
            try await analyzer.setContext(context)
        }
        let format = await SpeechAnalyzer.bestAvailableAudioFormat(compatibleWith: [t]) ?? appleInputFormat
        let (stream, input) = AsyncStream<AnalyzerInput>.makeStream()
        let e = AnalyzerEngine(locale: locale, analyzer: analyzer, input: input, format: format)
        e.results = Task {
            do {
                for try await r in t.results {
                    let s = String(r.text.characters)
                    let all = e.lock.withLock { () -> String in
                        if r.isFinal { e.text.final(s) } else { e.text.partial(s) }
                        return e.text.text
                    }
                    onPartial(all)
                }
            } catch {
                log("speech analyzer (\(locale)): \(errorMessage(error))")
            }
        }
        try await analyzer.start(inputSequence: stream)
        return e
    }

    func append(_ pcm: Data) throws {
        guard let buf = floatBuffer(pcm) else { return }
        guard let converter else {
            input.yield(AnalyzerInput(buffer: buf))
            return
        }
        let ratio = format.sampleRate / appleInputFormat.sampleRate
        guard let outBuf = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: AVAudioFrameCount(Double(buf.frameLength) * ratio) + 32) else { return }
        var fed = false
        var err: NSError?
        _ = converter.convert(to: outBuf, error: &err) { _, status in
            if fed {
                status.pointee = .noDataNow
                return nil
            }
            fed = true
            status.pointee = .haveData
            return buf
        }
        if let err { throw RPCError(Code.internal, "Audio conversion: \(err.localizedDescription)") }
        input.yield(AnalyzerInput(buffer: outBuf))
    }

    func finish() async -> String {
        input.finish()
        do {
            try await analyzer.finalizeAndFinishThroughEndOfInput()
        } catch {
            log("speech analyzer (\(locale)) finish: \(errorMessage(error))")
        }
        await results?.value
        return lock.withLock { text.text }
    }

    func cancel() async {
        input.finish()
        await analyzer.cancelAndFinishNow()
        results?.cancel()
    }
}
#endif
