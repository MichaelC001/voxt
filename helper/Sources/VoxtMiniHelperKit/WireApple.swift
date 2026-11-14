import Foundation

// apple.*: internal/helper/proto/apple.go field for field (Apple's
// on-device models, docs/apple-models.md).

public enum AppleEngine {
    public static let analyzer = "analyzer"
    public static let recognizer = "recognizer"
}

public enum LocaleStatus {
    public static let installed = "installed"
    public static let available = "available"
    public static let downloading = "downloading"
    public static let unsupported = "unsupported"
}

public enum AppleReason {
    public static let notEnabled = "notEnabled"
    public static let deviceNotEligible = "deviceNotEligible"
    public static let modelNotReady = "modelNotReady"
    public static let unsupportedOS = "unsupportedOS"
    public static let other = "other"
}

public enum Capability {
    public static let speech = "speech"
    public static let intelligence = "intelligence"
    public static let locale = "locale"
}

public struct AppleStatusParams: Codable, Sendable {
    public var locales: [String]
}

public struct AppleLocale: Codable, Sendable, Equatable {
    public var locale: String
    public var status: String

    public init(locale: String, status: String) {
        self.locale = locale
        self.status = status
    }
}

public struct AppleSpeechStatus: Codable, Sendable {
    public var engine: String?
    public var reason: String?
    public var locales: [AppleLocale]

    public init(engine: String? = nil, reason: String? = nil, locales: [AppleLocale]) {
        self.engine = engine
        self.reason = reason
        self.locales = locales
    }
}

public struct AppleIntelligenceStatus: Codable, Sendable {
    /// available | unavailable
    public var status: String
    public var reason: String?
    public var languages: [String]?

    public init(status: String, reason: String? = nil, languages: [String]? = nil) {
        self.status = status
        self.reason = reason
        self.languages = languages
    }
}

public struct AppleStatus: Codable, Sendable {
    public var speech: AppleSpeechStatus
    public var intelligence: AppleIntelligenceStatus

    public init(speech: AppleSpeechStatus, intelligence: AppleIntelligenceStatus) {
        self.speech = speech
        self.intelligence = intelligence
    }
}

public struct AppleInstallParams: Codable, Sendable {
    public var locales: [String]
}

public struct AppleInstallResult: Codable, Sendable {
    public var locales: [AppleLocale]
    public init(locales: [AppleLocale]) { self.locales = locales }
}

public struct AppleProgressParams: Codable, Sendable {
    public var job: String
    public var fraction: Double

    public init(job: String, fraction: Double) {
        self.job = job
        self.fraction = fraction
    }
}

public struct AppleSpeechParams: Codable, Sendable {
    public var locale: String
    public var hints: [String]?
}

public struct AppleSpeechResult: Codable, Sendable {
    public var text: String
    public var locale: String
    public var engine: String
    public var audioMs: Int

    public init(text: String, locale: String, engine: String, audioMs: Int) {
        self.text = text
        self.locale = locale
        self.engine = engine
        self.audioMs = audioMs
    }
}

public struct AppleSpeechStartedParams: Codable, Sendable {
    public var job: String
    public var locale: String
    public var engine: String

    public init(job: String, locale: String, engine: String) {
        self.job = job
        self.locale = locale
        self.engine = engine
    }
}

public struct ApplePartialParams: Codable, Sendable {
    public var job: String
    public var text: String

    public init(job: String, text: String) {
        self.job = job
        self.text = text
    }
}

public struct AppleAudioParams: Codable, Sendable {
    public var job: String
    /// Mono PCM16 little endian at 16 kHz; base64 on the wire.
    public var data: Data
}

public struct AppleJobParams: Codable, Sendable {
    public var job: String
}

public struct AppleMessage: Codable, Sendable, Equatable {
    /// system | user | assistant
    public var role: String
    public var content: String

    public init(role: String, content: String) {
        self.role = role
        self.content = content
    }
}

public struct AppleChatParams: Codable, Sendable {
    public var messages: [AppleMessage]
    public var maxTokens: Int?
    public var temperature: Double?
}

public struct AppleChatResult: Codable, Sendable {
    public var text: String
    /// stop | length
    public var finish: String

    public init(text: String, finish: String) {
        self.text = text
        self.finish = finish
    }
}

public struct AppleDeltaParams: Codable, Sendable {
    public var job: String
    public var text: String

    public init(job: String, text: String) {
        self.job = job
        self.text = text
    }
}
