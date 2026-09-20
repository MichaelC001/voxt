// Lightweight, cached presentation for the status menu; no storage reads on hover.

import Foundation

enum StatusMenuHistorySupport {
    static let recentLimit = 5

    static func filters(availability: FeatureAvailabilitySettings) -> [HistoryFilterTab] {
        HistoryFilterTab.allCases.filter {
            $0.correspondingFeatureTab.isEnabled(in: availability)
        }
    }

    static func recentEntries(
        from candidates: [TranscriptionHistoryListEntry],
        availability: FeatureAvailabilitySettings
    ) -> [TranscriptionHistoryListEntry] {
        Array(candidates.filter {
            $0.kind == .normal || ($0.kind == .translation && availability.translationEnabled)
        }.sorted {
            if $0.createdAt != $1.createdAt { return $0.createdAt > $1.createdAt }
            return $0.id.uuidString < $1.id.uuidString
        }.prefix(recentLimit))
    }

    static func previewTitle(for entry: TranscriptionHistoryListEntry) -> String {
        // Bound both text layout and menu width, without changing the copied text.
        let preview = entry.previewText.split(whereSeparator: { $0.isWhitespace }).joined(separator: " ")
        let prefix = String(preview.prefix(48))
        // SQLite length()/substr() count Unicode code points, not Swift graphemes.
        let wasTruncatedByRepository = entry.textLength > entry.previewText.unicodeScalars.count
        return prefix + (preview.count > 48 || wasTruncatedByRepository ? "…" : "")
    }
}
