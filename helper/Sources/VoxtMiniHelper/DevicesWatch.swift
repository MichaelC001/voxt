import VoxtMiniHelperKit
import CoreAudio
import Foundation

/// capture.devices.changed: once the app has listed the inputs
/// (capture.devices), the helper tells it when they change: a device
/// plugged in or out, another default input. Core Audio calls the
/// listeners several times for one change (a Bluetooth headset adds its
/// input in steps), so the list is read 300 ms after the first call and
/// sent only when it differs from the last one sent.
final class DevicesWatch: @unchecked Sendable {
    private let out: Writer
    private let queue = DispatchQueue(label: "voxtmini.devices", qos: .utility)
    private let lock = NSLock()
    private var listening = false
    private var pending = false
    private var last = Data()
    private let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.outputFormatting = .sortedKeys
        return e
    }()

    init(out: Writer) { self.out = out }

    /// The inputs now; the first call starts the listeners.
    func list() -> DevicesResult {
        let r = DevicesResult(devices: captureDevices())
        let data = (try? encoder.encode(r)) ?? Data()
        let first = lock.withLock { () -> Bool in
            last = data
            defer { listening = true }
            return !listening
        }
        if first { listen() }
        return r
    }

    private func listen() {
        for selector in [kAudioHardwarePropertyDevices, kAudioHardwarePropertyDefaultInputDevice] {
            var addr = AudioObjectPropertyAddress(mSelector: selector, mScope: kAudioObjectPropertyScopeGlobal,
                                                  mElement: kAudioObjectPropertyElementMain)
            let status = AudioObjectAddPropertyListenerBlock(AudioObjectID(kAudioObjectSystemObject), &addr, queue) { [weak self] _, _ in
                self?.changed()
            }
            if status != noErr { log("devices: no listener for \(selector) (\(status))") }
        }
    }

    private func changed() {
        let schedule = lock.withLock { () -> Bool in
            defer { pending = true }
            return !pending
        }
        guard schedule else { return }
        queue.asyncAfter(deadline: .now() + .milliseconds(300)) { [self] in
            let r = DevicesResult(devices: captureDevices())
            let data = (try? encoder.encode(r)) ?? Data()
            let send = lock.withLock { () -> Bool in
                pending = false
                defer { last = data }
                return data != last
            }
            if send { out.notify(Method.devicesChanged, r) }
        }
    }
}
