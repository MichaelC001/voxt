import Foundation
import Synchronization

// The pure half of the hotkey taps (the CoreGraphics half is the helper's
// Hotkey.swift): which events go to which tap, how the tap's events become
// recognizer input, the counters hotkey.status reports, and the clock of
// CGEvent timestamps.
//
// Two taps (three with a mouse binding), so macOS never waits on the helper
// for what it cannot swallow:
// - the keyboard tap is active (.defaultTap): it must answer, at once,
//   whether a key is swallowed (key bindings, Escape while recording, fn
//   with swallowFn), so macOS holds each key until the helper answers;
// - the pointer tap is listen-only: mouse presses only spoil a modifier
//   chord (an ⌥-click is no ⌥ tap) and always pass, so macOS sends the
//   helper a copy and goes on. A helper that is slow (swapped out, paging
//   back in) can no longer hold up a click (incident 2026-11-12);
// - the button tap, active, for the middle and side buttons only
//   (otherMouseDown/Up), installed only while a mouse button is bound: a
//   bound button's press is swallowed, so macOS must wait for the answer.
//   Left and right clicks never go through it. While it is installed the
//   pointer tap leaves the other buttons to it.

/// The CGEventType raw values the taps use (the Kit does not import
/// CoreGraphics; the helper's tests compare them with CGEventType).
public enum TapEvent {
    public static let leftMouseDown: UInt32 = 1
    public static let rightMouseDown: UInt32 = 3
    public static let keyDown: UInt32 = 10
    public static let keyUp: UInt32 = 11
    public static let flagsChanged: UInt32 = 12
    public static let otherMouseDown: UInt32 = 25
    public static let otherMouseUp: UInt32 = 26
    public static let disabledByTimeout: UInt32 = 0xFFFF_FFFE
    public static let disabledByUserInput: UInt32 = 0xFFFF_FFFF
}

public enum TapPlan {
    /// The active tap's events: the keyboard's, the only ones a binding can swallow.
    public static let keyboardMask: UInt64 = bit(TapEvent.keyDown) | bit(TapEvent.keyUp) | bit(TapEvent.flagsChanged)
    /// The listen-only tap's: mouse presses (not moves or scrolls: too many).
    public static let pointerMask: UInt64 = bit(TapEvent.leftMouseDown) | bit(TapEvent.rightMouseDown) | bit(TapEvent.otherMouseDown)
    /// The button tap's (active, only while a mouse button is bound): the
    /// middle and side buttons, never the left and right ones.
    public static let buttonMask: UInt64 = bit(TapEvent.otherMouseDown) | bit(TapEvent.otherMouseUp)

    static func bit(_ type: UInt32) -> UInt64 { 1 << UInt64(type) }

    /// Whether macOS disabled the tap (it was too slow, or the user's secure input).
    public static func isDisabled(_ type: UInt32) -> Bool {
        type == TapEvent.disabledByTimeout || type == TapEvent.disabledByUserInput
    }

    /// A keyboard tap's event as recognizer input; nil for other types.
    @inline(__always)
    public static func keyInput(type: UInt32, keyCode: UInt16, flags: UInt64, isRepeat: Bool, t: Double) -> KeyInput? {
        switch type {
        case TapEvent.flagsChanged: .flagsChanged(keyCode: keyCode, flags: flags, t: t)
        case TapEvent.keyDown: .keyDown(keyCode: keyCode, flags: flags, isRepeat: isRepeat, t: t)
        case TapEvent.keyUp: .keyUp(keyCode: keyCode, flags: flags, t: t)
        default: nil
        }
    }

    /// A pointer tap's event as recognizer input; nil for other types, and
    /// for the other buttons while the button tap has them.
    @inline(__always)
    public static func pointerInput(type: UInt32, t: Double, buttonTap: Bool = false) -> KeyInput? {
        switch type {
        case TapEvent.leftMouseDown, TapEvent.rightMouseDown: .pointer(t: t)
        case TapEvent.otherMouseDown: buttonTap ? nil : .pointer(t: t)
        default: nil
        }
    }

    /// A button tap's event as recognizer input (button: the event's
    /// mouseEventButtonNumber); nil for other types and for the left and
    /// right buttons.
    @inline(__always)
    public static func buttonInput(type: UInt32, button: Int, flags: UInt64, t: Double) -> KeyInput? {
        guard button >= 2 else { return nil }
        switch type {
        case TapEvent.otherMouseDown: return .mouseButton(button: button, down: true, flags: flags, t: t)
        case TapEvent.otherMouseUp: return .mouseButton(button: button, down: false, flags: flags, t: t)
        default: return nil
        }
    }

    /// The warm-up's events: a ⇧ chord spoiled by a key and a click, the
    /// recognizer's paths for flags, keys and the pointer. Run on a copy of
    /// the recognizer every second (HotkeyTap), they keep the pages the
    /// tap's path needs in use, so an idle helper is not paged out from
    /// under the next key. Nothing fires: the chord is spoiled.
    public static func warmUp(_ r: inout HotkeyRecognizer, now t: Double) -> Int {
        var n = 0
        let shift = RawFlags.shift | 0x2
        for input: KeyInput in [.flagsChanged(keyCode: 56, flags: shift, t: t), .keyDown(keyCode: 0x7F, flags: shift, isRepeat: false, t: t),
                                .pointer(t: t), .mouseButton(button: 31, down: true, flags: shift, t: t), .mouseButton(button: 31, down: false, flags: shift, t: t),
                                .keyUp(keyCode: 0x7F, flags: shift, t: t), .flagsChanged(keyCode: 56, flags: 0, t: t)] {
            let res = r.handle(input)
            n += res.events.count + (res.swallow ? 1 : 0)
        }
        return n
    }
}

