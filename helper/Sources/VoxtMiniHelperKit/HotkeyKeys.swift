import Foundation

// The keys and flags the hotkey recognizer reads. The tap's path works on
// bit sets (ModKeys, KeyBits) rather than Set and Dictionary: an event that
// fires nothing allocates nothing, and touches a few words of memory
// (docs/helper-protocol.md, "The tap's cost").

/// One modifier key, with its side. fn has none.
public enum ModKey: String, CaseIterable, Codable, Sendable {
    case fn
    case leftShift, rightShift
    case leftControl, rightControl
    case leftOption, rightOption
    case leftCommand, rightCommand

    public var family: ModFamily {
        switch self {
        case .fn: .fn
        case .leftShift, .rightShift: .shift
        case .leftControl, .rightControl: .control
        case .leftOption, .rightOption: .option
        case .leftCommand, .rightCommand: .command
        }
    }

    public var isRight: Bool {
        switch self {
        case .rightShift, .rightControl, .rightOption, .rightCommand: true
        default: false
        }
    }

    /// Its bit in ModKeys.
    var bit: UInt16 {
        switch self {
        case .fn: 1 << 0
        case .leftShift: 1 << 1
        case .rightShift: 1 << 2
        case .leftControl: 1 << 3
        case .rightControl: 1 << 4
        case .leftOption: 1 << 5
        case .rightOption: 1 << 6
        case .leftCommand: 1 << 7
        case .rightCommand: 1 << 8
        }
    }

    /// The key a flagsChanged event's keycode names (kVK_*).
    public static func forKeyCode(_ k: UInt16) -> ModKey? {
        switch k {
        case 63: .fn
        case 56: .leftShift
        case 60: .rightShift
        case 59: .leftControl
        case 62: .rightControl
        case 58: .leftOption
        case 61: .rightOption
        case 55: .leftCommand
        case 54: .rightCommand
        default: nil
        }
    }
}

public enum ModFamily: String, CaseIterable, Codable, CodingKeyRepresentable, Sendable {
    case fn, shift, control, option, command

    /// Both sides' bits in ModKeys (fn: its one).
    var keys: ModKeys {
        switch self {
        case .fn: ModKeys(rawValue: 0b1)
        case .shift: ModKeys(rawValue: 0b110)
        case .control: ModKeys(rawValue: 0b1_1000)
        case .option: ModKeys(rawValue: 0b110_0000)
        case .command: ModKeys(rawValue: 0b1_1000_0000)
        }
    }

    /// The family's CGEventFlags bit (none for fn: key bindings ignore it).
    var flag: UInt64 {
        switch self {
        case .fn: 0
        case .shift: RawFlags.shift
        case .control: RawFlags.control
        case .option: RawFlags.option
        case .command: RawFlags.command
        }
    }
}

/// A set of modifier keys as bits: the recognizer's chord state.
public struct ModKeys: Equatable, Hashable, Sendable {
    public var rawValue: UInt16
    public init(rawValue: UInt16) { self.rawValue = rawValue }
    public init(_ keys: Set<ModKey>) { rawValue = keys.reduce(0) { $0 | $1.bit } }

    public static let empty = ModKeys(rawValue: 0)
    /// The right-hand keys (fn has no side: it is never one).
    static let right = ModKeys(rawValue: ModKey.rightShift.bit | ModKey.rightControl.bit | ModKey.rightOption.bit | ModKey.rightCommand.bit)
    static let all = ModKeys(rawValue: 0b1_1111_1111)

    public var isEmpty: Bool { rawValue == 0 }
    public func contains(_ k: ModKey) -> Bool { rawValue & k.bit != 0 }
    mutating func insert(_ k: ModKey) { rawValue |= k.bit }
    mutating func remove(_ k: ModKey) { rawValue &= ~k.bit }
    func union(_ o: ModKeys) -> ModKeys { ModKeys(rawValue: rawValue | o.rawValue) }
    func intersection(_ o: ModKeys) -> ModKeys { ModKeys(rawValue: rawValue & o.rawValue) }
    func subtracting(_ o: ModKeys) -> ModKeys { ModKeys(rawValue: rawValue & ~o.rawValue) }

    /// As a Set (tests, logs; allocates).
    public var keys: Set<ModKey> { Set(ModKey.allCases.filter(contains)) }
}

