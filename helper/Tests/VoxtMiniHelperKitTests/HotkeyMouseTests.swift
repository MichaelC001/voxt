@testable import VoxtMiniHelperKit
import XCTest

// Mouse bindings (the middle and side buttons) and the Space window.
final class HotkeyMouseTests: XCTestCase {
    let shift = RawFlags.shift | 0x2
    let ropt = RawFlags.option | 0x40

    func names(_ es: [HotkeyEvent]) -> [String] { es.map { "\($0.binding):\($0.phase.rawValue)" } }

    func config(_ bindings: [Binding], active: Bool = false, space: Bool = false) -> HotkeyConfig {
        HotkeyConfig(bindings: bindings, doubleTapMs: 400, longPressMs: 300, tapMaxMs: 1000, active: active, spaceFollowUp: space)
    }

    let middleTap = Binding(id: "transcribe", modifiers: [:], mouseButton: 2, behavior: .tap)
    let middleDouble = Binding(id: "rewrite", modifiers: [:], mouseButton: 2, behavior: .doubleTap)

    func testMiddleTapFiresOnPressAndSwallowsBoth() {
        var r = HotkeyRecognizer(config: config([middleTap]))
        XCTAssertTrue(r.bindsMouse)
        let down = r.handle(.mouseButton(button: 2, down: true, flags: 0, t: 0))
        XCTAssertEqual(names(down.events), ["transcribe:trigger"])
        XCTAssertTrue(down.swallow)
        let up = r.handle(.mouseButton(button: 2, down: false, flags: 0, t: 60))
        XCTAssertTrue(up.events.isEmpty)
        XCTAssertTrue(up.swallow)
    }

    func testOtherButtonsAndModifiersPass() {
        var r = HotkeyRecognizer(config: config([middleTap, Binding(id: "translate", modifiers: [.shift: .any], mouseButton: 3, behavior: .tap)]))
        // The back button without ⇧, the forward button, ⇧ middle: not bound.
        for (b, f) in [(3, UInt64(0)), (4, 0), (2, shift)] {
            let res = r.handle(.mouseButton(button: b, down: true, flags: f, t: 0))
            XCTAssertTrue(res.events.isEmpty, "button \(b)")
            XCTAssertFalse(res.swallow, "button \(b)")
            XCTAssertFalse(r.handle(.mouseButton(button: b, down: false, flags: f, t: 10)).swallow)
        }
        XCTAssertEqual(names(r.handle(.mouseButton(button: 3, down: true, flags: shift, t: 20)).events), ["translate:trigger"])
        XCTAssertFalse(HotkeyRecognizer(config: config([Binding(id: "t", modifiers: [.fn: .any], behavior: .tap)])).bindsMouse)
    }

    func testLongPressDownAndUp() {
        var r = HotkeyRecognizer(config: config([Binding(id: "transcribe", modifiers: [:], mouseButton: 4, behavior: .longPress)]))
        XCTAssertEqual(names(r.handle(.mouseButton(button: 4, down: true, flags: 0, t: 0)).events), ["transcribe:down"])
        let up = r.handle(.mouseButton(button: 4, down: false, flags: 0, t: 900))
        XCTAssertEqual(names(up.events), ["transcribe:up"])
        XCTAssertEqual(up.events.first?.heldMs, 900)
        XCTAssertTrue(up.swallow)
    }

    // Voxt's middle-button preset: a click transcribes after the double
    // click window, a double click rewrites (and no transcribe).
    func testTapAndDoubleTapOnOneButton() {
        var r = HotkeyRecognizer(config: config([middleTap, middleDouble]))
        XCTAssertTrue(r.handle(.mouseButton(button: 2, down: true, flags: 0, t: 0)).events.isEmpty)
        _ = r.handle(.mouseButton(button: 2, down: false, flags: 0, t: 50))
        XCTAssertEqual(r.nextDeadline, 400)
        XCTAssertTrue(r.tick(now: 399).isEmpty)
        XCTAssertEqual(names(r.tick(now: 400)), ["transcribe:trigger"])

        var d = HotkeyRecognizer(config: config([middleTap, middleDouble]))
        _ = d.handle(.mouseButton(button: 2, down: true, flags: 0, t: 1000))
        _ = d.handle(.mouseButton(button: 2, down: false, flags: 0, t: 1050))
        XCTAssertEqual(names(d.handle(.mouseButton(button: 2, down: true, flags: 0, t: 1200)).events), ["rewrite:trigger"])
        XCTAssertTrue(d.tick(now: 2000).isEmpty)
        XCTAssertNil(d.nextDeadline)
    }

