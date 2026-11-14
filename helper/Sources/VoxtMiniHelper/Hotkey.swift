import ApplicationServices
import Carbon
import VoxtMiniHelperKit
import CoreGraphics
import Foundation
import Synchronization

// The global hotkey event taps, two on one thread and run loop (the pure
// half, which event goes where and the counters: HotkeyTapKit.swift):
//
// - the keyboard tap, an active (.defaultTap) session tap: macOS authorizes
//   it with Accessibility alone (a listen-only keyboard tap would need Input
//   Monitoring), checked against VoxtMini.app, the helper's responsible
//   process. macOS holds every key until its callback answers whether to
//   swallow it, so the callback is short: one uncontended lock around the
//   recognizer (no allocation for an event that fires nothing), counters
//   without a lock, notifications queued for another thread;
// - the pointer tap, listen-only, for mouse presses: they only spoil a
//   modifier chord (an ⌥-click is no ⌥ tap) and always pass, so macOS does
//   not wait for it. Before 2026-11-12 they were in the active tap, and a
//   swapped-out helper held up every click for seconds while it paged in.
//   Both taps' sources are on the same run loop, which takes their
//   messages in arrival order: a click before ⌥'s release is judged before
//   it. Mouse events need no permission to listen to; should the tap still
//   fail, chords are not spoiled by clicks (status: pointer.installed).
//
// - the button tap, active, for the middle and side buttons only
//   (otherMouseDown/Up), and only while a mouse button is bound (Voxt's
//   middle-button shortcut): a bound button's press and release are
//   swallowed, so macOS waits for it, but never for a left or right click.
//   It comes and goes with the bindings (syncButtonTap); while it is there
//   the pointer tap leaves the other buttons to it.
//
// The Space window (spaceFollowUp, the answer card's follow-up): the
// recognizer takes the next bare Space; this side closes the window first
// when the app in front is not the one it opened on, or secure input is on.
//
// A warm-up every second runs the recognizer's paths on a copy, so an idle
// helper keeps the pages its keyboard path needs; whatever macOS waits on
// still has to page in (the callback, CoreGraphics' part), which
// maxLagMs shows: an event's timestamp to its callback.
//
// Without Accessibility the tap cannot be created: it retries every 2 s
// and sends hotkey.state once in (no restart needed after the user grants
// it). Accessibility taken away removes the taps, and it retries again.
//
// Events VoxtMini posts itself (⌘V, ⌘C, Return) go in after the session's
// taps (postKey), so this tap should not see them; should it, they carry
// InjectedEvent.marker ("VOXT") in eventSourceUserData and pass untouched
// (counted: a paste's diag.tapSaw). Other apps' posted events (Auralink's
// "AURA") are judged as typing.

final class HotkeyTap: @unchecked Sendable {
    let out: Writer
    let front: FrontApps
    /// The recognizer: the taps' callbacks and timers (the tap thread),
    /// configure and status (the server's). Never held across a call out.
    private let recognizer = Mutex(HotkeyRecognizer(config: HotkeyConfig(bindings: [])))
    private let keyboardStats = TapStats()
    private let pointerStats = TapStats()
    private let buttonStats = TapStats()
    /// The button tap is installed (the pointer tap's callback reads it).
    private let buttonTapOn = Atomic<Bool>(false)
    /// The app in front as the Space window opened (pid; -1: unknown).
    private let spaceFront = Atomic<Int>(-1)
    /// The installation; not touched by the callbacks' usual path.
    private let lock = NSLock()
    private var keyboard: CFMachPort?
    private var pointer: CFMachPort?
    private var pointerError: String?
    private var buttons: CFMachPort?
    private var buttonSource: CFRunLoopSource?
    private var buttonError: String?
    /// Serializes syncButtonTap (configure's thread and the tap thread's start).
    private let buttonSync = NSLock()
    private var runLoop: CFRunLoop?
    private var timers: [CFRunLoopTimer] = []
    private var retry: DispatchSourceTimer?
    private var lastError: String?
    // The tap thread's own, set before the taps are enabled.
    private var deadlineTimer: CFRunLoopTimer?
    private var armed: Double?
    private var clock: EventClock?
    /// hotkey.event goes out in order, off the tap's thread.
    private let sendQueue = DispatchQueue(label: "voxtmini.hotkey.send", qos: .userInteractive)

    init(out: Writer, front: FrontApps) {
        self.out = out
        self.front = front
    }

