package compose

import (
	"strings"
	"unicode"

	"github.com/hehehai/VoxtMini/internal/lang"
)

// TargetLanguages are the languages Translate offers, in the menu's
// order: VoxtMini's three (internal/lang), Chinese as Simplified Chinese.
var TargetLanguages = []string{lang.English, "zh-Hans", lang.Japanese}

// LanguageName names a language in a template set's language: the lang
// table's name, Chinese as Simplified Chinese (a target must say which
// script); a code of no language of VoxtMini's as it is.
func LanguageName(code, set string) string {
	l, ok := lang.Lookup(code)
	switch {
	case !ok:
		return code
	case l.Code != lang.Chinese:
		return l.NameIn(set)
	case set == Chinese || strings.HasPrefix(strings.ToLower(set), "zh"):
		return "简体中文"
	}
	return "Simplified Chinese"
}

// Untranslated reports whether a translation into target looks like it
// was not made, and is worth asking again (Input.Strict):
//
//   - it is the source again, ignoring case, spaces and punctuation;
//   - or its letters are mostly not in the target's script while the
//     source's are not either (Chinese asked, Latin text back; English
//     asked, Chinese back). A source in Latin letters into English is
//     judged by the first rule only: it may be English already.
//
// A source already in the target's script (Chinese into Chinese) is
// never judged untranslated: the model may rightly return it.
func Untranslated(source, result, target string) bool {
	src, res := fold(source), fold(result)
	if src == "" || res == "" {
		return false
	}
	is := scriptOf(target)
	if is == nil {
		return src == res
	}
	srcShare, srcLetters := share(source, is)
	resShare, resLetters := share(result, is)
	if srcLetters > 0 && srcShare >= 0.5 && !isLatin(target) {
		return false // the source is in the target's script already
	}
	if src == res {
		return true
	}
	return resLetters > 0 && resShare < 0.5 && srcShare < 0.5
}

// fold is s without case, spaces and punctuation.
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

// scriptOf is a test of the letters a target language is written in; nil
// when unknown.
func scriptOf(target string) func(rune) bool {
	switch lang.Canonical(target) {
	case lang.English:
		return func(r rune) bool { return unicode.Is(unicode.Latin, r) }
	case lang.Chinese:
		return func(r rune) bool { return unicode.Is(unicode.Han, r) }
	case lang.Japanese:
		return func(r rune) bool { return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) }
	}
	return nil
}

func isLatin(target string) bool { return lang.Canonical(target) == lang.English }

// share is the part of s's words that is in the script, and how many
// words s has. A word is a run of letters of an alphabet, or one
// ideograph, kana or Hangul syllable (a Chinese name in Latin text weighs
// what an English one in Chinese text does).
func share(s string, in func(rune) bool) (float64, int) {
	n, k := 0, 0
	inWord := false
	for _, r := range s {
		switch {
		case !unicode.IsLetter(r):
			inWord = false
		case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul):
			inWord = false
			n++
			if in(r) {
				k++
			}
		case !inWord:
			inWord = true
			n++
			if in(r) {
				k++
			}
		}
	}
	if n == 0 {
		return 0, 0
	}
	return float64(k) / float64(n), n
}
