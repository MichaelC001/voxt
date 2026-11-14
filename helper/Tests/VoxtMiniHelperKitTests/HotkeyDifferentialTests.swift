@testable import VoxtMiniHelperKit
import XCTest

// The recognizer's hot path was rewritten on bits (no allocation per
// event); its semantics must not change. Random event streams — chords on
// both sides, missed releases, synthetic flags, typing, mouse presses,
// the 🌐 key, Escape, key bindings, config changes, timer ticks — go to the
// new recognizer and to the old one (ReferenceRecognizer) and must give
// the same events, swallows, deadlines and counters at every step.
final class HotkeyDifferentialTests: XCTestCase {
    /// SplitMix64: the same streams on every run.
    struct Rng {
        var state: UInt64
        mutating func next() -> UInt64 {
            state &+= 0x9E37_79B9_7F4A_7C15
            var z = state
            z = (z ^ (z >> 30)) &* 0xBF58_476D_1CE4_E5B9
            z = (z ^ (z >> 27)) &* 0x94D0_49BB_1331_11EB
            return z ^ (z >> 31)
        }
        mutating func int(_ n: Int) -> Int { Int(next() % UInt64(n)) }
        mutating func chance(_ p: Double) -> Bool { Double(next() % 10000) / 10000 < p }
    }

    static let configs: [HotkeyConfig] = [
        HotkeyConfig(bindings: [
            Binding(id: "transcribe", modifiers: [.fn: .any], behavior: .tap),
            Binding(id: "translate", modifiers: [.fn: .any, .shift: .any], behavior: .tap),
            Binding(id: "rewrite", modifiers: [.fn: .any, .control: .any], behavior: .tap),
            Binding(id: "dbl", modifiers: [.fn: .any], behavior: .doubleTap),
        ], doubleTapMs: 400, longPressMs: 300, tapMaxMs: 1000, swallowFn: true),
        HotkeyConfig(bindings: [
            Binding(id: "transcribe/0", modifiers: [.option: .right], behavior: .tap),
            Binding(id: "translate/0", modifiers: [.option: .right, .shift: .any], behavior: .tap),
            Binding(id: "rewrite/0", modifiers: [.control: .any, .option: .right], behavior: .tap),
            Binding(id: "paste-last", modifiers: [.control: .any, .option: .any], key: 9, behavior: .tap),
        ]),
        HotkeyConfig(bindings: [
            Binding(id: "ptt", modifiers: [.command: .right], behavior: .longPress),
            Binding(id: "lc", modifiers: [.command: .left, .shift: .left], behavior: .tap),
            Binding(id: "fnr", modifiers: [.fn: .right], behavior: .tap),
            Binding(id: "fnl", modifiers: [.fn: .left, .option: .any], behavior: .doubleTap),
            Binding(id: "k-long", modifiers: [.command: .any], key: 40, behavior: .longPress),
            Binding(id: "k-dbl", modifiers: [:], key: 96, behavior: .doubleTap, swallow: false),
            Binding(id: "k-tap", modifiers: [.control: .any, .shift: .any], key: 0, behavior: .tap),
            Binding(id: "k-high", modifiers: [:], key: 300, behavior: .tap),
        ], doubleTapMs: 500, longPressMs: 250, tapMaxMs: 1500),
        HotkeyConfig(bindings: [
            Binding(id: "t", modifiers: [.fn: .any], behavior: .longPress),
            Binding(id: "tr", modifiers: [.fn: .any, .shift: .any], behavior: .tap),
            Binding(id: "d", modifiers: [.fn: .any, .shift: .any], behavior: .doubleTap),
            Binding(id: "rc", modifiers: [.command: .right, .option: .left], behavior: .doubleTap),
        ], swallowFn: true, active: true),
    ]

    /// Modifier keys with their keycodes, family flags and device bits.
    static let mods: [(code: UInt16, family: UInt64, device: UInt64)] = [
        (63, RawFlags.secondaryFn, 0),
        (56, RawFlags.shift, 0x2), (60, RawFlags.shift, 0x4),
        (59, RawFlags.control, 0x1), (62, RawFlags.control, 0x2000),
        (58, RawFlags.option, 0x20), (61, RawFlags.option, 0x40),
        (55, RawFlags.command, 0x8), (54, RawFlags.command, 0x10),
    ]
    static let keys: [UInt16] = [0, 9, 40, 96, 53, 179, 124, 57, 300, 12]

    func flags(_ down: Set<Int>, synthetic: Bool) -> UInt64 {
        var f: UInt64 = 0x100
        for i in down {
            f |= Self.mods[i].family
            if !synthetic { f |= Self.mods[i].device }
        }
        return f
    }