    func testActiveTapFiresAtOnce() {
        var r = HotkeyRecognizer(config: config([middleTap, middleDouble], active: true))
        XCTAssertEqual(names(r.handle(.mouseButton(button: 2, down: true, flags: 0, t: 0)).events), ["transcribe:trigger"])
        XCTAssertNil(r.nextDeadline)
    }

    func testMousePressSpoilsAChord() {
        var r = HotkeyRecognizer(config: config([middleTap, Binding(id: "ropt", modifiers: [.option: .right], behavior: .tap)]))
        _ = r.handle(.flagsChanged(keyCode: 61, flags: ropt, t: 0))
        _ = r.handle(.mouseButton(button: 4, down: true, flags: ropt, t: 30))
        _ = r.handle(.mouseButton(button: 4, down: false, flags: ropt, t: 60))
        XCTAssertTrue(r.handle(.flagsChanged(keyCode: 61, flags: 0, t: 90)).events.isEmpty)
    }

    // MARK: the Space window

    func testSpaceOnceWhileOpen() {
        var r = HotkeyRecognizer(config: config([middleTap], active: true, space: true))
        XCTAssertTrue(r.spaceOpen)
        let down = r.handle(.keyDown(keyCode: KeyCode.space, flags: 0, isRepeat: false, t: 0))
        XCTAssertEqual(names(down.events), ["space:trigger"])
        XCTAssertTrue(down.swallow)
        // Its repeats and its release are swallowed too; then Space is the user's.
        XCTAssertTrue(r.handle(.keyDown(keyCode: KeyCode.space, flags: 0, isRepeat: true, t: 500)).swallow)
        XCTAssertTrue(r.handle(.keyUp(keyCode: KeyCode.space, flags: 0, t: 600)).swallow)
        XCTAssertFalse(r.spaceOpen)
        let again = r.handle(.keyDown(keyCode: KeyCode.space, flags: 0, isRepeat: false, t: 700))
        XCTAssertTrue(again.events.isEmpty)
        XCTAssertFalse(again.swallow)
    }

    func testOtherKeysClicksAndModifiedSpaceCloseTheWindow() {
        let closers: [KeyInput] = [
            .keyDown(keyCode: 0, flags: 0, isRepeat: false, t: 0), // A
            .pointer(t: 0),
            .mouseButton(button: 3, down: true, flags: 0, t: 0),
            .keyDown(keyCode: KeyCode.space, flags: shift, isRepeat: false, t: 0), // ⇧ Space
            .keyDown(keyCode: KeyCode.space, flags: RawFlags.secondaryFn, isRepeat: false, t: 0), // fn Space
            .keyDown(keyCode: KeyCode.space, flags: 0, isRepeat: true, t: 0), // a repeat of a Space held before
        ]
        for input in closers {
            var r = HotkeyRecognizer(config: config([middleTap], space: true))
            XCTAssertFalse(r.handle(input).events.contains { $0.binding == "space" }, "\(input)")
            let res = r.handle(.keyDown(keyCode: KeyCode.space, flags: 0, isRepeat: false, t: 10))
            XCTAssertTrue(res.events.isEmpty, "after \(input)")
            XCTAssertFalse(res.swallow, "after \(input)")
        }
        // Modifiers alone (a shortcut on its way) leave it open.
        var r = HotkeyRecognizer(config: config([middleTap], space: true))
        _ = r.handle(.flagsChanged(keyCode: 56, flags: shift, t: 0))
        _ = r.handle(.flagsChanged(keyCode: 56, flags: 0, t: 50))
        XCTAssertEqual(names(r.handle(.keyDown(keyCode: KeyCode.space, flags: 0, isRepeat: false, t: 100)).events), ["space:trigger"])
    }

    func testWindowOpensOnlyOnTheRisingEdge() {
        var r = HotkeyRecognizer(config: config([middleTap], space: true))
        r.closeSpace()
        // The same window sent again (another setting changed): still shut.
        r.configure(config([middleTap], active: true, space: true))
        XCTAssertFalse(r.spaceOpen)
        r.configure(config([middleTap], space: false))
        XCTAssertFalse(r.spaceOpen)
        r.configure(config([middleTap], space: true))
        XCTAssertTrue(r.spaceOpen)
        XCTAssertFalse(HotkeyRecognizer(config: config([middleTap])).spaceOpen)
    }
}
