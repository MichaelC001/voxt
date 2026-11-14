package deliver

// Model output is not trusted text: a language model fed a prompt
// injection (a selection translated from a web page, a remote model, a
// web search) may answer with control characters. Pasted into a terminal
// they act: ESC[201~ ends bracketed paste, so the lines after it run as
// commands; a line break runs the line before it. So what VoxtMini pastes
// or copies goes through Sanitize, and a paste into a terminal keeps no
// trailing line break and presses no auto key after text with a line
// break (insert). The history keeps the model's text as it came.

import (
	"strings"
	"unicode/utf8"
)

// Sanitize drops what text must never carry into another app: C0 control
// characters but tab and line feed (a carriage return, alone or before a
// line feed, is a line feed), DEL, C1 controls, and whole terminal escape
// sequences (ESC [ … final byte: CSI, as the bracketed paste's end
// marker ESC[201~; ESC ] … BEL or ESC \: OSC; ESC and one byte else).
// Other text, zero-width characters and emoji sequences included, is kept
// as is.
func Sanitize(text string) string {
	if clean(text) {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); {
		r, n := utf8.DecodeRuneInString(text[i:])
		switch {
		case r == '\r':
			b.WriteByte('\n')
			if strings.HasPrefix(text[i+1:], "\n") {
				n++
			}
		case r == 0x1b || r == 0x9b || r == 0x9d: // ESC, C1 CSI, C1 OSC
			n = escapeLen(text[i:], r, n)
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f:
			// dropped
		default:
			b.WriteString(text[i : i+n])
		}
		i += n
	}
	return b.String()
}

// clean reports that text has nothing Sanitize changes (the usual case:
// no copy).
func clean(text string) bool {
	for _, r := range text {
		if r < 0x20 && r != '\n' && r != '\t' || r >= 0x7f && r <= 0x9f {
			return false
		}
	}
	return true
}

// escapeLen is the length of the escape sequence at the start of s,
// which starts with r (n bytes): ESC or a C1 introducer.
func escapeLen(s string, r rune, n int) int {
	kind := r
	if r == 0x1b && len(s) > 1 {
		switch s[1] {
		case '[':
			kind, n = 0x9b, 2
		case ']':
			kind, n = 0x9d, 2
		default:
			if s[1] >= 0x20 && s[1] < 0x7f {
				return 2 // ESC and one byte: ESC c, ESC 7, …
			}
			return 1
		}
	}
	switch kind {
	case 0x9b: // CSI: parameter and intermediate bytes, then a final one
		for n < len(s) {
			c := s[n]
			n++
			if c >= 0x40 && c <= 0x7e {
				return n
			}
			if c < 0x20 || c > 0x7e {
				return n - 1 // not a CSI byte: the sequence ends before it
			}
		}
	case 0x9d: // OSC: up to BEL or ESC \ (string terminator)
		for n < len(s) {
			if s[n] == 0x07 {
				return n + 1
			}
			if s[n] == 0x1b && n+1 < len(s) && s[n+1] == '\\' {
				return n + 2
			}
			if s[n] == '\n' {
				return n
			}
			n++
		}
	}
	return n
}

// terminals are the bundle ids of terminal emulators: a paste into one
// may run what it holds.
var terminals = map[string]bool{
	"com.apple.Terminal": true, "com.googlecode.iterm2": true,
	"dev.warp.Warp-Stable": true, "dev.warp.Warp-Preview": true, "dev.warp.Warp": true,
	"com.mitchellh.ghostty": true, "net.kovidgoyal.kitty": true, "org.alacritty": true, "io.alacritty": true,
	"com.github.wez.wezterm": true, "org.tabby": true, "co.zeit.hyper": true,
}

// Terminal reports whether bundleID is a terminal emulator's.
func Terminal(bundleID string) bool { return terminals[bundleID] }
