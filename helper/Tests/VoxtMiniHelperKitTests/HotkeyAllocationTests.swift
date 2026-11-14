@testable import VoxtMiniHelperKit
import XCTest
#if canImport(Darwin)
import Darwin

// The recognizer runs in the active tap's callback while macOS holds the
// key: an event that fires nothing must not allocate (malloc's locks and
// pages, maybe swapped out). libmalloc's malloc_logger hook counts the
// allocations this thread makes while a stream of such events goes through,
// per event of feed (0: outside the recognizer, the test's own code), and
// keeps the first stacks for the failure message.
//
// The measured code itself must not allocate in a debug build: no for-in
// (an unspecialized IndexingIterator.next() allocates per element; the
// measured `for _ in 0..<500` cost exactly 500 on macOS 26), no local
// function, no array literal.

private typealias MallocLogger = @convention(c) (UInt32, UInt, UInt, UInt, UInt, UInt32) -> Void
// execinfo's backtrace and backtrace_symbols, found with dlsym.
private typealias Backtrace = @convention(c) (UnsafeMutablePointer<UnsafeMutableRawPointer?>?, Int32) -> Int32
private typealias BacktraceSymbols = @convention(c) (UnsafePointer<UnsafeMutableRawPointer?>?, Int32) -> UnsafeMutablePointer<UnsafeMutablePointer<CChar>?>?
private let stackDepth = 24, keptStacks = 3, events = 16
nonisolated(unsafe) private var allocations = 0
nonisolated(unsafe) private var watchedThread: pthread_t?
nonisolated(unsafe) private var currentEvent = 0
nonisolated(unsafe) private var perEvent: UnsafeMutablePointer<Int>?
nonisolated(unsafe) private var stacks: UnsafeMutablePointer<UnsafeMutableRawPointer?>?
nonisolated(unsafe) private var depths: UnsafeMutablePointer<Int32>?
nonisolated(unsafe) private var backtrace: Backtrace?
private let countAllocations: MallocLogger = { type, _, _, _, _, _ in
    // MALLOC_LOG_TYPE_ALLOCATE (realloc's new block included).
    guard type & 2 != 0, let w = watchedThread, pthread_equal(pthread_self(), w) != 0,
          let perEvent, let stacks, let depths, let backtrace else { return }
    if allocations < keptStacks {
        // backtrace() walks frames (thread_stack_pcs): no allocation.
        depths[allocations] = backtrace(stacks + allocations * stackDepth, Int32(stackDepth))
    }
    allocations += 1
    perEvent[currentEvent] += 1
}

final class HotkeyAllocationTests: XCTestCase {
    let rOpt = RawFlags.option | 0x40 | 0x100
    let lOpt = RawFlags.option | 0x20 | 0x100
    let names = ["test code", "keyDown a", "keyUp a", "left ⌥ down", "⌥ keyDown", "⌥ repeat", "⌥ keyUp",
                 "left ⌥ up", "right ⌥ down", "pointer", "right ⌥ up", "right ⌥ down", "keyDown 1",
                 "keyUp 1", "right ⌥ up", "caps lock"]

    /// Typing, ⌥-typing with the left key, an ⌥-click, a right ⌥ chord
    /// spoiled by a key, Caps Lock: nothing fires.
    func feed(_ r: inout HotkeyRecognizer, _ t: inout Double) -> Int {
        var n = 0
        t += 10
        n += event(1, &r, .keyDown(keyCode: 0, flags: 0x100, isRepeat: false, t: t))
        n += event(2, &r, .keyUp(keyCode: 0, flags: 0x100, t: t + 1))
        n += event(3, &r, .flagsChanged(keyCode: 58, flags: lOpt, t: t + 2))
        n += event(4, &r, .keyDown(keyCode: 14, flags: lOpt, isRepeat: false, t: t + 3))
        n += event(5, &r, .keyDown(keyCode: 14, flags: lOpt, isRepeat: true, t: t + 4))
        n += event(6, &r, .keyUp(keyCode: 14, flags: lOpt, t: t + 5))
        n += event(7, &r, .flagsChanged(keyCode: 58, flags: 0x100, t: t + 6))
        n += event(8, &r, .flagsChanged(keyCode: 61, flags: rOpt, t: t + 7))
        n += event(9, &r, .pointer(t: t + 8))
        n += event(10, &r, .flagsChanged(keyCode: 61, flags: 0x100, t: t + 9))
        n += event(11, &r, .flagsChanged(keyCode: 61, flags: rOpt, t: t + 10))
        n += event(12, &r, .keyDown(keyCode: 2, flags: rOpt, isRepeat: false, t: t + 11))
        n += event(13, &r, .keyUp(keyCode: 2, flags: rOpt, t: t + 12))
        n += event(14, &r, .flagsChanged(keyCode: 61, flags: 0x100, t: t + 13))
        n += event(15, &r, .flagsChanged(keyCode: 57, flags: 0x1_0100, t: t + 14))
        t += 20
        return n
    }

