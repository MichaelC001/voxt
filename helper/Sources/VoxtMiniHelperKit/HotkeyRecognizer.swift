import Foundation

// The hotkey recognizer: pure logic, no CoreGraphics. The event tap feeds it
// every keyboard event (flags, key down/up) with a timestamp and asks, at
// once, whether to swallow the event; timers call tick(now:) at
// nextDeadline. Go configures the bindings (hotkey.configure) and gets
// hotkey.event notifications; what a trigger means (start, stop, cancel) is
// Go's business.
//
// Modifier-only chords (fn, fn+⇧, fn+⌃, right ⌘ …) are judged per "chord
// session": from the first modifier pressed to the last one released. The
// chord is every modifier seen during the session, so fn then ⇧ is fn+⇧, never
// fn. A non-modifier key during the session (fn+→, ⌘C, right ⌥+E typing é)
// spoils it, and so does a mouse button (⌥-click, ⌥-drag): the
// modifier was used for something else. Modifier-only chords are never
// swallowed (fn's own events aside, with swallowFn), so a modifier the
// user holds to type or click works as usual: the shortcut just does not
// fire. Sides are told apart by the device bits of the flags: a binding
// for right ⌥ ignores left ⌥, so ⌥-characters typed with the left key are
// untouched.
//
// Mouse bindings (the middle and side buttons, with modifiers) work as key
// bindings do: a tap fires on the press, a long press fires down on the
// press and up on the release, both swallowed. A button bound to a tap and
// a double tap (Voxt's middle-button preset: click transcribes, double
// click rewrites) defers the tap by doubleTapMs, as a modifier chord does.
//
// The Space window (config.spaceFollowUp, Go opens it while the answer card
// can take a follow-up): the next bare Space fires "space" and is
// swallowed. Any other key down or mouse press closes the window first, so
// Space typed after anything else is the user's; it opens again only when a
// configure turns spaceFollowUp on after one without it.

//
// The tap calls handle() synchronously for every keyboard event while macOS
// waits (an active tap), so its path keeps to a few words of inline state:
// modifier sets are bits (ModKeys, KeyBits), the bindings are compiled and
// sorted once by configure(), and an event that fires nothing allocates
// nothing (HotkeyAllocationTests). Its loops index the arrays: a for-in
// iterator allocates in an unoptimized build (swift test), where those tests
// run too.

/// A binding with its chord rule as bits: the keys it allows and the
/// families it needs (one key each, at least).
struct CompiledBinding: Sendable {
    let binding: Binding
    /// Keys the chord may hold: the binding's families, on the allowed sides.
    let allowed: ModKeys
    /// The families' bit groups the chord must touch.
    let required: [ModKeys]
    /// A key binding's families as CGEventFlags (fn aside).
    let familyFlags: UInt64

    init(_ b: Binding) {
        binding = b
        var allowed = ModKeys.empty, required: [ModKeys] = [], flags: UInt64 = 0
        for family in ModFamily.allCases {
            guard let rule = b.modifiers[family] else { continue }
            let keys = family.keys
            switch rule {
            case .any: allowed = allowed.union(keys)
            case .left: allowed = allowed.union(keys.subtracting(.right))
            case .right: allowed = allowed.union(keys.intersection(.right))
            }
            required.append(keys)
            flags |= family.flag
        }
        self.allowed = allowed
        self.required = required
        familyFlags = flags
    }

    /// Exactly this binding's chord: every key allowed, every family there.
    @inline(__always)
    func matches(_ chord: ModKeys) -> Bool {
        if !chord.subtracting(allowed).isEmpty { return false }
        var i = 0
        while i < required.count {
            if chord.intersection(required[i]).isEmpty { return false }
            i += 1
        }
        return true
    }

    /// A key event's families (fn aside) are exactly this binding's.
    @inline(__always)
    func matches(flags: UInt64) -> Bool { flags & RawFlags.families == familyFlags }
}

public struct HotkeyRecognizer: Sendable {
    public private(set) var config: HotkeyConfig
    public init(config: HotkeyConfig) {
        self.config = config
        spaceOpen = config.spaceFollowUp
        compile()
    }

    /// Modifier events found missing so far (diagnostics).
    public private(set) var missedEvents = 0

    // Compiled from config (configure): most specific first.
    private var chordBindings: [CompiledBinding] = []
    private var keyBindings: [CompiledBinding] = []
    private var mouseBindings: [CompiledBinding] = []
    /// swallowFn, and an fn chord is bound.
    private var swallowsFn = false