    func configure(_ p: HotkeyConfigureParams) -> HotkeyStatus {
        let c = HotkeyConfig(bindings: p.bindings, doubleTapMs: p.doubleTapMs ?? systemDoubleClickMs(),
                             longPressMs: p.longPressMs ?? 300, tapMaxMs: p.tapMaxMs ?? 1500,
                             swallowFn: p.swallowFn ?? false, active: p.active ?? false,
                             passEscape: p.passEscape ?? false, spaceFollowUp: p.spaceFollowUp ?? false)
        let opened = recognizer.withLock { r -> Bool in
            let was = r.spaceOpen
            r.configure(c)
            return r.spaceOpen && !was
        }
        if opened { spaceFront.store(front.cached?.pid ?? -1, ordering: .relaxed) }
        if p.bindings.isEmpty {
            uninstall()
        } else if lock.withLock({ keyboard == nil && retry == nil }) {
            install()
        } else {
            syncButtonTap()
        }
        return status()
    }

    func status() -> HotkeyStatus {
        // AXIsProcessTrusted asks tccd: never under a lock the taps take.
        let trusted = AXIsProcessTrusted()
        let (bindings, missed) = recognizer.withLock { ($0.config.bindings.count, $0.missedEvents) }
        let bindsMouse = recognizer.withLock { $0.bindsMouse }
        let (installed, pointerInstalled, pErr, buttonsInstalled, bErr, err) = lock.withLock {
            (keyboard != nil, pointer != nil, pointerError, buttons != nil, buttonError, lastError)
        }
        return HotkeyStatus(installed: installed, accessibility: trusted, bindings: bindings,
                            events: keyboardStats.eventCount, disabledByTimeout: keyboardStats.disabledCount, missedEvents: missed,
                            maxCallbackMicros: keyboardStats.maxCallbackMicros, maxLagMs: keyboardStats.maxLagMs,
                            pointer: pointerStats.status(installed: pointerInstalled, error: pErr),
                            buttons: bindsMouse ? buttonStats.status(installed: buttonsInstalled, error: bErr) : nil, error: err)
    }

    /// For a paste's log (PasteDiag): VoxtMini's own posted keys the
    /// keyboard tap has seen so far, and how long ago macOS last disabled it.
    func probe() -> (own: Int, disabledAgoMs: Double?) {
        (keyboardStats.ownCount, keyboardStats.disabledAgoMs(now: monotonicMs()))
    }

    /// Creates the taps on their thread; without Accessibility it retries.
    private func install() {
        let ready = DispatchSemaphore(value: 0)
        let thread = Thread { [self] in
            let me = Unmanaged.passUnretained(self).toOpaque()
            let onKey: CGEventTapCallBack = { _, type, event, refcon in
                guard let refcon else { return Unmanaged.passUnretained(event) }
                let tap = Unmanaged<HotkeyTap>.fromOpaque(refcon).takeUnretainedValue()
                return tap.handleKey(type, event) ? nil : Unmanaged.passUnretained(event)
            }
            let onPointer: CGEventTapCallBack = { _, type, event, refcon in
                if let refcon { Unmanaged<HotkeyTap>.fromOpaque(refcon).takeUnretainedValue().handlePointer(type, event) }
                return Unmanaged.passUnretained(event)
            }
            guard let key = CGEvent.tapCreate(tap: .cgSessionEventTap, place: .headInsertEventTap, options: .defaultTap,
                                              eventsOfInterest: TapPlan.keyboardMask, callback: onKey, userInfo: me) else {
                let trusted = AXIsProcessTrusted()
                lock.withLock {
                    lastError = trusted ? "CGEvent.tapCreate failed; retrying every 2 s"
                        : "CGEvent.tapCreate failed: VoxtMini may not use Accessibility; retrying every 2 s"
                }
                ready.signal()
                scheduleRetry()
                return
            }
            CGEvent.tapEnable(tap: key, enable: false) // until the thread is ready
            let ptr = CGEvent.tapCreate(tap: .cgSessionEventTap, place: .headInsertEventTap, options: .listenOnly,
                                        eventsOfInterest: TapPlan.pointerMask, callback: onPointer, userInfo: me)
            if ptr == nil { log("the listen-only mouse tap could not be created: clicks do not spoil chords") }
            let loop = CFRunLoopGetCurrent()!
            clock = findEventClock()
            let timers = makeTimers(loop)
            for port in [key, ptr].compactMap({ $0 }) {
                CFRunLoopAddSource(loop, CFMachPortCreateRunLoopSource(kCFAllocatorDefault, port, 0), .commonModes)
            }
            let wasRetrying = lock.withLock { () -> Bool in
                keyboard = key
                pointer = ptr
                pointerError = ptr == nil ? "CGEvent.tapCreate (listen-only, mouse) failed" : nil
                runLoop = loop
                self.timers = timers
                lastError = nil
                defer { retry = nil }
                return retry != nil
            }
            CGEvent.tapEnable(tap: key, enable: true)
            if let ptr { CGEvent.tapEnable(tap: ptr, enable: true) }
            syncButtonTap()
            ready.signal()
            // status() asks tccd: not on the thread the taps wait on.
            if wasRetrying { DispatchQueue.global().async { [self] in out.notify(Method.hotkeyState, status()) } }
            watchTrust()
            CFRunLoopRun()
        }
        thread.name = "voxtmini.hotkey"
        thread.qualityOfService = .userInteractive
        thread.start()
        _ = ready.wait(timeout: .now() + 1)
    }