    func testMatchesReferenceOnRandomStreams() {
        continueAfterFailure = false
        var rng = Rng(state: 0x564F_5854)
        var steps = 0
        for run in 0..<400 {
            var config = Self.configs[run % Self.configs.count]
            var new = HotkeyRecognizer(config: config)
            var old = ReferenceRecognizer(config: config)
            var down = Set<Int>()
            var t = Double(rng.int(1000))
            var trace: [String] = []
            for _ in 0..<(40 + rng.int(160)) {
                t += [0, 1, 15, 60, 120, 250, 380, 450, 700, 1200, 1700][rng.int(11)]
                let synthetic = rng.chance(0.1)
                let input: KeyInput?
                switch rng.int(100) {
                case 0..<45:
                    let i = rng.int(Self.mods.count)
                    if rng.chance(0.04) {
                        // macOS dropped an event: this one repeats a state.
                    } else if down.contains(i) {
                        down.remove(i)
                    } else {
                        down.insert(i)
                    }
                    input = .flagsChanged(keyCode: Self.mods[i].code, flags: flags(down, synthetic: synthetic), t: t)
                case 45..<52:
                    input = .flagsChanged(keyCode: 57, flags: flags(down, synthetic: synthetic) | 0x1_0000, t: t)
                case 52..<70:
                    input = .keyDown(keyCode: Self.keys[rng.int(Self.keys.count)], flags: flags(down, synthetic: synthetic),
                                     isRepeat: rng.chance(0.2), t: t)
                case 70..<85:
                    input = .keyUp(keyCode: Self.keys[rng.int(Self.keys.count)], flags: flags(down, synthetic: synthetic), t: t)
                case 85..<91:
                    input = .pointer(t: t)
                case 91..<95:
                    // Go flips active, or changes the timings, or the bindings.
                    switch rng.int(3) {
                    case 0: config.active.toggle()
                    case 1: config.doubleTapMs = [300, 500, 800][rng.int(3)]
                    default: config = Self.configs[rng.int(Self.configs.count)]
                    }
                    new.configure(config)
                    old.configure(config)
                    input = nil
                case 95..<97:
                    new.reset()
                    old.reset()
                    input = nil
                default:
                    let a = new.tick(now: t), b = old.tick(now: t)
                    trace.append("tick \(t)")
                    XCTAssertEqual(a, b, "run \(run): tick at \(t)\n\(trace.joined(separator: "\n"))")
                    input = nil
                }
                if let input {
                    trace.append("\(input)")
                    let a = new.handle(input), b = old.handle(input)
                    XCTAssertEqual(a.events, b.events, "run \(run) events\n\(trace.joined(separator: "\n"))")
                    XCTAssertEqual(a.swallow, b.swallow, "run \(run) swallow\n\(trace.joined(separator: "\n"))")
                }
                XCTAssertEqual(new.nextDeadline, old.nextDeadline, "run \(run) deadline\n\(trace.joined(separator: "\n"))")
                XCTAssertEqual(new.missedEvents, old.missedEvents, "run \(run) missed")
                steps += 1
            }
        }
        XCTAssertGreaterThan(steps, 30000)
    }

    /// The bit rules agree with the Set rules for every chord and binding.
    func testCompiledMatchesAgreeWithSets() {
        let rules: [SideRule?] = [nil, .any, .left, .right]
        var rng = Rng(state: 7)
        for _ in 0..<3000 {
            var mods: [ModFamily: SideRule] = [:]
            for f in ModFamily.allCases { if let r = rules[rng.int(rules.count)] { mods[f] = r } }
            let b = Binding(id: "b", modifiers: mods, behavior: .tap)
            let chord = ModKeys(rawValue: UInt16(rng.int(512)))
            let set = chord.keys
            var want = true
            for family in ModFamily.allCases {
                let keys = set.filter { $0.family == family }
                guard let rule = mods[family] else {
                    if !keys.isEmpty { want = false }
                    continue
                }
                if keys.isEmpty { want = false }
                if rule == .left, keys.contains(where: { $0.isRight }) { want = false }
                if rule == .right, keys.contains(where: { !$0.isRight }) { want = false }
            }
            XCTAssertEqual(CompiledBinding(b).matches(chord), want, "\(mods) \(set)")
            XCTAssertEqual(ModKeys(set), chord)
            let flags = UInt64(rng.int(1 << 6)) << 16 | 0x100
            XCTAssertEqual(CompiledBinding(b).matches(flags: flags), b.matches(families: RawFlags.families(flags)), "\(mods) \(flags)")
        }
    }

    func testKeyBits() {
        var k = KeyBits()
        XCTAssertTrue(k.isEmpty)
        for c: UInt16 in [0, 63, 64, 127, 128, 179, 255, 256, 300, 65535] {
            XCTAssertFalse(k.contains(c))
            k.insert(c)
            XCTAssertTrue(k.contains(c))
        }
        for c: UInt16 in [0, 63, 64, 127, 128, 179, 255, 256, 300, 65535] {
            XCTAssertTrue(k.remove(c))
            XCTAssertFalse(k.remove(c))
        }
        XCTAssertTrue(k.isEmpty)
    }
}
