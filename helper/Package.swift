// swift-tools-version:6.0
import Foundation
import PackageDescription

// voxtmini-helper: the small Swift process VoxtMini.app runs beside its Go
// UI, for what Go (MyGo, purego, no cgo) cannot do well: microphone capture
// (AVAudioEngine), the global hotkey event tap (CGEventTap, which must decide
// synchronously whether to swallow a key), text in and out of the app in
// front (Accessibility, the clipboard, posted keys) and browser URLs
// (Accessibility). JSON lines over stdin/stdout: helper
// protocol 1 (docs/helper-protocol.md).
//
// No third-party dependencies; `swift build -c release` takes seconds
// (scripts/build-helper.sh). VoxtMiniHelperKit holds the pure logic (the
// wire types, the hotkey recognizer, PCM framing, the app list), so
// `swift test` covers it without permissions, against helper/Fixtures.

/// The helper's own Info.plist, linked into the binary: it names the
/// helper (and its version) when it runs outside VoxtMini.app.
let infoPlist = URL(fileURLWithPath: #filePath).deletingLastPathComponent().appendingPathComponent("Support/Info.plist").path

let package = Package(
    name: "VoxtMiniHelper",
    platforms: [.macOS(.v15)],
    products: [
        .executable(name: "voxtmini-helper", targets: ["VoxtMiniHelper"]),
    ],
    targets: [
        .target(
            name: "VoxtMiniHelperKit",
            path: "Sources/VoxtMiniHelperKit"
        ),
        .executableTarget(
            name: "VoxtMiniHelper",
            dependencies: ["VoxtMiniHelperKit"],
            path: "Sources/VoxtMiniHelper",
            linkerSettings: [.unsafeFlags(["-Xlinker", "-sectcreate", "-Xlinker", "__TEXT", "-Xlinker", "__info_plist", "-Xlinker", infoPlist])]
        ),
        .testTarget(
            name: "VoxtMiniHelperKitTests",
            dependencies: ["VoxtMiniHelperKit"],
            path: "Tests/VoxtMiniHelperKitTests"
        ),
    ]
)
