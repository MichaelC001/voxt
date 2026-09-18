// MeetingSpeakerDiarizationEngines.swift
// Provides Meeting Speaker Diarization Engines for meeting speaker analysis.

import Foundation
import MLX
import MLXAudioVAD

protocol MeetingSpeakerDiarizationEngine: Sendable {
    func diarize(
        asset: MeetingAudioAsset,
        options: MeetingSpeakerDiarizationOptions
    ) async throws -> [MeetingSpeakerTurn]

    func diarizeSession(
        descriptors: [MeetingAudioAssetDescriptor],
        loadAsset: @escaping @Sendable (MeetingAudioAssetDescriptor) async -> MeetingAudioAsset?,
        continuousAudioURL: URL?,
        options: MeetingSpeakerDiarizationOptions,
        progress: (@Sendable (Double) async -> Void)?
    ) async throws -> [MeetingSpeakerTurn]
}

extension MeetingSpeakerDiarizationEngine {
    func diarizeSession(
        descriptors: [MeetingAudioAssetDescriptor],
        loadAsset: @escaping @Sendable (MeetingAudioAssetDescriptor) async -> MeetingAudioAsset?,
        continuousAudioURL _: URL?,
        options: MeetingSpeakerDiarizationOptions,
        progress: (@Sendable (Double) async -> Void)?
    ) async throws -> [MeetingSpeakerTurn] {
        var turns: [MeetingSpeakerTurn] = []
        let descriptorCount = max(descriptors.count, 1)
        await progress?(0)
        for (index, descriptor) in descriptors.enumerated() {
            try Task.checkCancellation()
            if let asset = await loadAsset(descriptor) {
                turns.append(contentsOf: try await diarize(asset: asset, options: options))
            }
            await progress?(Double(index + 1) / Double(descriptorCount))
        }
        return turns
    }
}

enum MeetingSpeakerDiarizationEngineFactory {
    nonisolated private static let sharedSortformerEngine = SortformerMeetingSpeakerDiarizationEngine()

    nonisolated static func makeDefault(defaults _: UserDefaults = .standard) -> (any MeetingSpeakerDiarizationEngine)? {
        sharedSortformerEngine
    }
}

actor SortformerMeetingSpeakerDiarizationEngine: MeetingSpeakerDiarizationEngine {
    private var model: SortformerModel?

    func diarize(
        asset: MeetingAudioAsset,
        options _: MeetingSpeakerDiarizationOptions
    ) async throws -> [MeetingSpeakerTurn] {
        let model = try await loadModelIfAvailable()
        let prepared = ASRVoiceActivitySampleRateConverter.resample(
            samples: asset.samples,
            from: asset.sampleRate,
            to: 16_000
        )
        guard !prepared.isEmpty else { return [] }

        let state = model.initStreamingState()
        let (output, _) = try await model.feed(
            chunk: MLXArray(prepared),
            state: state,
            sampleRate: 16_000,
            threshold: 0.5,
            minDuration: 0.25,
            mergeGap: 0.18
        )

        return output.segments.map { item in
            MeetingSpeakerTurn(
                source: asset.source,
                speakerID: "sortformer-\(item.speaker)",
                displayName: MeetingSpeakerDisplayNameFormatter.displayName(ordinal: item.speaker + 1),
                startSeconds: asset.sessionStartOffset + TimeInterval(item.start),
                endSeconds: asset.sessionStartOffset + TimeInterval(item.end),
                confidence: nil
            )
        }
        .filter { $0.endSeconds > $0.startSeconds }
    }

    func diarizeSession(
        descriptors: [MeetingAudioAssetDescriptor],
        loadAsset: @escaping @Sendable (MeetingAudioAssetDescriptor) async -> MeetingAudioAsset?,
        continuousAudioURL _: URL?,
        options _: MeetingSpeakerDiarizationOptions,
        progress: (@Sendable (Double) async -> Void)?
    ) async throws -> [MeetingSpeakerTurn] {
        let model = try await loadModelIfAvailable()
        var state = model.initStreamingState()
        var streamBaseOffset = descriptors.first?.sessionStartOffset ?? 0
        var previousDescriptor: MeetingAudioAssetDescriptor?
        var turns: [MeetingSpeakerTurn] = []
        let descriptorCount = max(descriptors.count, 1)
        await progress?(0)

        for (index, descriptor) in descriptors.enumerated() {
            try Task.checkCancellation()
            if let previousDescriptor {
                let expectedStart = previousDescriptor.sessionStartOffset + previousDescriptor.durationSeconds
                let isContinuous = descriptor.source == previousDescriptor.source
                    && abs(descriptor.sessionStartOffset - expectedStart) < 0.05
                if !isContinuous {
                    state = model.initStreamingState()
                    streamBaseOffset = descriptor.sessionStartOffset
                }
            }
            previousDescriptor = descriptor

            guard let asset = await loadAsset(descriptor) else {
                await progress?(Double(index + 1) / Double(descriptorCount))
                continue
            }
            let prepared = ASRVoiceActivitySampleRateConverter.resample(
                samples: asset.samples,
                from: asset.sampleRate,
                to: 16_000
            )
            guard !prepared.isEmpty else {
                await progress?(Double(index + 1) / Double(descriptorCount))
                continue
            }

            let (output, newState) = try await model.feed(
                chunk: MLXArray(prepared),
                state: state,
                sampleRate: 16_000,
                threshold: 0.5,
                minDuration: 0.25,
                mergeGap: 0.18
            )
            state = newState
            turns.append(contentsOf: output.segments.compactMap { item in
                let turn = MeetingSpeakerTurn(
                    source: asset.source,
                    speakerID: "sortformer-\(item.speaker)",
                    displayName: MeetingSpeakerDisplayNameFormatter.displayName(ordinal: item.speaker + 1),
                    startSeconds: streamBaseOffset + TimeInterval(item.start),
                    endSeconds: streamBaseOffset + TimeInterval(item.end),
                    confidence: nil
                )
                return turn.endSeconds > turn.startSeconds ? turn : nil
            })
            await progress?(Double(index + 1) / Double(descriptorCount))
        }
        return turns
    }

    private func loadModelIfAvailable() async throws -> SortformerModel {
        if let model {
            return model
        }
        let directory = await MeetingSortformerModelStorage.validatedModelDirectory()
        guard let directory else {
            throw MeetingVADModelError.modelNotDownloaded
        }
        let loaded = try SortformerModel.fromModelDirectory(directory)
        model = loaded
        return loaded
    }
}
