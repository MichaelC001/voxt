// MeetingMemoryPressureMonitor.swift
// Converts macOS memory-pressure notifications into a bounded-inference safety signal.

import Dispatch
import Foundation
import Darwin

nonisolated final class MeetingMemoryPressureMonitor: @unchecked Sendable {
    private let lock = NSLock()
    private var source: DispatchSourceMemoryPressure?

    /// XNU exposes the current system level in dispatch NOTE_MEMORYSTATUS units.
    /// This read-only probe is optional: unavailable/unknown values must NEVER
    /// clear a previously observed pressure event merely because time passed.
    static func currentConstraint() -> Bool? {
        var level: UInt32 = 0
        var size = MemoryLayout<UInt32>.size
        guard sysctlbyname("kern.memorystatus_vm_pressure_level", &level, &size, nil, 0) == 0,
              size == MemoryLayout<UInt32>.size else { return nil }
        return constraint(forRawLevel: level)
    }

    static func constraint(forRawLevel level: UInt32) -> Bool? {
        switch UInt(level) {
        case DispatchSource.MemoryPressureEvent.normal.rawValue: return false
        case DispatchSource.MemoryPressureEvent.warning.rawValue,
             DispatchSource.MemoryPressureEvent.critical.rawValue: return true
        default: return nil
        }
    }

    func start(handler: @escaping @Sendable (Bool) -> Void) {
        lock.lock()
        defer { lock.unlock() }
        guard source == nil else { return }

        let source = DispatchSource.makeMemoryPressureSource(
            eventMask: [.normal, .warning, .critical],
            queue: DispatchQueue.global(qos: .utility)
        )
        source.setEventHandler {
            let event = source.data
            handler(event.contains(.warning) || event.contains(.critical))
        }
        source.resume()
        self.source = source
    }

    deinit {
        source?.cancel()
    }
}
