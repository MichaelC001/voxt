@testable import VoxtMiniHelperKit
import XCTest
#if canImport(CoreGraphics)
import CoreGraphics
#endif

// The taps' pure half (HotkeyTapKit.swift): what goes to the active tap and
// what to the listen-only one, the counters, the event clock, the warm-up.
final class HotkeyTapKitTests: XCTestCase {
    /// Mouse presses never reach the active tap: macOS must not wait on
    /// the helper for a click (incident 2026-11-12).
    func testMousePressesOnlyInTheListenOnlyTap() {
        for t in [TapEvent.leftMouseDown, TapEvent.rightMouseDown, TapEvent.otherMouseDown] {
            XCTAssertEqual(TapPlan.keyboardMask & TapPlan.bit(t), 0)
            XCTAssertNotEqual(TapPlan.pointerMask & TapPlan.bit(t), 0)
        }
        XCTAssertEqual(TapPlan.keyboardMask & TapPlan.pointerMask, 0)
        XCTAssertEqual(TapPlan.keyboardMask, 1 << 10 | 1 << 11 | 1 << 12)
        XCTAssertEqual(TapPlan.pointerMask, 1 << 1 | 1 << 3 | 1 << 25)
    }

    /// The button tap (active, only with a mouse binding) has the middle and
    /// side buttons, never a left or right click.
    func testButtonTapNeverHasLeftOrRightClicks() {
        XCTAssertEqual(TapPlan.buttonMask, 1 << 25 | 1 << 26)
        XCTAssertEqual(TapPlan.buttonMask & (TapPlan.bit(TapEvent.leftMouseDown) | TapPlan.bit(TapEvent.rightMouseDown)), 0)
        XCTAssertEqual(TapPlan.buttonMask & TapPlan.keyboardMask, 0)
        // With it, the pointer tap leaves the other buttons to it.
        XCTAssertNil(TapPlan.pointerInput(type: TapEvent.otherMouseDown, t: 1, buttonTap: true))
        guard case .pointer(1)? = TapPlan.pointerInput(type: TapEvent.leftMouseDown, t: 1, buttonTap: true) else { return XCTFail("left") }
        guard case .mouseButton(2, true, 0x20000, 3)? = TapPlan.buttonInput(type: TapEvent.otherMouseDown, button: 2, flags: 0x20000, t: 3) else {
            return XCTFail("middle down")
        }
        guard case .mouseButton(4, false, 0, 4)? = TapPlan.buttonInput(type: TapEvent.otherMouseUp, button: 4, flags: 0, t: 4) else {
            return XCTFail("forward up")
        }
        XCTAssertNil(TapPlan.buttonInput(type: TapEvent.otherMouseDown, button: 1, flags: 0, t: 0))
        XCTAssertNil(TapPlan.buttonInput(type: TapEvent.keyDown, button: 2, flags: 0, t: 0))
    }

    #if canImport(CoreGraphics)
    func testEventTypesAreCoreGraphics() {
        XCTAssertEqual(TapEvent.leftMouseDown, CGEventType.leftMouseDown.rawValue)
        XCTAssertEqual(TapEvent.rightMouseDown, CGEventType.rightMouseDown.rawValue)
        XCTAssertEqual(TapEvent.otherMouseDown, CGEventType.otherMouseDown.rawValue)
        XCTAssertEqual(TapEvent.otherMouseUp, CGEventType.otherMouseUp.rawValue)
        XCTAssertEqual(TapEvent.keyDown, CGEventType.keyDown.rawValue)
        XCTAssertEqual(TapEvent.keyUp, CGEventType.keyUp.rawValue)
        XCTAssertEqual(TapEvent.flagsChanged, CGEventType.flagsChanged.rawValue)
        XCTAssertEqual(TapEvent.disabledByTimeout, CGEventType.tapDisabledByTimeout.rawValue)
        XCTAssertEqual(TapEvent.disabledByUserInput, CGEventType.tapDisabledByUserInput.rawValue)
    }
    #endif

