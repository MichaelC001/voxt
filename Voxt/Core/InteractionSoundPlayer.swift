// InteractionSoundPlayer.swift
// Provides Interaction Sound Player for core app behavior.

import AVFoundation
import Foundation

final class InteractionSoundPlayer: @unchecked Sendable {
    private let playbackQueue = DispatchQueue(label: "com.voxt.interactionSound", qos: .userInitiated)
    // Keep cues clearly audible without competing with speech or meeting audio.
    private let volume: Float = 0.50
    private var preparedPlayers: [String: AVAudioPlayer] = [:]
    private var preparedPresetRawValue: String?
    private var activePlayer: AVAudioPlayer?

    /// Loads the currently selected cue pair away from the main actor.
    ///
    /// This is intentionally bounded to the active preset so changing the sound
    /// preference does not eagerly allocate players for every system sound.
    func prewarm() {
        playbackQueue.async { [weak self] in
            guard let self else { return }
            self.preparePlayersIfNeeded(for: self.currentPreset())
        }
    }

    @discardableResult
    func playStart() -> TimeInterval {
        playbackQueue.sync {
            play(role: "start", for: currentPreset())
        }
    }

    /// Starts playback off the main actor and invokes completion after the cue
    /// duration. This keeps wake feedback from blocking AppKit's first frame.
    func playStartAsync(completion: (@MainActor () -> Void)? = nil) {
        playbackQueue.async { [weak self] in
            guard let self else { return }
            let duration = self.play(role: "start", for: self.currentPreset())
            self.playbackQueue.asyncAfter(deadline: .now() + duration) {
                guard let completion else { return }
                Task { @MainActor in completion() }
            }
        }
    }

    @discardableResult
    func playEnd() -> TimeInterval {
        playbackQueue.sync {
            play(role: "end", for: currentPreset())
        }
    }

    @discardableResult
    func playPreview(preset: InteractionSoundPreset) -> TimeInterval {
        playbackQueue.sync {
            play(role: "start", for: preset)
        }
    }

    func reset() {
        playbackQueue.sync {
            activePlayer?.stop()
            activePlayer = nil
            preparedPlayers.removeAll(keepingCapacity: false)
            preparedPresetRawValue = nil
        }
    }

    private func currentPreset() -> InteractionSoundPreset {
        let raw = UserDefaults.standard.string(forKey: AppPreferenceKey.interactionSoundPreset) ?? ""
        return InteractionSoundPreset(rawValue: raw) ?? .soft
    }

    private func resolvedSounds(for preset: InteractionSoundPreset) -> (start: String, end: String) {
        switch preset {
        case .soft:
            return ("Pop", "Tink")
        case .glass:
            return ("Ping", "Ping")
        case .funk:
            return ("Morse", "Morse")
        case .submarine:
            return ("Submarine", "Submarine")
        case .basso:
            return ("Basso", "Basso")
        case .bottle:
            return ("Bottle", "Bottle")
        case .frog:
            return ("Frog", "Frog")
        case .hero:
            return ("Hero", "Hero")
        case .purr:
            return ("Purr", "Purr")
        case .sosumi:
            return ("Sosumi", "Sosumi")
        }
    }

    private func play(role: String, for preset: InteractionSoundPreset) -> TimeInterval {
        preparePlayersIfNeeded(for: preset)
        guard let player = preparedPlayers[role] else { return 0 }

        player.stop()
        player.currentTime = 0
        player.volume = volume
        player.prepareToPlay()
        guard player.play() else {
            // A player can become invalid after sleep or an output-device change.
            // Drop only the failed cue so the next invocation can rebuild it.
            preparedPlayers.removeValue(forKey: role)
            preparedPresetRawValue = nil
            activePlayer = nil
            return 0
        }
        activePlayer = player
        return player.duration
    }

    private func preparePlayersIfNeeded(for preset: InteractionSoundPreset) {
        guard preparedPresetRawValue != preset.rawValue else { return }

        preparedPlayers.removeAll(keepingCapacity: true)
        preparedPresetRawValue = preset.rawValue
        let sounds = resolvedSounds(for: preset)
        for (role, name) in [("start", sounds.start), ("end", sounds.end)] {
            guard let url = soundURL(named: name) ?? soundURL(named: "Pop") ?? soundURL(named: "Tink") else {
                continue
            }
            do {
                let player = try AVAudioPlayer(contentsOf: url)
                player.volume = volume
                player.prepareToPlay()
                preparedPlayers[role] = player
            } catch {
                VoxtLog.audioWarning(
                    "Interaction sound failed to prewarm. name=\(name), error=\(error.localizedDescription)"
                )
            }
        }
    }

    private func soundURL(named name: String) -> URL? {
        let systemURL = URL(fileURLWithPath: "/System/Library/Sounds", isDirectory: true)
            .appendingPathComponent(name)
            .appendingPathExtension("aiff")
        return FileManager.default.fileExists(atPath: systemURL.path) ? systemURL : nil
    }
}