    /// The deadline timer (deferred taps, long presses), re-armed rather
    /// than made per event, and the warm-up. On the tap's thread.
    private func makeTimers(_ loop: CFRunLoop) -> [CFRunLoopTimer] {
        let never = CFAbsoluteTimeGetCurrent() + 1e9
        let deadline = CFRunLoopTimerCreateWithHandler(kCFAllocatorDefault, never, 1e9, 0, 0) { [weak self] _ in
            guard let self else { return }
            self.armed = nil
            let (fired, next) = self.recognizer.withLock { r -> ([HotkeyEvent], Double?) in
                (r.tick(now: monotonicMs()), r.nextDeadline)
            }
            self.send(fired)
            self.arm(next)
        }!
        let warm = CFRunLoopTimerCreateWithHandler(kCFAllocatorDefault, CFAbsoluteTimeGetCurrent() + 1, 1, 0, 0) { [weak self] _ in
            guard let self else { return }
            var copy = self.recognizer.withLock { $0 }
            _ = TapPlan.warmUp(&copy, now: monotonicMs())
        }!
        CFRunLoopTimerSetTolerance(warm, 0.5)
        deadlineTimer = deadline
        armed = nil
        CFRunLoopAddTimer(loop, deadline, .commonModes)
        CFRunLoopAddTimer(loop, warm, .commonModes)
        return [deadline, warm]
    }

    /// The clock of CGEvent timestamps, from a fresh event's.
    private func findEventClock() -> EventClock? {
        guard let probe = CGEvent(source: nil) else { return nil }
        var tb = mach_timebase_info_data_t()
        _ = mach_timebase_info(&tb)
        let now: [EventClock.Source: UInt64] = [
            .uptimeNs: clock_gettime_nsec_np(CLOCK_UPTIME_RAW), .absoluteTicks: mach_absolute_time(),
            .continuousNs: clock_gettime_nsec_np(CLOCK_MONOTONIC_RAW), .continuousTicks: mach_continuous_time(),
        ]
        let c = EventClock.pick(timestamp: probe.timestamp, now: now, numer: UInt64(tb.numer), denom: UInt64(tb.denom))
        if c == nil { log("event timestamps are on no known clock: no maxLagMs") }
        return c
    }

    private func scheduleRetry() {
        let t = DispatchSource.makeTimerSource(queue: .global())
        t.schedule(deadline: .now() + 2)
        t.setEventHandler { [weak self] in
            guard let self else { return }
            let bound = self.recognizer.withLock { !$0.config.bindings.isEmpty }
            let need = bound && self.lock.withLock { self.keyboard == nil }
            if need {
                self.install()
            } else {
                self.lock.withLock { self.retry = nil }
            }
        }
        lock.withLock { retry = t }
        t.resume()
    }

    /// Every 2 s while installed: Accessibility taken away leaves a tap that
    /// sees nothing; remove it, say so, and wait for it to come back.
    private func watchTrust() {
        let t = DispatchSource.makeTimerSource(queue: .global())
        t.schedule(deadline: .now() + 2, repeating: 2)
        t.setEventHandler { [weak self, weak t] in
            guard let self else { t?.cancel(); return }
            guard self.lock.withLock({ self.keyboard != nil }) else { t?.cancel(); return }
            if !AXIsProcessTrusted() {
                t?.cancel()
                self.uninstall()
                self.lock.withLock { self.lastError = "Accessibility was turned off; retrying every 2 s" }
                self.out.notify(Method.hotkeyState, self.status())
                self.scheduleRetry()
            }
        }
        t.resume()
    }

    /// Installs the button tap while a mouse button is bound and removes it
    /// when none is, on the taps' run loop (CFRunLoopAddSource may be called
    /// from any thread). Without the keyboard tap there is no run loop: it
    /// runs again once install() has one.
    private func syncButtonTap() {
        buttonSync.withLock { syncButtonTapLocked() }
    }

