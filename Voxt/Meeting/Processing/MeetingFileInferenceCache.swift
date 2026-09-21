import Foundation
@preconcurrency import MLX

/// File inference keeps a modest reusable allocator cache between work units.
/// This is a boundary policy, NOT a cap on model weights or total process memory.
/// Do not alter MLX's global cache/memory limits or discard live speaker state.
nonisolated enum MeetingFileInferenceCache {
    static let retainedCacheThresholdBytes = 256 * 1_024 * 1_024

    static func shouldTrim(cacheBytes: Int, underPressure: Bool) -> Bool {
        cacheBytes > 0 && (underPressure || cacheBytes > retainedCacheThresholdBytes)
    }

    static func trimIfNeeded(underPressure: Bool) {
        let before = Memory.snapshot()
        guard shouldTrim(cacheBytes: before.cacheMemory, underPressure: underPressure) else { return }
        // MLX releases only allocator-owned unused buffers. Live weights, the
        // Sortformer FIFO and in-flight allocations stay owned by their users.
        // Called after file native work returns, before its permit is released,
        // or before admission while this coordinator's lane is idle.
        Memory.clearCache()
        let after = Memory.snapshot()
        VoxtLog.meeting(
            "File inference cache reclaimed. pressure=\(underPressure), cacheBeforeBytes=\(before.cacheMemory), cacheAfterBytes=\(after.cacheMemory), activeBeforeBytes=\(before.activeMemory), activeAfterBytes=\(after.activeMemory)"
        )
        MeetingFileTrace.event("file-cache-reclaimed", "pressure=\(underPressure), cacheBeforeBytes=\(before.cacheMemory), cacheAfterBytes=\(after.cacheMemory), activeAfterBytes=\(after.activeMemory)")
    }
}