    func testInputs() {
        guard case .keyDown(9, 0x40000, true, 5)? = TapPlan.keyInput(type: TapEvent.keyDown, keyCode: 9, flags: 0x40000, isRepeat: true, t: 5) else {
            return XCTFail("keyDown")
        }
        guard case .flagsChanged(61, 0x80140, 6)? = TapPlan.keyInput(type: TapEvent.flagsChanged, keyCode: 61, flags: 0x80140, isRepeat: false, t: 6) else {
            return XCTFail("flags")
        }
        guard case .keyUp(9, 0, 7)? = TapPlan.keyInput(type: TapEvent.keyUp, keyCode: 9, flags: 0, isRepeat: false, t: 7) else {
            return XCTFail("keyUp")
        }
        XCTAssertNil(TapPlan.keyInput(type: TapEvent.leftMouseDown, keyCode: 0, flags: 0, isRepeat: false, t: 0))
        XCTAssertNil(TapPlan.keyInput(type: TapEvent.disabledByTimeout, keyCode: 0, flags: 0, isRepeat: false, t: 0))
        for t in [TapEvent.leftMouseDown, TapEvent.rightMouseDown, TapEvent.otherMouseDown] {
            guard case .pointer(8)? = TapPlan.pointerInput(type: t, t: 8) else { return XCTFail("pointer \(t)") }
        }
        XCTAssertNil(TapPlan.pointerInput(type: TapEvent.keyDown, t: 0))
        XCTAssertTrue(TapPlan.isDisabled(TapEvent.disabledByTimeout))
        XCTAssertTrue(TapPlan.isDisabled(TapEvent.disabledByUserInput))
        XCTAssertFalse(TapPlan.isDisabled(TapEvent.keyDown))
    }

    /// A click from the listen-only tap still spoils an ⌥ chord: the same
    /// recognizer input as before the split.
    func testPointerTapSpoilsTheChord() {
        var r = HotkeyRecognizer(config: HotkeyConfig(bindings: [Binding(id: "t", modifiers: [.option: .right], behavior: .tap)]))
        let rOpt = RawFlags.option | 0x40
        _ = r.handle(TapPlan.keyInput(type: TapEvent.flagsChanged, keyCode: 61, flags: rOpt, isRepeat: false, t: 0)!)
        _ = r.handle(TapPlan.pointerInput(type: TapEvent.leftMouseDown, t: 40)!)
        XCTAssertEqual(r.handle(.flagsChanged(keyCode: 61, flags: 0, t: 90)).events, [])
        _ = r.handle(.flagsChanged(keyCode: 61, flags: rOpt, t: 200))
        XCTAssertEqual(r.handle(.flagsChanged(keyCode: 61, flags: 0, t: 260)).events.map(\.binding), ["t"])
    }

    func testStats() {
        let s = TapStats()
        XCTAssertNil(s.maxLagMs)
        XCTAssertEqual(s.status(installed: false, error: nil),
                       HotkeyTapStatus(installed: false, events: 0, disabledByTimeout: 0, maxCallbackMicros: 0))
        DispatchQueue.concurrentPerform(iterations: 8) { i in
            for j in 0..<1000 {
                s.event()
                s.callback(ns: i * 1000 + j)
                s.lag(ns: (i * 1000 + j) * 1000)
            }
        }
        XCTAssertNil(s.disabledAgoMs(now: 100))
        s.disabledOnce(at: 1000.25)
        XCTAssertEqual(s.disabledAgoMs(now: 1500.5), 500.3)
        XCTAssertEqual(s.ownCount, 0)
        s.ownEvent()
        s.ownEvent()
        XCTAssertEqual(s.ownCount, 2)
        XCTAssertEqual(s.eventCount, 8000)
        XCTAssertEqual(s.maxCallbackMicros, 8.0) // 7999 ns
        XCTAssertEqual(s.maxLagMs, 8.0) // 7.999 ms
        XCTAssertEqual(s.status(installed: true, error: "x"),
                       HotkeyTapStatus(installed: true, events: 8000, disabledByTimeout: 1, maxCallbackMicros: 8.0, maxLagMs: 8.0, error: "x"))
    }

