// Package hans writes Chinese in Simplified characters: VoxtMini's
// Chinese is Simplified Chinese only (user, 2026-11-13), whatever a
// speech or language model wrote (Whisper often writes Traditional for
// Mandarin, Qwen3-ASR and remote services sometimes). Simplify converts
// with OpenCC's Traditional-to-Simplified tables (t2s.txt, gen.go):
// phrases first, the longest that matches, else character by character.
//
// Japanese is never converted: many of its kanji are Traditional Chinese
// characters (東京, 時間, 電話, 後), which the tables would change. A text
// with kana in it is Japanese and left as it is; the caller skips a text
// it knows is Japanese (Japanese is the language in use). Other scripts,
// English among them, have no entry in the tables.
//
// Imports: none (leaf).
package hans

//go:generate go run gen.go

import (
	_ "embed"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed t2s.txt
var table string

var (
	once    sync.Once
	chars   map[rune]string
	phrases map[string]string
	starts  map[rune]bool // a phrase's first runes
	longest int           // the longest phrase, in runes
)

// maxPhrase is the longest phrase the tables may have, in runes.
const maxPhrase = 16

func load() {
	chars = make(map[rune]string, 3300)
	phrases = make(map[string]string, 600)
	starts = make(map[rune]bool, 400)
	for line := range strings.Lines(table) {
		line = strings.TrimSuffix(line, "\n")
		if line == "" || line[0] == '#' {
			continue
		}
		k, v, _ := strings.Cut(line, "\t")
		r, size := utf8.DecodeRuneInString(k)
		if size == len(k) {
			chars[r] = v
			continue
		}
		if utf8.RuneCountInString(k) > maxPhrase {
			continue // gen.go's tables have none; TestIdempotent tells
		}
		phrases[k] = v
		starts[r] = true
		longest = max(longest, utf8.RuneCountInString(k))
	}
}

// Simplify returns text with its Traditional Chinese in Simplified
// characters (頭髮 → 头发, 看著 → 看着; 乾隆 and 著名 stay), the rest
// as it is. A text with kana in it is Japanese: it comes back unchanged.
// Converting twice changes nothing more than once.
func Simplify(text string) string {
	once.Do(load)
	i := first(text)
	if i < 0 || Japanese(text) {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	b.WriteString(text[:i])
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		if starts[r] {
			if k, v := match(text[i:]); k > 0 {
				b.WriteString(v)
				i += k
				continue
			}
		}
		if v, ok := chars[r]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(text[i : i+size])
		}
		i += size
	}
	return b.String()
}

// first is the byte offset of the first conversion that changes text,
// -1 for none (the text stays as it is). It reads text as Simplify does:
// a phrase that stays (著名) is passed over whole.
func first(text string) int {
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r < 0x2E80 { // below the CJK blocks: nothing to convert
			i += size
			continue
		}
		if starts[r] {
			if k, v := match(text[i:]); k > 0 {
				if v != text[i:i+k] {
					return i
				}
				i += k
				continue
			}
		}
		if _, ok := chars[r]; ok {
			return i
		}
		i += size
	}
	return -1
}

// match finds the longest phrase s starts with: its length in bytes
// and its conversion, 0 for none.
func match(s string) (int, string) {
	// The byte offsets after each of the next longest runes.
	var buf [maxPhrase]int
	ends := buf[:0]
	for i, r := range s {
		if len(ends) == longest {
			break
		}
		ends = append(ends, i+utf8.RuneLen(r))
	}
	for n := len(ends); n >= 2; n-- {
		if v, ok := phrases[s[:ends[n-1]]]; ok {
			return ends[n-1], v
		}
	}
	return 0, ""
}

// Japanese reports whether text has kana in it (hiragana, katakana, the
// half-width katakana) or 々: it is Japanese. The katakana middle dot
// (・) does not count: Chinese text uses it for names too.
func Japanese(text string) bool {
	for _, r := range text {
		switch {
		case r < 0x3005:
		case r == 0x3005: // 々
			return true
		case r >= 0x3041 && r <= 0x309F: // hiragana
			return true
		case r >= 0x30A0 && r <= 0x30FF && r != 0x30FB: // katakana
			return true
		case r >= 0x31F0 && r <= 0x31FF: // katakana phonetic extensions
			return true
		case r >= 0xFF66 && r <= 0xFF9F: // half-width katakana
			return true
		}
	}
	return false
}
