@testable import VoxtMiniHelperKit
import Foundation

// The recognizer as it was before the tap's hot path was made
// allocation-free (Set-based, filters per event): the reference the
// differential test (HotkeyDifferentialTests) holds the new one to. Do not
// change it; it is the old semantics.

struct ReferenceRecognizer: Sendable {
    private(set) var config: HotkeyConfig
    init(config: HotkeyConfig) { self.config = config }

    /// Modifier events found missing so far (diagnostics).
    private(set) var missedEvents = 0

    // Chord session.
    private var pressed = Set<ModKey>()
    private var chordStart: Double?
    private var chordMax = Set<ModKey>()
    private var spoiled = false
    private var longPressFired: Binding?
    private var longPressDeadline: (binding: Binding, at: Double)?
    // Double tap.
    private var doubleArm: (chord: Set<ModKey>, until: Double)?
    private var pendingTap: (binding: Binding, at: Double, heldMs: Double)?
    // Key bindings.
    private var swallowedKeys = Set<UInt16>()
    private var keyDownAt: [UInt16: Double] = [:]
    private var keyLongFired: [UInt16: Binding] = [:]
    private var keyDoubleArm: (key: UInt16, until: Double)?

    /// Sets the config. Changing only the timings or "active" keeps a
    /// chord in progress (Go flips active as a session starts, while the
    /// key that started it may still be down).
    mutating func configure(_ c: HotkeyConfig) {
        let same = c.bindings == config.bindings
        config = c
        if !same { reset() }
    }

    /// Forgets transient state (the tap was disabled, the config changed).
    mutating func reset() {
        chordStart = nil
        chordMax = []
        spoiled = false
        longPressFired = nil
        longPressDeadline = nil
        doubleArm = nil
        pendingTap = nil
        keyDoubleArm = nil
        keyLongFired = [:]
        keyDownAt = [:]
    }

    /// When tick(now:) must run next, if at all.
    var nextDeadline: Double? {
        // A deferred tap waits while a chord is down: it may be the second tap.
        [chordStart == nil ? pendingTap?.at : nil, longPressDeadline?.at].compactMap { $0 }.min()
    }

    /// Feeds one event; returns what fired and whether to swallow it.
    mutating func handle(_ input: KeyInput) -> (events: [HotkeyEvent], swallow: Bool) {
        var out = tick(now: input.t)
        var swallow = false
        switch input {
        case .flagsChanged(let keyCode, let flags, let t):
            if keyCode == KeyCode.capsLock { break }
            swallow = handleFlags(keyCode: keyCode, flags: flags, t: t, &out)
        case .keyDown(let keyCode, _, _, _) where keyCode == KeyCode.globe,
             .keyUp(let keyCode, _, _) where keyCode == KeyCode.globe:
            // The 🌐 key's own key events, sent as fn is released; macOS runs
            // the 🌐 action (emoji, input source) on them. Swallowed with
            // swallowFn; they never spoil a chord.
            swallow = config.swallowFn && chordBindings.contains { $0.modifiers[.fn] != nil }
        case .keyDown(let keyCode, let flags, let isRepeat, let t):
            if !pressed.isEmpty { spoiled = true; longPressDeadline = nil }
            swallow = handleKeyDown(keyCode: keyCode, flags: flags, isRepeat: isRepeat, t: t, &out)
        case .keyUp(let keyCode, _, let t):
            swallow = handleKeyUp(keyCode: keyCode, t: t, &out)
        case .pointer:
            if !pressed.isEmpty { spoiled = true; longPressDeadline = nil }
        case .mouseButton:
            break // newer than the reference: never in the differential streams
        }
        return (out, swallow)
    }

    /// Fires what is due at now (deferred taps, long presses).
    mutating func tick(now: Double) -> [HotkeyEvent] {
        var out: [HotkeyEvent] = []
        if let p = pendingTap, now >= p.at, chordStart == nil {
            out.append(HotkeyEvent(binding: p.binding.id, phase: .trigger, behavior: .tap, heldMs: p.heldMs, t: p.at))
            pendingTap = nil
            doubleArm = nil
        }
        if let d = longPressDeadline, now >= d.at {
            longPressDeadline = nil
            if !spoiled, pressed == chordMax, d.binding.matches(chord: pressed) {
                longPressFired = d.binding
                out.append(HotkeyEvent(binding: d.binding.id, phase: .down, behavior: .longPress, heldMs: d.at - (chordStart ?? d.at), t: d.at))
            }
        }
        return out
    }

    private var chordBindings: [Binding] {
        config.bindings.filter { $0.key == nil }.sorted { $0.specificity > $1.specificity }
    }

