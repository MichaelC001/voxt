import Foundation
import VoxtMiniHelperKit
#if canImport(FoundationModels)
import FoundationModels
#endif

/// Apple Intelligence (FoundationModels, macOS 26): its availability and
/// one streamed answer per apple.chat, in a fresh session (Voxt's
/// TextEnhancer, without its @Generable wrapper: plain text, streamed).
/// Built without the macOS 26 SDK, it is unavailable (unsupportedOS).
enum AppleChat {
    static func status() -> AppleIntelligenceStatus {
        #if canImport(FoundationModels)
        if #available(macOS 26.0, *) {
            let model = SystemLanguageModel.default
            switch model.availability {
            case .available:
                let langs = Set(model.supportedLanguages.compactMap { $0.languageCode?.identifier })
                return AppleIntelligenceStatus(status: "available", languages: langs.sorted())
            case .unavailable(let reason):
                let r: String
                switch reason {
                case .appleIntelligenceNotEnabled: r = AppleReason.notEnabled
                case .deviceNotEligible: r = AppleReason.deviceNotEligible
                case .modelNotReady: r = AppleReason.modelNotReady
                @unknown default: r = AppleReason.other
                }
                return AppleIntelligenceStatus(status: "unavailable", reason: r)
            }
        }
        #endif
        return AppleIntelligenceStatus(status: "unavailable", reason: AppleReason.unsupportedOS)
    }

    static func run(job: String, _ p: AppleChatParams, out: Writer) async throws -> AppleChatResult {
        let plan = try ChatPlan(p.messages)
        #if canImport(FoundationModels)
        if #available(macOS 26.0, *) {
            return try await answer(job: job, plan, p, out: out)
        }
        #endif
        throw RPCError.unsupported(Capability.intelligence, "Apple Intelligence needs macOS 26")
    }

    #if canImport(FoundationModels)
    @available(macOS 26.0, *)
    private static func answer(job: String, _ plan: ChatPlan, _ p: AppleChatParams, out: Writer) async throws -> AppleChatResult {
        let model = SystemLanguageModel.default
        guard case .available = model.availability else {
            throw RPCError.unsupported(Capability.intelligence, "Apple Intelligence is not available (\(status().reason ?? AppleReason.other))")
        }
        let session = LanguageModelSession(model: model, tools: [], transcript: transcript(plan))
        var options = GenerationOptions(temperature: p.temperature, maximumResponseTokens: p.maxTokens)
        if p.temperature == 0 {
            options = GenerationOptions(sampling: .greedy, maximumResponseTokens: p.maxTokens)
        }
        var sent = "", last = "" // the text sent as deltas; the latest snapshot (a revision is not sent)
        do {
            for try await snapshot in session.streamResponse(to: plan.prompt, options: options) {
                last = snapshot.content
                if last.hasPrefix(sent), last.count > sent.count {
                    out.notify(Method.appleDelta, AppleDeltaParams(job: job, text: String(last.dropFirst(sent.count))))
                    sent = last
                }
            }
        } catch let e as LanguageModelSession.GenerationError {
            throw rpcError(e)
        }
        return AppleChatResult(text: last, finish: "stop")
    }

    /// The plan as a transcript: instructions, then the turns before the
    /// prompt (compose's clean-up example).
    @available(macOS 26.0, *)
    private static func transcript(_ plan: ChatPlan) -> Transcript {
        func text(_ s: String) -> [Transcript.Segment] { [.text(Transcript.TextSegment(content: s))] }
        var entries: [Transcript.Entry] = []
        if !plan.instructions.isEmpty {
            entries.append(.instructions(Transcript.Instructions(segments: text(plan.instructions), toolDefinitions: [])))
        }
        for m in plan.turns {
            if m.role == "user" {
                entries.append(.prompt(Transcript.Prompt(segments: text(m.content))))
            } else {
                entries.append(.response(Transcript.Response(assetIDs: [], segments: text(m.content))))
            }
        }
        return Transcript(entries: entries)
    }

    @available(macOS 26.0, *)
    private static func rpcError(_ e: LanguageModelSession.GenerationError) -> RPCError {
        switch e {
        case .exceededContextWindowSize:
            return RPCError(Code.contextLength, "The prompt does not fit Apple Intelligence's context window")
        case .guardrailViolation, .refusal:
            return RPCError(Code.refused, "Apple Intelligence's guardrails refused the request")
        case .unsupportedLanguageOrLocale:
            return RPCError.unsupported(Capability.locale, "Apple Intelligence does not support this language")
        case .assetsUnavailable:
            return RPCError.unsupported(Capability.intelligence, "Apple Intelligence's model is not available")
        case .rateLimited, .concurrentRequests:
            return RPCError(Code.busy, "Apple Intelligence is busy")
        default:
            return RPCError(Code.internal, errorMessage(e))
        }
    }
    #endif
}