/// One tap's counters, written by its callback without a lock.
public final class TapStats: Sendable {
    private let events = Atomic<Int>(0)
    private let disabled = Atomic<Int>(0)
    /// VoxtMini's own posted events seen (InjectedEvent), for a paste's log.
    private let own = Atomic<Int>(0)
    /// When macOS last disabled the tap, monotonic µs; -1: never.
    private let lastDisabledMicros = Atomic<Int>(-1)
    private let maxCallbackNs = Atomic<Int>(0)
    /// -1 while no event's lag could be measured.
    private let maxLagNs = Atomic<Int>(-1)

    public init() {}

    @inline(__always) public func event() { events.add(1, ordering: .relaxed) }
    @inline(__always) public func ownEvent() { own.add(1, ordering: .relaxed) }
    /// macOS disabled the tap at monotonic time ms.
    public func disabledOnce(at ms: Double) {
        disabled.add(1, ordering: .relaxed)
        lastDisabledMicros.store(Int(ms * 1000), ordering: .relaxed)
    }
    @inline(__always) public func callback(ns: Int) { Self.raise(maxCallbackNs, to: ns) }
    @inline(__always) public func lag(ns: Int) { Self.raise(maxLagNs, to: ns) }

    private static func raise(_ a: borrowing Atomic<Int>, to v: Int) {
        var cur = a.load(ordering: .relaxed)
        while v > cur {
            let (done, orig) = a.compareExchange(expected: cur, desired: v, ordering: .relaxed)
            if done { return }
            cur = orig
        }
    }

    public var eventCount: Int { events.load(ordering: .relaxed) }
    public var disabledCount: Int { disabled.load(ordering: .relaxed) }
    public var ownCount: Int { own.load(ordering: .relaxed) }
    /// How long before now (monotonic ms) the tap was last disabled; nil: never.
    public func disabledAgoMs(now: Double) -> Double? {
        let us = lastDisabledMicros.load(ordering: .relaxed)
        return us < 0 ? nil : max(0, ((now * 1000 - Double(us)) / 100).rounded() / 10)
    }
    /// The slowest callback, in µs to 0.1.
    public var maxCallbackMicros: Double { (Double(maxCallbackNs.load(ordering: .relaxed)) / 100).rounded() / 10 }
    /// The longest an event waited for its callback (its timestamp to the
    /// callback's start), in ms to 0.1; nil when unknown.
    public var maxLagMs: Double? {
        let ns = maxLagNs.load(ordering: .relaxed)
        return ns < 0 ? nil : (Double(ns) / 100_000).rounded() / 10
    }

    public func status(installed: Bool, error: String?) -> HotkeyTapStatus {
        HotkeyTapStatus(installed: installed, events: eventCount, disabledByTimeout: disabledCount,
                        maxCallbackMicros: maxCallbackMicros, maxLagMs: maxLagMs, error: error)
    }
}

/// The clock of CGEvent timestamps. Not CLOCK_UPTIME_RAW's nanoseconds
/// (the spike's lagMs was always out of range): the helper finds it once,
/// against a fresh event's timestamp, among the candidates, and measures
/// each event's lag on it.
public struct EventClock: Equatable, Sendable {
    public enum Source: String, CaseIterable, Sendable {
        /// clock_gettime_nsec_np(CLOCK_UPTIME_RAW), ns.
        case uptimeNs
        /// mach_absolute_time(), ticks.
        case absoluteTicks
        /// clock_gettime_nsec_np(CLOCK_MONOTONIC_RAW), ns (counts sleep).
        case continuousNs
        /// mach_continuous_time(), ticks.
        case continuousTicks
    }

    public let source: Source
    /// Ticks to ns (mach_timebase_info), 1/1 for the ns sources.
    public let numer: UInt64
    public let denom: UInt64

    public init(source: Source, numer: UInt64, denom: UInt64) {
        self.source = source
        self.numer = numer
        self.denom = max(1, denom)
    }

    /// The source a timestamp taken at `now` (each source's reading) is on:
    /// the one within 50 ms of it. nil when none is (an unknown clock: no lag).
    public static func pick(timestamp: UInt64, now: [Source: UInt64], numer: UInt64, denom: UInt64) -> EventClock? {
        for source in Source.allCases {
            guard let n = now[source] else { continue }
            let ticks = source == .absoluteTicks || source == .continuousTicks
            let c = EventClock(source: source, numer: ticks ? numer : 1, denom: ticks ? denom : 1)
            if let lag = c.lagNs(timestamp: timestamp, now: n, allowNegative: true), abs(lag) < 50_000_000 { return c }
        }
        return nil
    }

    /// now − timestamp in ns; nil when the timestamp is ahead (not this
    /// clock, or a posted event's own time) or more than a minute behind.
    @inline(__always)
    public func lagNs(timestamp: UInt64, now: UInt64, allowNegative: Bool = false) -> Int? {
        let neg = timestamp > now
        if neg, !allowNegative { return nil }
        let d = neg ? timestamp - now : now - timestamp
        let (hi, lo) = d.multipliedFullWidth(by: numer)
        guard hi < denom else { return nil }
        let ns = denom.dividingFullWidth((hi, lo)).quotient
        guard ns < 60_000_000_000 else { return nil }
        return neg ? -Int(ns) : Int(ns)
    }
}
