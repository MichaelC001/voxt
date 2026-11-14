import Foundation

// The hotkey recognizer's types: bindings, its config, what it reads and
// what it reports (HotkeyRecognizer.swift).

public enum Behavior: String, Codable, Sendable {
    case tap, doubleTap, longPress
}

/// What one modifier family must be in a chord.
public enum SideRule: String, Codable, Sendable {
    case any, left, right
}

/// A binding: a modifier-only chord (key and mouseButton nil), a key with
/// modifiers, or a mouse button with modifiers.
public struct Binding: Codable, Equatable, Sendable {
    public var id: String
    /// Modifier families required, each with a side rule; others must be up.
    public var modifiers: [ModFamily: SideRule]
    /// The non-modifier key (kVK), or nil for a modifier-only chord.
    public var key: UInt16?
    /// A mouse button (CGMouseButton: 2 the middle one, 3 and 4 the side
    /// ones) instead of a key; its presses come from the button tap.
    public var mouseButton: Int?
    public var behavior: Behavior
    /// Swallow the key's (or button's) events when it matches (key and
    /// mouse bindings only; the default is true). Modifier-only chords are never swallowed except fn
    /// with Config.swallowFn.
    public var swallow: Bool?

    public init(id: String, modifiers: [ModFamily: SideRule], key: UInt16? = nil, mouseButton: Int? = nil, behavior: Behavior, swallow: Bool? = nil) {
        self.id = id
        self.modifiers = modifiers
        self.key = key
        self.mouseButton = mouseButton
        self.behavior = behavior
        self.swallow = swallow
    }

    /// Whether a chord (modifier keys seen) is exactly this binding's.
    public func matches(chord: Set<ModKey>) -> Bool { CompiledBinding(self).matches(ModKeys(chord)) }

    /// Whether the modifier families of a key event are exactly this binding's.
    public func matches(families: Set<ModFamily>) -> Bool {
        Set(modifiers.keys.filter { $0 != .fn }) == families
    }

    /// More modifiers is more specific (fn+⇧ before fn).
    var specificity: Int { modifiers.count * 2 + (modifiers.values.contains { $0 != .any } ? 1 : 0) }
}

public struct HotkeyConfig: Codable, Equatable, Sendable {
    public var bindings: [Binding]
    /// A second press starting within this long of the first release makes
    /// a double tap (judged at its release; real double taps on mac-air had
    /// 196–450 ms between releases, so the release alone is too strict).
    public var doubleTapMs: Double
    /// Held this long, a longPress binding fires down.
    public var longPressMs: Double
    /// A modifier-only press held longer than this is no tap.
    public var tapMaxMs: Double
    /// Swallow fn's own flagsChanged events while an fn chord is bound, so
    /// the 🌐 key's system action (emoji, input source) does not run.
    public var swallowFn: Bool
    /// While a session records, a tap fires at once (no wait for a double
    /// tap) and Escape is reported (and swallowed).
    public var active: Bool
    /// While active, Escape is neither reported nor swallowed (the user
    /// turned "Escape cancels" off).
    public var passEscape: Bool
    /// The Space window: turned on, the next bare Space fires "space" and
    /// is swallowed, once; any other key down or a mouse press closes it
    /// (HotkeyRecognizer.spaceOpen).
    public var spaceFollowUp: Bool

    public init(bindings: [Binding], doubleTapMs: Double = 500, longPressMs: Double = 300, tapMaxMs: Double = 1500,
                swallowFn: Bool = false, active: Bool = false, passEscape: Bool = false, spaceFollowUp: Bool = false) {
        self.bindings = bindings
        self.doubleTapMs = doubleTapMs
        self.longPressMs = longPressMs
        self.tapMaxMs = tapMaxMs
        self.swallowFn = swallowFn
        self.active = active
        self.passEscape = passEscape
        self.spaceFollowUp = spaceFollowUp
    }
}

public enum Phase: String, Codable, Sendable {
    /// tap or doubleTap fired; a key binding's down for tap.
    case trigger
    /// longPress: held long enough.
    case down
    /// longPress: released after down.
    case up
}

public struct HotkeyEvent: Codable, Equatable, Sendable {
    public var binding: String
    public var phase: Phase
    public var behavior: Behavior
    /// Milliseconds the chord was held (release − press), when known.
    public var heldMs: Double?
    /// Timestamp of the event that decided it (ms, monotonic).
    public var t: Double
}

/// A keyboard event, as the tap sees it.
public enum KeyInput: Sendable {
    case flagsChanged(keyCode: UInt16, flags: UInt64, t: Double)
    case keyDown(keyCode: UInt16, flags: UInt64, isRepeat: Bool, t: Double)
    case keyUp(keyCode: UInt16, flags: UInt64, t: Double)
    /// A mouse button pressed (from the listen-only tap): it only spoils a
    /// chord.
    case pointer(t: Double)
    /// A middle or side button pressed or released, from the button tap
    /// (active: the answer swallows it), installed while one is bound.
    case mouseButton(button: Int, down: Bool, flags: UInt64, t: Double)

    var t: Double {
        switch self {
        case .flagsChanged(_, _, let t), .keyDown(_, _, _, let t), .keyUp(_, _, let t), .pointer(let t),
             .mouseButton(_, _, _, let t): t
        }
    }
}

