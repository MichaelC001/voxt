import XCTest
@testable import VoxtMiniHelperKit

/// textio.paste's check: a change in what both readings tell is an
/// insert; nothing to compare is unknown, never unchanged.
final class PasteCheckTests: XCTestCase {
    private let caret0 = TextRange(location: 0, length: 0)

    func testCompare() {
        let empty = FieldReading(length: 0, digest: PasteChecker.digest(""), selection: caret0)
        let typed = FieldReading(length: 2, digest: PasteChecker.digest("Hi"), selection: TextRange(location: 2, length: 0))
        XCTAssertEqual(PasteChecker.compare(empty, typed), .changed)
        XCTAssertEqual(PasteChecker.compare(empty, empty), .same)

        // A selection replaced by text of the same length: the value's digest.
        let ab = FieldReading(length: 2, digest: PasteChecker.digest("ab"), selection: TextRange(location: 0, length: 2))
        let cd = FieldReading(length: 2, digest: PasteChecker.digest("cd"), selection: TextRange(location: 0, length: 2))
        XCTAssertEqual(PasteChecker.compare(ab, cd), .changed)

        // A web editor with a caret and no value: the caret moves.
        let caret = FieldReading(length: nil, selection: TextRange(location: 5, length: 0))
        XCTAssertEqual(PasteChecker.compare(caret, FieldReading(length: nil, selection: TextRange(location: 9, length: 0))), .changed)
        XCTAssertEqual(PasteChecker.compare(caret, caret), .same)

        // A web area: only the caret's text marker.
        let page = FieldReading(length: nil, selection: nil, marker: 11)
        XCTAssertTrue(page.readable)
        XCTAssertEqual(PasteChecker.compare(page, FieldReading(length: nil, selection: nil, marker: 12)), .changed)
        XCTAssertEqual(PasteChecker.compare(page, page), .same, "a page that ignored ⌘V")
        XCTAssertEqual(page.state, FieldState(length: 0, selection: nil, marker: true))

        // Nothing to compare.
        let blank = FieldReading(length: nil, selection: nil)
        XCTAssertEqual(PasteChecker.compare(blank, blank), .unreadable)
        XCTAssertEqual(PasteChecker.compare(empty, nil), .unreadable, "the element is gone")
        XCTAssertEqual(PasteChecker.compare(FieldReading(length: 3, selection: nil), FieldReading(length: nil, selection: caret0)), .unreadable,
                       "no reading in common")
    }

    func testOutcome() {
        let empty = FieldReading(length: 0, selection: caret0)
        XCTAssertEqual(PasteChecker.outcome(sawChange: true, before: empty, last: empty, focusMoved: false), PasteCheck.inserted)
        XCTAssertEqual(PasteChecker.outcome(sawChange: false, before: empty, last: empty, focusMoved: false), PasteCheck.unchanged)
        XCTAssertEqual(PasteChecker.outcome(sawChange: false, before: empty, last: empty, focusMoved: true), PasteCheck.unknown,
                       "another element has the focus: the text may be there")
        XCTAssertEqual(PasteChecker.outcome(sawChange: false, before: FieldReading(length: nil, selection: nil), last: nil, focusMoved: false),
                       PasteCheck.unknown, "the element tells nothing")
    }

    func testState() {
        XCTAssertNil(FieldReading(length: nil, selection: nil).state)
        XCTAssertEqual(FieldReading(length: nil, selection: caret0).state, FieldState(length: 0, selection: caret0))
        XCTAssertEqual(FieldReading(length: 7, digest: 1, selection: nil).state, FieldState(length: 7, selection: nil))
    }

    /// The second ⌘V only when nothing happened since the first.
    func testRetryBlock() {
        let field = FieldReading(length: 217, digest: 7, selection: TextRange(location: 217, length: 0))
        func block(now: FieldReading? = nil, same: Bool = true, front: Bool = true, ours: Bool = true, secure: Bool = false) -> String? {
            PasteChecker.retryBlock(before: field, now: now ?? field, sameElement: same, targetInFront: front, clipboardOurs: ours, secureInput: secure)
        }
        XCTAssertNil(block(), "the same element, reading as before: ⌘V again")
        XCTAssertEqual(block(same: false), PasteRetrySkip.focusMoved)
        XCTAssertEqual(block(front: false), PasteRetrySkip.notFront)
        XCTAssertEqual(block(ours: false), PasteRetrySkip.clipboard, "someone copied meanwhile: the clipboard is not our text")
        XCTAssertEqual(block(secure: true), PasteRetrySkip.secureInput)
        XCTAssertEqual(block(now: FieldReading(length: nil, selection: nil)), PasteRetrySkip.unreadable, "a reading that failed: no guess")
        XCTAssertEqual(PasteChecker.retryBlock(before: field, now: nil, sameElement: true, targetInFront: true, clipboardOurs: true, secureInput: false),
                       PasteRetrySkip.unreadable)
        let web = FieldReading(length: nil, selection: nil, marker: 3)
        XCTAssertNil(PasteChecker.retryBlock(before: web, now: web, sameElement: true, targetInFront: true, clipboardOurs: true, secureInput: false),
                     "a web area whose caret did not move")
    }

    func testModifierNames() {
        XCTAssertEqual(PasteChecker.modifierNames(0x100), [])
        XCTAssertEqual(PasteChecker.modifierNames(RawFlags.option | 0x40), ["option"], "right ⌥ still down")
        XCTAssertEqual(PasteChecker.modifierNames(RawFlags.command | RawFlags.shift | RawFlags.control | RawFlags.secondaryFn),
                       ["command", "shift", "control", "fn"])
    }
}
