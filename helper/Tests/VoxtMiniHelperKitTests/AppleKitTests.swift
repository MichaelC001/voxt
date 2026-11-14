import Foundation
import XCTest
@testable import VoxtMiniHelperKit

/// The pure halves of the Apple models (AppleKit.swift).
final class AppleKitTests: XCTestCase {
    func testSpeechTextAnalyzer() {
        var t = SpeechText()
        t.partial("Hello")
        XCTAssertEqual(t.text, "Hello")
        t.partial("Hello wor")
        t.final("Hello world.")
        XCTAssertEqual(t.text, "Hello world.")
        t.partial("How are")
        XCTAssertEqual(t.text, "Hello world. How are")
        t.final("How are you?")
        XCTAssertEqual(t.text, "Hello world. How are you?")
        var z = SpeechText()
        z.final("你好，")
        z.partial("世界")
        XCTAssertEqual(z.text, "你好，世界")
        z.final("世界。VoxtMini")
        z.partial("いいね")
        XCTAssertEqual(z.text, "你好，世界。VoxtMiniいいね") // no space next to CJK text
    }

    func testSpeechTextRecognizer() {
        var t = SpeechText()
        t.whole("Hello")
        t.whole("Hello, world")
        XCTAssertEqual(t.text, "Hello, world")
        t.whole("")
        XCTAssertEqual(t.text, "")
    }

    func testPCM16() {
        let data = Data([0x00, 0x00, 0x00, 0x40, 0x00, 0x80, 0xFF, 0x7F, 0x01])
        let f = PCM16.floats(data)
        XCTAssertEqual(f.count, 4)
        XCTAssertEqual(f[0], 0)
        XCTAssertEqual(f[1], 0.5)
        XCTAssertEqual(f[2], -1)
        XCTAssertEqual(f[3], 32767.0 / 32768, accuracy: 1e-6)
        // Unaligned: a slice that starts at an odd byte.
        let odd = Data([0x55] + [0x00, 0x40]).dropFirst()
        XCTAssertEqual(PCM16.floats(Data(odd)), [0.5])
        XCTAssertEqual(PCM16.floats(odd), [0.5])
    }

    func testChatPlan() throws {
        let m = { (r: String, c: String) in AppleMessage(role: r, content: c) }
        let plan = try ChatPlan([m("system", "Clean up."), m("user", "example in"), m("assistant", "Example out."), m("user", "um hello")])
        XCTAssertEqual(plan.instructions, "Clean up.")
        XCTAssertEqual(plan.turns, [m("user", "example in"), m("assistant", "Example out.")])
        XCTAssertEqual(plan.prompt, "um hello")
        let bare = try ChatPlan([m("user", "hi")])
        XCTAssertEqual(bare.instructions, "")
        XCTAssertEqual(bare.turns, [])
        XCTAssertThrowsError(try ChatPlan([m("system", "x")]))
        XCTAssertThrowsError(try ChatPlan([m("tool", "x"), m("user", "y")]))
    }

    func testJobs() {
        for m in [Method.captureStart, Method.appleInstall, Method.appleSpeech, Method.appleChat] {
            XCTAssertTrue(Method.isJob(m), m)
        }
        XCTAssertFalse(Method.isJob(Method.appleSpeechAudio))
        XCTAssertTrue(Code.all.contains(Code.contextLength) && Code.all.contains(Code.refused))
    }
}
