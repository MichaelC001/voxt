import Foundation

// The pure halves of the helper's Apple models (docs/apple-models.md):
// the text a recogniser's results make, PCM16 to floats, and a chat's
// messages as instructions, turns and a prompt.

/// What an apple.speech job's text is, from the recogniser's results: the
/// finalized parts and the volatile tail (SpeechAnalyzer), or the whole
/// hypothesis (SFSpeechRecognizer).
public struct SpeechText: Sendable {
    public private(set) var finalized = ""
    public private(set) var volatile = ""

    public init() {}

    /// The whole text so far.
    public var text: String { SpeechText.join(finalized, volatile) }

    /// A final result of a range (SpeechAnalyzer): appended; the volatile
    /// text it replaces goes.
    public mutating func final(_ s: String) {
        finalized = SpeechText.join(finalized, s)
        volatile = ""
    }

    /// A volatile result: it replaces the previous one.
    public mutating func partial(_ s: String) { volatile = s }

    /// The recogniser's whole hypothesis (SFSpeechRecognizer).
    public mutating func whole(_ s: String) {
        finalized = ""
        volatile = s
    }

    /// Joins two pieces: a space between Latin words, none next to CJK
    /// text (the recognisers give each piece without its separator).
    static func join(_ a: String, _ b: String) -> String {
        let a = a.trimmingCharacters(in: .whitespaces), b = b.trimmingCharacters(in: .whitespaces)
        if a.isEmpty { return b }
        if b.isEmpty { return a }
        if let x = a.unicodeScalars.last, let y = b.unicodeScalars.first, isCJK(x) || isCJK(y) {
            return a + b
        }
        return a + " " + b
    }

    static func isCJK(_ u: Unicode.Scalar) -> Bool {
        switch u.value {
        case 0x3000...0x30FF, 0x3400...0x4DBF, 0x4E00...0x9FFF, 0xF900...0xFAFF, 0xFF00...0xFFEF: true
        default: false
        }
    }
}

public enum PCM16 {
    /// Mono PCM16 little endian as floats in [-1, 1) (an odd last byte is
    /// dropped).
    public static func floats(_ data: Data) -> [Float] {
        let n = data.count / 2
        var out = [Float](repeating: 0, count: n)
        data.withUnsafeBytes { raw in
            for i in 0..<n {
                out[i] = Float(Int16(littleEndian: raw.loadUnaligned(fromByteOffset: 2 * i, as: Int16.self))) / 32768
            }
        }
        return out
    }
}

/// An apple.chat's messages as FoundationModels takes them: the system
/// messages as instructions, the turns before the last user message, and
/// that message as the prompt.
public struct ChatPlan: Sendable, Equatable {
    public var instructions: String
    public var turns: [AppleMessage]
    public var prompt: String

    public init(_ messages: [AppleMessage]) throws {
        instructions = messages.filter { $0.role == "system" }.map(\.content).joined(separator: "\n\n")
        let rest = messages.filter { $0.role != "system" }
        guard let last = rest.lastIndex(where: { $0.role == "user" }) else {
            throw RPCError(Code.badRequest, "No user message")
        }
        for m in rest where m.role != "user" && m.role != "assistant" {
            throw RPCError(Code.badRequest, "Unknown role \(m.role)")
        }
        turns = Array(rest[..<last])
        prompt = rest[last].content
    }
}