/// Raw CGEventFlags bits the recognizer reads (IOKit's NX_* device masks).
public enum RawFlags {
    public static let secondaryFn: UInt64 = 0x80_0000
    public static let shift: UInt64 = 0x2_0000
    public static let control: UInt64 = 0x4_0000
    public static let option: UInt64 = 0x8_0000
    public static let command: UInt64 = 0x10_0000
    /// The four families a key binding compares (fn aside).
    static let families: UInt64 = shift | control | option | command
    static let deviceLeftControl: UInt64 = 0x0001
    static let deviceLeftShift: UInt64 = 0x0002
    static let deviceRightShift: UInt64 = 0x0004
    static let deviceLeftCommand: UInt64 = 0x0008
    static let deviceRightCommand: UInt64 = 0x0010
    static let deviceLeftOption: UInt64 = 0x0020
    static let deviceRightOption: UInt64 = 0x0040
    static let deviceRightControl: UInt64 = 0x2000

    /// The modifier keys down according to flags. When a family's flag is set
    /// but no device bit says which side (synthetic events), the left key.
    public static func pressedKeys(_ flags: UInt64) -> ModKeys {
        var s = ModKeys.empty
        if flags & secondaryFn != 0 { s.insert(.fn) }
        side(&s, flags, shift, deviceLeftShift, deviceRightShift, .leftShift, .rightShift)
        side(&s, flags, control, deviceLeftControl, deviceRightControl, .leftControl, .rightControl)
        side(&s, flags, option, deviceLeftOption, deviceRightOption, .leftOption, .rightOption)
        side(&s, flags, command, deviceLeftCommand, deviceRightCommand, .leftCommand, .rightCommand)
        return s
    }

    @inline(__always)
    private static func side(_ s: inout ModKeys, _ flags: UInt64, _ family: UInt64, _ l: UInt64, _ r: UInt64, _ lk: ModKey, _ rk: ModKey) {
        let hasL = flags & l != 0, hasR = flags & r != 0
        if hasL { s.insert(lk) }
        if hasR { s.insert(rk) }
        if flags & family != 0, !hasL, !hasR { s.insert(lk) }
    }

    /// pressedKeys as a Set.
    public static func pressed(_ flags: UInt64) -> Set<ModKey> { pressedKeys(flags).keys }

    /// The families of a key event's flags (fn aside), as a Set.
    public static func families(_ flags: UInt64) -> Set<ModFamily> {
        Set(ModFamily.allCases.filter { $0 != .fn && flags & $0.flag != 0 })
    }
}

/// Virtual key codes the recognizer knows (Carbon kVK_*).
public enum KeyCode {
    public static let function: UInt16 = 63
    public static let escape: UInt16 = 53
    public static let space: UInt16 = 49
    public static let capsLock: UInt16 = 57
    /// The 🌐 key's key down/up after fn (seen on macOS 26 with an Apple keyboard).
    public static let globe: UInt16 = 179
    public static let modifiers: Set<UInt16> = [54, 55, 56, 57, 58, 59, 60, 61, 62, 63]
}

/// A set of key codes below 256 (every kVK code) as bits, inline: the keys
/// whose down was swallowed. Codes from 256 on (none known) go in a Set.
struct KeyBits: Equatable, Sendable {
    private var w0: UInt64 = 0, w1: UInt64 = 0, w2: UInt64 = 0, w3: UInt64 = 0
    private var high: Set<UInt16> = []

    var isEmpty: Bool { w0 | w1 | w2 | w3 == 0 && high.isEmpty }

    func contains(_ k: UInt16) -> Bool {
        let b = UInt64(1) << UInt64(k & 63)
        switch k >> 6 {
        case 0: return w0 & b != 0
        case 1: return w1 & b != 0
        case 2: return w2 & b != 0
        case 3: return w3 & b != 0
        default: return !high.isEmpty && high.contains(k)
        }
    }

    mutating func insert(_ k: UInt16) {
        let b = UInt64(1) << UInt64(k & 63)
        switch k >> 6 {
        case 0: w0 |= b
        case 1: w1 |= b
        case 2: w2 |= b
        case 3: w3 |= b
        default: high.insert(k)
        }
    }

    /// Removes k; whether it was there.
    mutating func remove(_ k: UInt16) -> Bool {
        guard contains(k) else { return false }
        let b = ~(UInt64(1) << UInt64(k & 63))
        switch k >> 6 {
        case 0: w0 &= b
        case 1: w1 &= b
        case 2: w2 &= b
        case 3: w3 &= b
        default: high.remove(k)
        }
        return true
    }
}
