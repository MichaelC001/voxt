import Foundation
@preconcurrency import Speech
import VoxtMiniHelperKit

/// Apple's on-device models (apple.*, docs/apple-models.md): speech
/// recognition and Apple Intelligence. The jobs (apple.install,
/// apple.speech, apple.chat) run in tasks of their own, kept here so that
/// job.cancel can end them; a speech job's audio comes in
/// apple.speech.audio requests, which the server hands over on its reader
/// thread, in order.
final class AppleModels: @unchecked Sendable {
    let out: Writer
    private let lock = NSLock()
    private var tasks: [String: Task<Void, Never>] = [:]
    private var speeches: [String: SpeechJob] = [:]

    init(out: Writer) { self.out = out }

    // MARK: status

    func status(_ p: AppleStatusParams) async -> AppleStatus {
        AppleStatus(speech: await SpeechEngines.status(p.locales), intelligence: AppleChat.status())
    }

    // MARK: jobs

    /// Starts job id of method (apple.install, apple.speech, apple.chat):
    /// it answers when it ends.
    func start(job id: String, _ method: String, _ msg: Incoming) {
        let task = Task { [self] in
            do {
                let result: any Encodable & Sendable
                switch method {
                case Method.appleInstall:
                    result = try await SpeechEngines.install(try msg.decodeParams(AppleInstallParams.self).locales) { [out] fraction in
                        out.notify(Method.appleProgress, AppleProgressParams(job: id, fraction: fraction))
                    }
                case Method.appleSpeech:
                    result = try await speech(job: id, try msg.decodeParams(AppleSpeechParams.self))
                default:
                    result = try await AppleChat.run(job: id, try msg.decodeParams(AppleChatParams.self), out: out)
                }
                out.result(id, result)
            } catch {
                out.fail(id, rpcError(Task.isCancelled ? CancellationError() : error))
            }
            lock.withLock { tasks[id] = nil }
        }
        lock.withLock { tasks[id] = task }
    }

    /// job.cancel: ends job id, answering after it (false: no such job).
    func cancel(job id: String) async -> Bool {
        guard let task = lock.withLock({ tasks[id] }) else { return false }
        task.cancel()
        lock.withLock { speeches[id] }?.cancel()
        await task.value
        return true
    }

    /// Ends every job (shutdown, stdin closed).
    func cancelAll() {
        let all = lock.withLock { Array(tasks.values) }
        all.forEach { $0.cancel() }
        lock.withLock { Array(speeches.values) }.forEach { $0.cancel() }
    }

    // MARK: apple.speech

    private func speech(job id: String, _ p: AppleSpeechParams) async throws -> AppleSpeechResult {
        try await SpeechPermission.ensure()
        let job = SpeechJob()
        let engine = try await SpeechEngines.make(locale: p.locale, hints: p.hints ?? []) { [out] text in
            out.notify(Method.applePartial, ApplePartialParams(job: id, text: text))
        }
        job.engine = engine
        lock.withLock { speeches[id] = job }
        defer { lock.withLock { speeches[id] = nil } }
        out.notify(Method.appleStarted, AppleSpeechStartedParams(job: id, locale: engine.locale, engine: engine.name))
        let finished = await withTaskCancellationHandler { await job.waitForFinish() } onCancel: { job.cancel() }
        guard finished, !Task.isCancelled else {
            await engine.cancel()
            throw CancellationError()
        }
        let text = await engine.finish()
        return AppleSpeechResult(text: text, locale: engine.locale, engine: engine.name, audioMs: job.audioMs)
    }

    /// apple.speech.audio and apple.speech.finish, on the reader thread.
    func feed(_ method: String, _ msg: Incoming) throws {
        if method == Method.appleSpeechAudio {
            let p = try msg.decodeParams(AppleAudioParams.self)
            guard let job = lock.withLock({ speeches[p.job] }) else {
                throw RPCError(Code.badRequest, "No apple.speech job \(p.job)")
            }
            try job.append(p.data)
            return
        }
        let p = try msg.decodeParams(AppleJobParams.self)
        guard let job = lock.withLock({ speeches[p.job] }), job.finish() else {
            throw RPCError(Code.badRequest, "No apple.speech job \(p.job)")
        }
    }
}

/// One apple.speech job's state: its engine, the audio it took, and the
/// finish the app asked for.
final class SpeechJob: @unchecked Sendable {
    private let lock = NSLock()
    var engine: SpeechEngine?
    private var samples = 0
    private var finished = false
    private var cancelled = false
    private var waiter: CheckedContinuation<Bool, Never>?

    var audioMs: Int { lock.withLock { samples / 16 } }

    func append(_ pcm: Data) throws {
        let done = lock.withLock { finished }
        guard !done, let engine else { throw RPCError(Code.badRequest, "The audio has ended") }
        lock.withLock { samples += pcm.count / 2 }
        try engine.append(pcm)
    }

    /// The app's apple.speech.finish (false: it came already).
    func finish() -> Bool {
        let w: CheckedContinuation<Bool, Never>?
        lock.lock()
        if finished {
            lock.unlock()
            return false
        }
        finished = true
        w = waiter
        waiter = nil
        lock.unlock()
        w?.resume(returning: true)
        return true
    }

    /// Ends the wait without a finish (job.cancel).
    func cancel() {
        let w = lock.withLock { () -> CheckedContinuation<Bool, Never>? in
            cancelled = true
            let w = waiter
            waiter = nil
            return w
        }
        w?.resume(returning: false)
    }

    /// Waits for finish (true) or cancel (false).
    func waitForFinish() async -> Bool {
        await withCheckedContinuation { c in
            lock.lock()
            if finished || cancelled {
                lock.unlock()
                c.resume(returning: finished && !cancelled)
                return
            }
            waiter = c
            lock.unlock()
        }
    }
}

/// Speech Recognition, the permission Apple Speech needs.
enum SpeechPermission {
    static func status() -> String {
        switch SFSpeechRecognizer.authorizationStatus() {
        case .authorized: "granted"
        case .denied: "denied"
        case .restricted: "restricted"
        case .notDetermined: "notDetermined"
        @unknown default: "denied"
        }
    }

    /// Shows macOS's prompt (once: then only System Settings changes it).
    static func request() async -> String {
        if SFSpeechRecognizer.authorizationStatus() == .notDetermined {
            await withCheckedContinuation { (c: CheckedContinuation<Void, Never>) in
                SFSpeechRecognizer.requestAuthorization { _ in c.resume() }
            }
        }
        return status()
    }

    /// Asks when not asked yet; throws permission_denied unless granted.
    static func ensure() async throws {
        guard await request() == "granted" else {
            throw RPCError.denied(PermissionName.speechRecognition, "Speech recognition is not allowed")
        }
    }
}