    // Chord session.
    private var pressed = ModKeys.empty
    private var chordStart: Double?
    private var chordMax = ModKeys.empty
    private var spoiled = false
    private var longPressFired: Binding?
    private var longPressDeadline: (binding: CompiledBinding, at: Double)?
    // Double tap.
    private var doubleArm: (chord: ModKeys, until: Double)?
    private var pendingTap: (binding: Binding, at: Double, heldMs: Double)?
    // Key bindings.
    private var swallowedKeys = KeyBits()
    private var keyDownAt: [UInt16: Double] = [:]
    private var keyLongFired: [UInt16: Binding] = [:]
    private var keyDoubleArm: (key: UInt16, until: Double)?
    // Mouse bindings.
    private var mouseSwallowed: Set<Int> = []
    private var mouseDownAt: [Int: Double] = [:]
    private var mouseLongFired: [Int: Binding] = [:]
    private var mouseDoubleArm: (button: Int, until: Double)?
    private var pendingMouseTap: (binding: Binding, at: Double)?
    /// The Space window is open: the next bare Space is a follow-up.
    public private(set) var spaceOpen = false

    /// Whether a mouse button is bound (the helper then installs the button tap).
    public var bindsMouse: Bool { !mouseBindings.isEmpty }

    private mutating func compile() {
        let sorted = config.bindings.sorted { $0.specificity > $1.specificity }.map(CompiledBinding.init)
        chordBindings = sorted.filter { $0.binding.key == nil && $0.binding.mouseButton == nil }
        keyBindings = sorted.filter { $0.binding.key != nil }
        mouseBindings = sorted.filter { $0.binding.key == nil && $0.binding.mouseButton != nil }
        swallowsFn = config.swallowFn && chordBindings.contains { $0.binding.modifiers[.fn] != nil }
    }

    /// Sets the config. Changing only the timings or "active" keeps a
    /// chord in progress (Go flips active as a session starts, while the
    /// key that started it may still be down).
    public mutating func configure(_ c: HotkeyConfig) {
        let same = c.bindings == config.bindings
        if !c.spaceFollowUp {
            spaceOpen = false
        } else if !config.spaceFollowUp {
            spaceOpen = true
        }
        config = c
        compile()
        if !same { reset() }
    }

    /// Forgets transient state (the tap was disabled, the config changed).
    public mutating func reset() {
        chordStart = nil
        chordMax = .empty
        spoiled = false
        longPressFired = nil
        longPressDeadline = nil
        doubleArm = nil
        pendingTap = nil
        keyDoubleArm = nil
        if !keyLongFired.isEmpty { keyLongFired = [:] }
        if !keyDownAt.isEmpty { keyDownAt = [:] }
        mouseDoubleArm = nil
        pendingMouseTap = nil
        if !mouseLongFired.isEmpty { mouseLongFired = [:] }
        if !mouseDownAt.isEmpty { mouseDownAt = [:] }
    }

    /// Closes the Space window (the tap found another app in front, or
    /// secure input).
    public mutating func closeSpace() { spaceOpen = false }

    /// When tick(now:) must run next, if at all.
    public var nextDeadline: Double? {
        // A deferred tap waits while a chord is down: it may be the second tap.
        let p = chordStart == nil ? pendingTap?.at : nil
        let l = longPressDeadline?.at
        let m = pendingMouseTap?.at
        var next: Double? = p
        if let l { next = min(next ?? l, l) }
        if let m { next = min(next ?? m, m) }
        return next
    }

