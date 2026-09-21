import AppKit
import SwiftUI

struct FileTranscriptPreview: Identifiable {
    let id: UUID
    let text: String
}

/// Read-only draft; deliberately not a history detail model. Opening this sheet
/// cannot trigger summary, translation, speaker processing, or persist success.
struct FileTranscriptPreviewSheet: View {
    @Environment(\.dismiss) private var dismiss
    let preview: FileTranscriptPreview

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(AppLocalization.localizedString("Transcription ready — speaker analysis incomplete"))
                .font(.headline)
            ScrollView {
                Text(verbatim: preview.text)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            HStack {
                Spacer()
                Button(AppLocalization.localizedString("Copy")) {
                    _ = AppDelegate.shared?.pasteboardTextWriter.write(preview.text, to: .general, restorePrevious: false)
                }
                Button(AppLocalization.localizedString("Close")) { dismiss() }
                    .keyboardShortcut(.cancelAction)
            }
        }
        .padding(20)
        .frame(width: 600, height: 420)
    }
}