    private func syncButtonTapLocked() {
        let want = recognizer.withLock { $0.bindsMouse }
        let (loop, have) = lock.withLock { (runLoop, buttons != nil) }
        guard let loop, want != have else { return }
        guard want else {
            let (port, src) = lock.withLock { () -> (CFMachPort?, CFRunLoopSource?) in
                defer { buttons = nil; buttonSource = nil; buttonError = nil }
                return (buttons, buttonSource)
            }
            buttonTapOn.store(false, ordering: .relaxed)
            if let port { CGEvent.tapEnable(tap: port, enable: false); CFMachPortInvalidate(port) }
            if let src { CFRunLoopRemoveSource(loop, src, .commonModes) }
            return
        }
        let me = Unmanaged.passUnretained(self).toOpaque()
        let onButton: CGEventTapCallBack = { _, type, event, refcon in
            guard let refcon else { return Unmanaged.passUnretained(event) }
            let tap = Unmanaged<HotkeyTap>.fromOpaque(refcon).takeUnretainedValue()
            return tap.handleButton(type, event) ? nil : Unmanaged.passUnretained(event)
        }
        guard let port = CGEvent.tapCreate(tap: .cgSessionEventTap, place: .headInsertEventTap, options: .defaultTap,
                                           eventsOfInterest: TapPlan.buttonMask, callback: onButton, userInfo: me),
              let src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, port, 0) else {
            log("the mouse button tap could not be created: mouse shortcuts do not work")
            lock.withLock { buttonError = "CGEvent.tapCreate (active, middle and side buttons) failed" }
            return
        }
        lock.withLock {
            buttons = port
            buttonSource = src
            buttonError = nil
        }
        CFRunLoopAddSource(loop, src, .commonModes)
        buttonTapOn.store(true, ordering: .relaxed)
        CGEvent.tapEnable(tap: port, enable: true)
        CFRunLoopWakeUp(loop)
    }

    private func uninstall() {
        let (ports, loop, timers, r) = lock.withLock { () -> ([CFMachPort], CFRunLoop?, [CFRunLoopTimer], DispatchSourceTimer?) in
            defer {
                keyboard = nil
                pointer = nil
                pointerError = nil
                buttons = nil
                buttonSource = nil
                buttonError = nil
                runLoop = nil
                self.timers = []
                retry = nil
            }
            return ([keyboard, pointer, buttons].compactMap { $0 }, runLoop, self.timers, retry)
        }
        buttonTapOn.store(false, ordering: .relaxed)
        recognizer.withLock { $0.reset() }
        r?.cancel()
        for t in timers { CFRunLoopTimerInvalidate(t) }
        for port in ports {
            CGEvent.tapEnable(tap: port, enable: false)
            CFMachPortInvalidate(port)
        }
        if let loop { CFRunLoopStop(loop) }
    }

    /// The keyboard tap's callback, while macOS holds the event: true
    /// swallows it.
    private func handleKey(_ type: CGEventType, _ event: CGEvent) -> Bool {
        let start = clock_gettime_nsec_np(CLOCK_UPTIME_RAW)
        if TapPlan.isDisabled(type.rawValue) {
            disabled(type, keyboardStats, which: "keyboard")
            return false
        }
        let t = Double(start) / 1e6
        guard let input = TapPlan.keyInput(type: type.rawValue, keyCode: UInt16(truncatingIfNeeded: event.getIntegerValueField(.keyboardEventKeycode)),
                                           flags: event.flags.rawValue,
                                           isRepeat: type == .keyDown && event.getIntegerValueField(.keyboardEventAutorepeat) != 0, t: t) else {
            return false
        }
        keyboardStats.event()
        noteLag(event, keyboardStats)
        if InjectedEvent.isOurs(event.getIntegerValueField(.eventSourceUserData)) {
            keyboardStats.ownEvent()
            return false
        }
        if case .keyDown(KeyCode.space, _, false, _) = input, recognizer.withLock({ $0.spaceOpen }) {
            // Not the app the window opened on, or a password field: the
            // Space is the user's (asked only while the window is open).
            if IsSecureEventInputEnabled() || (front.cached?.pid ?? -1) != spaceFront.load(ordering: .relaxed) {
                recognizer.withLock { $0.closeSpace() }
            }
        }
        let (fired, swallow, deadline) = recognizer.withLock { r -> ([HotkeyEvent], Bool, Double?) in
            let res = r.handle(input)
            return (res.events, res.swallow, r.nextDeadline)
        }
        send(fired)
        arm(deadline)
        keyboardStats.callback(ns: Int(clock_gettime_nsec_np(CLOCK_UPTIME_RAW) &- start))
        return swallow
    }

    /// The pointer tap's callback: a copy of the event, macOS does not wait.
    private func handlePointer(_ type: CGEventType, _ event: CGEvent) {
        let start = clock_gettime_nsec_np(CLOCK_UPTIME_RAW)
        if TapPlan.isDisabled(type.rawValue) {
            disabled(type, pointerStats, which: "mouse")
            return
        }
        guard let input = TapPlan.pointerInput(type: type.rawValue, t: Double(start) / 1e6,
                                               buttonTap: buttonTapOn.load(ordering: .relaxed)) else { return }
        pointerStats.event()
        noteLag(event, pointerStats)
        let (fired, deadline) = recognizer.withLock { r -> ([HotkeyEvent], Double?) in
            (r.handle(input).events, r.nextDeadline)
        }
        send(fired)
        arm(deadline)
        pointerStats.callback(ns: Int(clock_gettime_nsec_np(CLOCK_UPTIME_RAW) &- start))
    }

    /// The button tap's callback, while macOS holds the event: true
    /// swallows it (a bound middle or side button).
    private func handleButton(_ type: CGEventType, _ event: CGEvent) -> Bool {
        let start = clock_gettime_nsec_np(CLOCK_UPTIME_RAW)
        if TapPlan.isDisabled(type.rawValue) {
            disabled(type, buttonStats, which: "button")
            return false
        }
        guard let input = TapPlan.buttonInput(type: type.rawValue, button: Int(event.getIntegerValueField(.mouseEventButtonNumber)),
                                              flags: event.flags.rawValue, t: Double(start) / 1e6) else { return false }
        buttonStats.event()
        noteLag(event, buttonStats)
        let (fired, swallow, deadline) = recognizer.withLock { r -> ([HotkeyEvent], Bool, Double?) in
            let res = r.handle(input)
            return (res.events, res.swallow, r.nextDeadline)
        }
        send(fired)
        arm(deadline)
        buttonStats.callback(ns: Int(clock_gettime_nsec_np(CLOCK_UPTIME_RAW) &- start))
        return swallow
    }

    /// macOS disabled a tap (too slow, or secure input): count it, forget
    /// the chord in progress, enable it again.
    private func disabled(_ type: CGEventType, _ stats: TapStats, which: String) {
        stats.disabledOnce(at: monotonicMs())
        recognizer.withLock { $0.reset() }
        let port = lock.withLock { which == "keyboard" ? keyboard : which == "button" ? buttons : pointer }
        if let port { CGEvent.tapEnable(tap: port, enable: true) }
        log("the \(which) event tap was disabled (\(type.rawValue)); enabled again")
    }

    /// The time from the event to its callback (paging in, a busy thread).
    @inline(__always)
    private func noteLag(_ event: CGEvent, _ stats: TapStats) {
        guard let clock else { return }
        let now: UInt64 = switch clock.source {
        case .uptimeNs: clock_gettime_nsec_np(CLOCK_UPTIME_RAW)
        case .absoluteTicks: mach_absolute_time()
        case .continuousNs: clock_gettime_nsec_np(CLOCK_MONOTONIC_RAW)
        case .continuousTicks: mach_continuous_time()
        }
        if let ns = clock.lagNs(timestamp: event.timestamp, now: now) { stats.lag(ns: ns) }
    }

    /// Sends what fired, with the app in front (as cached at this moment).
    private func send(_ fired: [HotkeyEvent]) {
        guard !fired.isEmpty else { return }
        let app = front.cached
        sendQueue.async { [out] in
            for e in fired { out.notify(Method.hotkeyEvent, HotkeyEventParams(e, front: app)) }
        }
    }

    /// Moves the deadline timer to the recognizer's next deadline, when it
    /// changed (on the tap's thread; the timer is the run loop's).
    @inline(__always)
    private func arm(_ deadline: Double?) {
        guard deadline != armed, let timer = deadlineTimer else { return }
        armed = deadline
        let delay = deadline.map { max(0, $0 - monotonicMs()) / 1000 } ?? 1e9
        CFRunLoopTimerSetNextFireDate(timer, CFAbsoluteTimeGetCurrent() + delay)
    }
}

/// The system's double-click interval in ms, at most 800 (else 500): the
/// default double-tap window. (NSEvent.doubleClickInterval without
/// AppKit's main thread.)
func systemDoubleClickMs() -> Double {
    let v = UserDefaults.standard.double(forKey: "com.apple.mouse.doubleClickThreshold")
    return v > 0 ? min(v, 0.8) * 1000 : 500
}