    private mutating func handleFlags(keyCode: UInt16, flags: UInt64, t: Double, _ out: inout [HotkeyEvent]) -> Bool {
        var now = RawFlags.pressed(flags)
        // fn's own event decides fn (some keyboards jitter the flag on others).
        if keyCode == KeyCode.function {
            if flags & RawFlags.secondaryFn != 0 { now.insert(.fn) } else { now.remove(.fn) }
        }
        let swallowFn = keyCode == KeyCode.function && config.swallowFn &&
            chordBindings.contains { $0.modifiers[.fn] != nil }
        var was = pressed
        // A modifier's flagsChanged always toggles that key. If its state
        // did not change, we missed an event (seen: the system eats fn's
        // release after fn+⌃): the chord in progress is unreliable, drop it
        // without firing and start over from this event.
        if let k = ModKey.forKeyCode(keyCode), was.contains(k) == now.contains(k) {
            missedEvents += 1
            chordStart = nil
            chordMax = []
            spoiled = false
            longPressFired = nil
            longPressDeadline = nil
            doubleArm = nil
            pendingTap = nil
            was = now.contains(k) ? now.subtracting([k]) : now
        }
        pressed = now
        if was.isEmpty, !now.isEmpty {
            chordStart = t
            chordMax = now
            spoiled = false
            longPressFired = nil
            if let arm = doubleArm, t > arm.until { doubleArm = nil }
        } else {
            chordMax.formUnion(now)
        }

        if now.isEmpty, !was.isEmpty {
            release(t: t, &out)
        } else if !now.isEmpty {
            // A long press waits for the chord to settle.
            longPressDeadline = nil
            if !spoiled, now == chordMax, longPressFired == nil,
               let b = chordBindings.first(where: { $0.behavior == .longPress && $0.matches(chord: now) }) {
                longPressDeadline = (b, t + config.longPressMs)
            }
        }
        return swallowFn
    }

    private mutating func release(t: Double, _ out: inout [HotkeyEvent]) {
        defer {
            chordStart = nil
            chordMax = []
            longPressDeadline = nil
            longPressFired = nil
            spoiled = false
        }
        let held = t - (chordStart ?? t)
        if let fired = longPressFired {
            out.append(HotkeyEvent(binding: fired.id, phase: .up, behavior: .longPress, heldMs: held, t: t))
            doubleArm = nil
            return
        }
        guard !spoiled, held <= config.tapMaxMs else {
            doubleArm = nil
            return
        }
        let chord = chordMax
        let matching = chordBindings.filter { $0.matches(chord: chord) }
        let tapB = matching.first { $0.behavior == .tap }
        let dblB = matching.first { $0.behavior == .doubleTap }
        if config.active, let b = tapB ?? dblB {
            pendingTap = nil
            doubleArm = nil
            out.append(HotkeyEvent(binding: b.id, phase: .trigger, behavior: b.behavior, heldMs: held, t: t))
            return
        }
        if let arm = doubleArm, arm.chord == chord, (chordStart ?? t) <= arm.until, let dblB {
            doubleArm = nil
            pendingTap = nil
            out.append(HotkeyEvent(binding: dblB.id, phase: .trigger, behavior: .doubleTap, heldMs: held, t: t))
            return
        }
        // A different chord ends a pending double tap: its tap fires now.
        if let p = pendingTap {
            out.append(HotkeyEvent(binding: p.binding.id, phase: .trigger, behavior: .tap, heldMs: p.heldMs, t: t))
            pendingTap = nil
        }
        doubleArm = nil
        if dblB != nil {
            doubleArm = (chord, t + config.doubleTapMs)
            if let tapB { pendingTap = (tapB, t + config.doubleTapMs, held) }
        } else if let tapB {
            out.append(HotkeyEvent(binding: tapB.id, phase: .trigger, behavior: .tap, heldMs: held, t: t))
        }
    }

    private var keyBindings: [Binding] { config.bindings.filter { $0.key != nil }.sorted { $0.specificity > $1.specificity } }

    private mutating func handleKeyDown(keyCode: UInt16, flags: UInt64, isRepeat: Bool, t: Double, _ out: inout [HotkeyEvent]) -> Bool {
        if config.active, !config.passEscape, keyCode == KeyCode.escape, RawFlags.families(flags).isEmpty {
            if !isRepeat { out.append(HotkeyEvent(binding: "escape", phase: .trigger, behavior: .tap, heldMs: nil, t: t)) }
            swallowedKeys.insert(keyCode)
            return true
        }
        let families = RawFlags.families(flags)
        guard let b = keyBindings.first(where: { $0.key == keyCode && $0.matches(families: families) }) else {
            if swallowedKeys.contains(keyCode) { return true } // repeats of a swallowed key
            return false
        }
        let swallow = b.swallow ?? true
        if swallow { swallowedKeys.insert(keyCode) }
        if isRepeat { return swallow }
        keyDownAt[keyCode] = t
        switch b.behavior {
        case .tap:
            out.append(HotkeyEvent(binding: b.id, phase: .trigger, behavior: .tap, heldMs: nil, t: t))
        case .longPress:
            keyLongFired[keyCode] = b
            out.append(HotkeyEvent(binding: b.id, phase: .down, behavior: .longPress, heldMs: nil, t: t))
        case .doubleTap:
            if let arm = keyDoubleArm, arm.key == keyCode, t <= arm.until {
                keyDoubleArm = nil
                out.append(HotkeyEvent(binding: b.id, phase: .trigger, behavior: .doubleTap, heldMs: nil, t: t))
            } else {
                keyDoubleArm = (keyCode, t + config.doubleTapMs)
            }
        }
        return swallow
    }

    private mutating func handleKeyUp(keyCode: UInt16, t: Double, _ out: inout [HotkeyEvent]) -> Bool {
        if let b = keyLongFired.removeValue(forKey: keyCode) {
            out.append(HotkeyEvent(binding: b.id, phase: .up, behavior: .longPress, heldMs: t - (keyDownAt[keyCode] ?? t), t: t))
        }
        keyDownAt[keyCode] = nil
        // A key up is swallowed only when its key down was.
        return swallowedKeys.remove(keyCode) != nil
    }
}