    /// One event, its allocations counted under k; what fired and swallowed.
    func event(_ k: Int, _ r: inout HotkeyRecognizer, _ input: KeyInput) -> Int {
        currentEvent = k
        let res = r.handle(input)
        currentEvent = 0
        return res.events.count + (res.swallow ? 1 : 0)
    }

    func testEventsThatFireNothingAllocateNothing() throws {
        guard let sym = dlsym(UnsafeMutableRawPointer(bitPattern: -2), "malloc_logger") else {
            throw XCTSkip("no malloc_logger")
        }
        let slot = sym.assumingMemoryBound(to: Optional<MallocLogger>.self)
        guard slot.pointee == nil else { throw XCTSkip("malloc_logger in use (MallocStackLogging)") }
        guard let bt = dlsym(UnsafeMutableRawPointer(bitPattern: -2), "backtrace"),
              let bs = dlsym(UnsafeMutableRawPointer(bitPattern: -2), "backtrace_symbols") else {
            throw XCTSkip("no backtrace")
        }
        let backtraceFn = unsafeBitCast(bt, to: Backtrace.self)
        var r = HotkeyRecognizer(config: HotkeyConfig(bindings: [
            Binding(id: "transcribe/0", modifiers: [.option: .right], behavior: .tap),
            Binding(id: "translate/0", modifiers: [.option: .right, .shift: .any], behavior: .tap),
            Binding(id: "rewrite/0", modifiers: [.control: .any, .option: .right], behavior: .doubleTap),
            Binding(id: "paste-last", modifiers: [.control: .any, .option: .any], key: 9, behavior: .tap),
        ]))
        let counts = UnsafeMutablePointer<Int>.allocate(capacity: events)
        counts.initialize(repeating: 0, count: events)
        let pcs = UnsafeMutablePointer<UnsafeMutableRawPointer?>.allocate(capacity: keptStacks * stackDepth)
        pcs.initialize(repeating: nil, count: keptStacks * stackDepth)
        let ds = UnsafeMutablePointer<Int32>.allocate(capacity: keptStacks)
        ds.initialize(repeating: 0, count: keptStacks)
        defer { counts.deallocate(); pcs.deallocate(); ds.deallocate() }
        _ = backtraceFn(pcs, Int32(stackDepth)) // first use outside the hook
        var t = 0.0
        var fired = feed(&r, &t) // first use: lazy globals
        perEvent = counts
        stacks = pcs
        depths = ds
        backtrace = backtraceFn
        watchedThread = pthread_self()
        allocations = 0
        slot.pointee = countAllocations
        let probe = malloc(64)
        free(probe)
        let sane = allocations
        allocations = 0
        counts[0] = 0
        var i = 0
        while i < 500 {
            fired += feed(&r, &t)
            i += 1
        }
        slot.pointee = nil
        watchedThread = nil
        perEvent = nil
        stacks = nil
        depths = nil
        backtrace = nil
        guard sane > 0 else { throw XCTSkip("malloc_logger did not see malloc") }
        XCTAssertEqual(fired, 0)
        XCTAssertEqual(allocations, 0, report(counts: counts, pcs: pcs, depths: ds, symbols: unsafeBitCast(bs, to: BacktraceSymbols.self)))
    }

    /// Where the allocations were: per event, then the first stacks.
    fileprivate func report(counts: UnsafeMutablePointer<Int>, pcs: UnsafeMutablePointer<UnsafeMutableRawPointer?>, depths: UnsafeMutablePointer<Int32>,
                symbols: BacktraceSymbols) -> String {
        var lines = ["allocations per event over 500 rounds:"]
        for k in 0..<events where counts[k] != 0 { lines.append("  \(k) \(names[k]): \(counts[k])") }
        for s in 0..<min(allocations, keptStacks) {
            lines.append("stack \(s + 1):")
            guard let syms = symbols(pcs + s * stackDepth, depths[s]) else { continue }
            for f in 0..<Int(depths[s]) {
                if let c = syms[f] { lines.append("  " + String(cString: c)) }
            }
            free(syms)
        }
        return lines.joined(separator: "\n")
    }
}
#endif