    func testEventClock() {
        // Apple silicon: 24 MHz ticks, 125/3 ns each.
        let now: [EventClock.Source: UInt64] = [.uptimeNs: 5_000_000_000_000, .absoluteTicks: 120_000_000_000,
                                                .continuousNs: 9_000_000_000_000, .continuousTicks: 216_000_000_000]
        let ticks = EventClock.pick(timestamp: 119_999_990_000, now: now, numer: 125, denom: 3)
        XCTAssertEqual(ticks, EventClock(source: .absoluteTicks, numer: 125, denom: 3))
        XCTAssertEqual(ticks?.lagNs(timestamp: 120_000_000_000 - 24_000, now: 120_000_000_000), 1_000_000) // 1 ms
        XCTAssertEqual(EventClock.pick(timestamp: 5_000_000_100_000, now: now, numer: 125, denom: 3)?.source, .uptimeNs)
        XCTAssertEqual(EventClock.pick(timestamp: 8_999_990_000_000, now: now, numer: 125, denom: 3)?.source, .continuousNs)
        XCTAssertNil(EventClock.pick(timestamp: 0, now: now, numer: 125, denom: 3))
        XCTAssertNil(EventClock.pick(timestamp: 42, now: [:], numer: 1, denom: 1))
        let ns = EventClock(source: .uptimeNs, numer: 1, denom: 1)
        XCTAssertNil(ns.lagNs(timestamp: 11, now: 10)) // ahead: no lag
        XCTAssertEqual(ns.lagNs(timestamp: 11, now: 10, allowNegative: true), -1)
        XCTAssertNil(ns.lagNs(timestamp: 0, now: 61_000_000_000)) // a minute: not this clock
        XCTAssertNil(EventClock(source: .absoluteTicks, numer: UInt64.max, denom: 1).lagNs(timestamp: 0, now: 3)) // overflow
    }

    /// The warm-up runs on a copy: nothing fires, the real recognizer's
    /// chord in progress and deferred tap are untouched.
    func testWarmUpTouchesOnlyACopy() {
        let c = HotkeyConfig(bindings: [
            Binding(id: "t", modifiers: [.option: .right], behavior: .tap),
            Binding(id: "d", modifiers: [.option: .right], behavior: .doubleTap),
            Binding(id: "s", modifiers: [.shift: .any], behavior: .tap),
            Binding(id: "k", modifiers: [.shift: .any], key: 0x7F, behavior: .tap, swallow: false),
        ])
        var r = HotkeyRecognizer(config: c)
        let rOpt = RawFlags.option | 0x40
        _ = r.handle(.flagsChanged(keyCode: 61, flags: rOpt, t: 0))
        _ = r.handle(.flagsChanged(keyCode: 61, flags: 0, t: 50))
        XCTAssertEqual(r.nextDeadline, 550)
        var copy = r
        XCTAssertEqual(TapPlan.warmUp(&copy, now: 100), 1) // k fires on the copy only
        XCTAssertEqual(r.nextDeadline, 550)
        XCTAssertEqual(r.tick(now: 550).map(\.binding), ["t"])
        var plain = HotkeyRecognizer(config: HotkeyConfig(bindings: [Binding(id: "t", modifiers: [.option: .right], behavior: .tap)]))
        XCTAssertEqual(TapPlan.warmUp(&plain, now: 0), 0)
    }

    func testStatusJSON() throws {
        let st = HotkeyStatus(installed: true, accessibility: true, bindings: 4, events: 12, disabledByTimeout: 0, missedEvents: 0,
                              maxCallbackMicros: 21.4, maxLagMs: 0.6,
                              pointer: HotkeyTapStatus(installed: true, events: 3, disabledByTimeout: 0, maxCallbackMicros: 4.2), error: nil)
        let json = String(decoding: try makeEncoder().encode(st), as: UTF8.self)
        XCTAssertTrue(json.contains(#""maxLagMs":0.6"#), json)
        XCTAssertTrue(json.contains(#""pointer":{"#), json)
        let back = try JSONDecoder().decode(HotkeyStatus.self, from: Data(json.utf8))
        XCTAssertEqual(back.pointer, st.pointer)
        let old = try JSONDecoder().decode(HotkeyStatus.self, from: Data(
            #"{"installed":true,"accessibility":true,"bindings":6,"events":4,"disabledByTimeout":0,"missedEvents":0,"maxCallbackMicros":38.5}"#.utf8))
        XCTAssertNil(old.pointer)
        XCTAssertNil(old.maxLagMs)
    }
}
