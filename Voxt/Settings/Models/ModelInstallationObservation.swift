import Combine
import Foundation

/// Observe only installation identities, not download progress or inference state.
/// Delivery on the next run-loop turn coalesces a burst of completed scans and
/// avoids reading @Published values during willSet. No polling or disk reads.
enum ModelInstallationObservation {
    static func changes(
        mlx: MLXModelManager,
        customLLM: CustomLLMModelManager
    ) -> AnyPublisher<Void, Never> {
        Publishers.Merge(
            mlx.$installationRevision.map { _ in () },
            customLLM.$installationRevision.map { _ in () }
        )
        .debounce(for: .milliseconds(100), scheduler: RunLoop.main)
        .eraseToAnyPublisher()
    }
}
