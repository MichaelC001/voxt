import Foundation

/// Blocking file operations run on a bounded Foundation queue, not Swift's
/// cooperative executor. UI waits are cancellable at ModelInstallationCache;
/// an already executing filesystem syscall cannot be safely interrupted.
nonisolated enum ModelDiskOperations {
    private static let queue: OperationQueue = {
        let queue = OperationQueue()
        queue.name = "Voxt.ModelDiskOperations"
        queue.qualityOfService = .utility
        queue.maxConcurrentOperationCount = 2
        return queue
    }()

    static func perform<Value: Sendable>(
        _ operation: @escaping @Sendable () throws -> Value
    ) async throws -> Value {
        try Task.checkCancellation()
        return try await withCheckedThrowingContinuation { continuation in
            queue.addOperation {
                continuation.resume(with: Result(catching: operation))
            }
        }
    }

    static func remove(_ directories: [URL]) async throws {
        try await perform {
            for directory in Set(directories) {
                if FileManager.default.fileExists(atPath: directory.path) {
                    try FileManager.default.removeItem(at: directory)
                }
            }
        }
    }
}