    /// Feeds one event; returns what fired and whether to swallow it.
    public mutating func handle(_ input: KeyInput) -> (events: [HotkeyEvent], swallow: Bool) {
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
            swallow = swallowsFn
        case .keyDown(let keyCode, let flags, let isRepeat, let t):
            if !pressed.isEmpty { spoiled = true; longPressDeadline = nil }
            swallow = handleKeyDown(keyCode: keyCode, flags: flags, isRepeat: isRepeat, t: t, &out)
        case .keyUp(let keyCode, _, let t):
            swallow = handleKeyUp(keyCode: keyCode, t: t, &out)
        case .pointer:
            spaceOpen = false
            if !pressed.isEmpty { spoiled = true; longPressDeadline = nil }
        case .mouseButton(let button, let down, let flags, let t):
            if down {
                spaceOpen = false
                if !pressed.isEmpty { spoiled = true; longPressDeadline = nil }
                swallow = handleMouseDown(button: button, flags: flags, t: t, &out)
            } else {
                swallow = handleMouseUp(button: button, t: t, &out)
            }
        }
        return (out, swallow)
    }

    /// Fires what is due at now (deferred taps, long presses).
    public mutating func tick(now: Double) -> [HotkeyEvent] {
        var out: [HotkeyEvent] = []
        if let p = pendingTap, now >= p.at, chordStart == nil {
            out.append(HotkeyEvent(binding: p.binding.id, phase: .trigger, behavior: .tap, heldMs: p.heldMs, t: p.at))
            pendingTap = nil
            doubleArm = nil
        }
        if let m = pendingMouseTap, now >= m.at {
            out.append(HotkeyEvent(binding: m.binding.id, phase: .trigger, behavior: .tap, heldMs: nil, t: m.at))
            pendingMouseTap = nil
            mouseDoubleArm = nil
        }
        if let d = longPressDeadline, now >= d.at {
            longPressDeadline = nil
            if !spoiled, pressed == chordMax, d.binding.matches(pressed) {
                longPressFired = d.binding.binding
                out.append(HotkeyEvent(binding: d.binding.binding.id, phase: .down, behavior: .longPress, heldMs: d.at - (chordStart ?? d.at), t: d.at))
            }
        }
        return out
    }

    private mutating func handleFlags(keyCode: UInt16, flags: UInt64, t: Double, _ out: inout [HotkeyEvent]) -> Bool {
        var now = RawFlags.pressedKeys(flags)
        // fn's own event decides fn (some keyboards jitter the flag on others).
        if keyCode == KeyCode.function {
            if flags & RawFlags.secondaryFn != 0 { now.insert(.fn) } else { now.remove(.fn) }
        }
        let swallowFn = keyCode == KeyCode.function && swallowsFn
        var was = pressed
        // A modifier's flagsChanged always toggles that key. If its state
        // did not change, we missed an event (seen: the system eats fn's
        // release after fn+⌃): the chord in progress is unreliable, drop it
        // without firing and start over from this event.
        if let k = ModKey.forKeyCode(keyCode), was.contains(k) == now.contains(k) {
            missedEvents += 1
            chordStart = nil
            chordMax = .empty
            spoiled = false
            longPressFired = nil
            longPressDeadline = nil
            doubleArm = nil
            pendingTap = nil
            var before = now
            before.remove(k)
            was = now.contains(k) ? before : now
        }
        pressed = now
        if was.isEmpty, !now.isEmpty {
            chordStart = t
            chordMax = now
            spoiled = false
            longPressFired = nil
            if let arm = doubleArm, t > arm.until { doubleArm = nil }
        } else {
            chordMax = chordMax.union(now)
        }

        if now.isEmpty, !was.isEmpty {
            release(t: t, &out)
        } else if !now.isEmpty {
            // A long press waits for the chord to settle.
            longPressDeadline = nil
            if !spoiled, now == chordMax, longPressFired == nil {
                var i = 0
                while i < chordBindings.count {
                    let b = chordBindings[i]
                    if b.binding.behavior == .longPress, b.matches(now) {
                        longPressDeadline = (b, t + config.longPressMs)
                        break
                    }
                    i += 1
                }
            }
        }
        return swallowFn
    }

    private mutating func release(t: Double, _ out: inout [HotkeyEvent]) {
        defer {
            chordStart = nil
            chordMax = .empty
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
        // The most specific matching tap and double tap.
        var tapB: Binding?, dblB: Binding?
        var i = 0
        while i < chordBindings.count, tapB == nil || dblB == nil {
            let c = chordBindings[i]
            if c.matches(chord) {
                switch c.binding.behavior {
                case .tap: if tapB == nil { tapB = c.binding }
                case .doubleTap: if dblB == nil { dblB = c.binding }
                case .longPress: break
                }
            }
            i += 1
        }
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

    private mutating func handleKeyDown(keyCode: UInt16, flags: UInt64, isRepeat: Bool, t: Double, _ out: inout [HotkeyEvent]) -> Bool {
        if spaceOpen {
            spaceOpen = false
            if keyCode == KeyCode.space, flags & (RawFlags.families | RawFlags.secondaryFn) == 0, !isRepeat {
                out.append(HotkeyEvent(binding: "space", phase: .trigger, behavior: .tap, heldMs: nil, t: t))
                swallowedKeys.insert(keyCode)
                return true
            }
        }
        if config.active, !config.passEscape, keyCode == KeyCode.escape, flags & RawFlags.families == 0 {
            if !isRepeat { out.append(HotkeyEvent(binding: "escape", phase: .trigger, behavior: .tap, heldMs: nil, t: t)) }
            swallowedKeys.insert(keyCode)
            return true
        }
        var match: Binding?
        var i = 0
        while i < keyBindings.count {
            if keyBindings[i].binding.key == keyCode, keyBindings[i].matches(flags: flags) {
                match = keyBindings[i].binding
                break
            }
            i += 1
        }
        guard let b = match else {
            return swallowedKeys.contains(keyCode) // repeats of a swallowed key
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
        if !keyLongFired.isEmpty, let b = keyLongFired.removeValue(forKey: keyCode) {
            out.append(HotkeyEvent(binding: b.id, phase: .up, behavior: .longPress, heldMs: t - (keyDownAt[keyCode] ?? t), t: t))
        }
        if !keyDownAt.isEmpty { keyDownAt[keyCode] = nil }
        // A key up is swallowed only when its key down was.
        return swallowedKeys.remove(keyCode)
    }
    // MARK: mouse bindings

    /// The first mouse binding of each behaviour for this button and flags.
    private func mouseMatches(button: Int, flags: UInt64) -> (tap: Binding?, double: Binding?, long: Binding?) {
        var tapB: Binding?, dblB: Binding?, longB: Binding?
        var i = 0
        while i < mouseBindings.count {
            let c = mouseBindings[i]
            if c.binding.mouseButton == button, c.matches(flags: flags) {
                switch c.binding.behavior {
                case .tap: if tapB == nil { tapB = c.binding }
                case .doubleTap: if dblB == nil { dblB = c.binding }
                case .longPress: if longB == nil { longB = c.binding }
                }
            }
            i += 1
        }
        return (tapB, dblB, longB)
    }

    private mutating func handleMouseDown(button: Int, flags: UInt64, t: Double, _ out: inout [HotkeyEvent]) -> Bool {
        let (tapB, dblB, longB) = mouseMatches(button: button, flags: flags)
        guard let any = tapB ?? dblB ?? longB else { return false }
        let swallow = any.swallow ?? true
        if swallow { mouseSwallowed.insert(button) }
        mouseDownAt[button] = t
        // A session records: a tap stops it at once.
        if config.active, let b = tapB ?? dblB {
            pendingMouseTap = nil
            mouseDoubleArm = nil
            out.append(HotkeyEvent(binding: b.id, phase: .trigger, behavior: b.behavior, heldMs: nil, t: t))
            return swallow
        }
        if let longB {
            mouseLongFired[button] = longB
            out.append(HotkeyEvent(binding: longB.id, phase: .down, behavior: .longPress, heldMs: nil, t: t))
            return swallow
        }
        if let dblB {
            if let arm = mouseDoubleArm, arm.button == button, t <= arm.until {
                mouseDoubleArm = nil
                pendingMouseTap = nil
                out.append(HotkeyEvent(binding: dblB.id, phase: .trigger, behavior: .doubleTap, heldMs: nil, t: t))
                return swallow
            }
            // Another button ends a pending double click: its tap fires now.
            if let p = pendingMouseTap {
                out.append(HotkeyEvent(binding: p.binding.id, phase: .trigger, behavior: .tap, heldMs: nil, t: t))
                pendingMouseTap = nil
            }
            mouseDoubleArm = (button, t + config.doubleTapMs)
            if let tapB { pendingMouseTap = (tapB, t + config.doubleTapMs) }
            return swallow
        }
        if let p = pendingMouseTap {
            out.append(HotkeyEvent(binding: p.binding.id, phase: .trigger, behavior: .tap, heldMs: nil, t: t))
            pendingMouseTap = nil
            mouseDoubleArm = nil
        }
        if let tapB {
            out.append(HotkeyEvent(binding: tapB.id, phase: .trigger, behavior: .tap, heldMs: nil, t: t))
        }
        return swallow
    }

    private mutating func handleMouseUp(button: Int, t: Double, _ out: inout [HotkeyEvent]) -> Bool {
        if let b = mouseLongFired.removeValue(forKey: button) {
            out.append(HotkeyEvent(binding: b.id, phase: .up, behavior: .longPress, heldMs: t - (mouseDownAt[button] ?? t), t: t))
        }
        mouseDownAt[button] = nil
        // An up is swallowed only when its down was.
        return mouseSwallowed.remove(button) != nil
    }
}
