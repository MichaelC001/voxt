// A paste's check (textio.paste's verifyMs): what the focused element
// showed before ⌘V and after it, and whether that changed. The reading
// itself is Accessibility's (the helper's TextIO); the rules are here,
// pure, so that they are tested.

/// One reading of the focused element: its value's length (UTF-16) and a
/// digest of the value, its selection, and a digest of its selected text
/// marker range (WebKit's and Chromium's caret, also on a web area that
/// has neither value nor selection); nil where Accessibility gave nothing.
public struct FieldReading: Equatable, Sendable {
    public var length: Int?
    public var digest: Int?
    public var selection: TextRange?
    public var marker: Int?

    public init(length: Int?, digest: Int? = nil, selection: TextRange?, marker: Int? = nil) {
        self.length = length
        self.digest = digest
        self.selection = selection
        self.marker = marker
    }

    /// The element tells something a paste would change.
    public var readable: Bool { length != nil || selection != nil || marker != nil }

    /// The reading as the result shows it (nil: nothing readable).
    public var state: FieldState? {
        readable ? FieldState(length: length ?? 0, selection: selection, marker: marker != nil ? true : nil) : nil
    }
}

public enum FieldChange: Equatable, Sendable {
    /// The element shows the paste (or something else typed meanwhile).
    case changed
    /// It reads as before.
    case same
    /// Nothing to compare: the element is gone or tells nothing.
    case unreadable
}

public enum PasteChecker {
    /// Whether after differs from before in what both could read: the
    /// value's length or digest, the selection, or the text marker (a caret
    /// moves after a paste even where the value is not readable, as in
    /// some web editors and on a web area).
    public static func compare(_ before: FieldReading, _ after: FieldReading?) -> FieldChange {
        guard before.readable, let after, after.readable else { return .unreadable }
        var compared = false
        if let a = before.length, let b = after.length {
            compared = true
            if a != b { return .changed }
        }
        if let a = before.digest, let b = after.digest {
            compared = true
            if a != b { return .changed }
        }
        if let a = before.selection, let b = after.selection {
            compared = true
            if a != b { return .changed }
        }
        if let a = before.marker, let b = after.marker {
            compared = true
            if a != b { return .changed }
        }
        return compared ? .same : .unreadable
    }

    /// The check's outcome at its end: inserted once a reading changed;
    /// unknown when nothing could be compared, or another element has the
    /// focus now (focusMoved); else unchanged.
    public static func outcome(sawChange: Bool, before: FieldReading, last: FieldReading?, focusMoved: Bool) -> String {
        if sawChange { return PasteCheck.inserted }
        if focusMoved || compare(before, last) == .unreadable { return PasteCheck.unknown }
        return PasteCheck.unchanged
    }

    /// Whether a paste whose element still reads as before gets its second
    /// ⌘V (PasteParams.retryMs): nil when it does, else why not
    /// (PasteRetrySkip). Only when nothing else happened since the first:
    /// the same element has the focus in the same app, it reads as before,
    /// the clipboard still holds the text, and no app has secure input on
    /// (the second ⌘V would be kept away too). A first ⌘V the app takes
    /// late would make the text go in twice: that is the price of the
    /// retry, which only comes after retryMs (a web text area shows a
    /// paste within 50–200 ms).
    public static func retryBlock(before: FieldReading, now: FieldReading?, sameElement: Bool, targetInFront: Bool,
                                  clipboardOurs: Bool, secureInput: Bool) -> String? {
        if !sameElement { return PasteRetrySkip.focusMoved }
        if !targetInFront { return PasteRetrySkip.notFront }
        if compare(before, now) != .same { return PasteRetrySkip.unreadable }
        if !clipboardOurs { return PasteRetrySkip.clipboard }
        if secureInput { return PasteRetrySkip.secureInput }
        return nil
    }

    /// The modifier families in CGEventFlags' raw value, for the log
    /// (PasteDiag.held): command, shift, option, control, fn.
    public static func modifierNames(_ flags: UInt64) -> [String] {
        var out: [String] = []
        for (bit, name) in [(RawFlags.command, "command"), (RawFlags.shift, "shift"), (RawFlags.option, "option"),
                            (RawFlags.control, "control"), (RawFlags.secondaryFn, "fn")] where flags & bit != 0 {
            out.append(name)
        }
        return out
    }

    /// The digest of a value (stable within the process).
    public static func digest(_ value: String) -> Int {
        var h = Hasher()
        h.combine(value)
        return h.finalize()
    }
}
