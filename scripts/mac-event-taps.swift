// Lists the event taps installed on this Mac (CGGetEventTapList): who owns
// each, whether macOS waits for it (active) or only sends it a copy
// (listen-only), which events it takes, and the latency macOS measured
// for it. Read-only: no window, no event posted, no permission needed.
//
//   swift scripts/mac-event-taps.swift            # every tap
//   swift scripts/mac-event-taps.swift voxtmini   # those of processes whose name contains it
//
// An active tap that takes mouse presses holds every click until its
// callback answers; that is what froze clicks on mac-air on 2026-11-12
// (VoxtNew 2.0.0's helper, swapped out). VoxtMini's helper has an active
// keyboard tap and a listen-only mouse tap (docs/helper-protocol.md,
// "The taps").
import CoreGraphics
import Darwin
import Foundation

let filter = CommandLine.arguments.dropFirst().first?.lowercased()

var count: UInt32 = 0
guard CGGetEventTapList(0, nil, &count) == .success else { fatalError("CGGetEventTapList failed") }
var taps = [CGEventTapInformation](repeating: CGEventTapInformation(), count: Int(count))
guard CGGetEventTapList(count, &taps, &count) == .success else { fatalError("CGGetEventTapList failed") }

func processName(_ pid: pid_t) -> String {
    var buf = [CChar](repeating: 0, count: 4 * Int(MAXPATHLEN))
    guard proc_pidpath(pid, &buf, UInt32(buf.count)) > 0 else { return "?" }
    return URL(fileURLWithPath: String(cString: buf)).lastPathComponent
}

let names: [(UInt32, String)] = [
    (CGEventType.leftMouseDown.rawValue, "lDown"), (CGEventType.leftMouseUp.rawValue, "lUp"),
    (CGEventType.rightMouseDown.rawValue, "rDown"), (CGEventType.rightMouseUp.rawValue, "rUp"),
    (CGEventType.otherMouseDown.rawValue, "oDown"), (CGEventType.otherMouseUp.rawValue, "oUp"),
    (CGEventType.mouseMoved.rawValue, "move"), (CGEventType.leftMouseDragged.rawValue, "lDrag"),
    (CGEventType.scrollWheel.rawValue, "scroll"),
    (CGEventType.keyDown.rawValue, "keyDown"), (CGEventType.keyUp.rawValue, "keyUp"),
    (CGEventType.flagsChanged.rawValue, "flags"),
]
let mouseDowns: UInt64 = (1 << UInt64(CGEventType.leftMouseDown.rawValue)) | (1 << UInt64(CGEventType.rightMouseDown.rawValue)) |
    (1 << UInt64(CGEventType.otherMouseDown.rawValue))

func pad(_ s: String, _ n: Int, right: Bool = false) -> String {
    let fill = String(repeating: " ", count: max(0, n - s.count))
    return right ? fill + s : s + fill
}
func us(_ v: Float) -> String { pad(String(format: "%.0f", Double(v)), 9, right: true) }

print([pad("pid", 6), pad("process", 28), pad("mode", 11), pad("on", 3), pad("point", 9),
       pad("min µs", 9, right: true), pad("avg µs", 9, right: true), pad("max µs", 9, right: true), "  events"].joined(separator: " "))
var warnings: [String] = []
for t in taps.prefix(Int(count)) {
    let name = processName(t.tappingProcess)
    if let filter, !name.lowercased().contains(filter) { continue }
    let mask = t.eventsOfInterest
    let events = mask == ~0 ? "all" : names.filter { mask & (1 << UInt64($0.0)) != 0 }.map(\.1).joined(separator: ",")
    let active = t.options == .defaultTap
    let point = ["hid", "session", "annotated"][min(Int(t.tapPoint.rawValue), 2)]
    print([pad(String(t.tappingProcess), 6), pad(String(name.prefix(28)), 28), pad(active ? "active" : "listen-only", 11),
           pad(t.enabled ? "yes" : "no", 3), pad(point, 9), us(t.minUsecLatency), us(t.avgUsecLatency), us(t.maxUsecLatency),
           "  " + events].joined(separator: " "))
    if active, t.enabled, mask & mouseDowns != 0 {
        warnings.append("\(name) (pid \(t.tappingProcess)) holds every click until its callback answers")
    }
}
for w in warnings { print("note: " + w) }
