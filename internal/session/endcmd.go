package session

import (
	"strings"
	"sync"
	"unicode"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/lang"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/vad"
)

// The voice end command (Voxt's VoiceEndCommandFlow, simpler): with
// settings.EndCommand on, a recording whose live text ends with the word
// stops by itself once the microphone has been quiet for EndSilence, and
// the word is taken off the end of the recognised text (also after a
// manual stop, and with a speech model that shows no live text, which
// cannot stop by itself). Behaviour notes: docs/behavior.md 5.43.

// EndSilence is the quiet after the word that stops the recording, from
// the end of the speech as the energy detector (internal/vad) hears it:
// Voxt's 1 s.
const EndSilence = 1000 // ms

// EndWords are the words offered in Settings → Shortcuts; a user may type
// another one.
var EndWords = []string{"over", "end", "完毕", "以上"}

// EndWord is the end command's word for settings s: the user's, else the
// main language's ("完毕" for Chinese, "以上" for Japanese, else "over").
func EndWord(s settings.Settings, locale string) string {
	if w := strings.TrimSpace(s.Features.EndCommand.Word); w != "" {
		return w
	}
	switch lang.Canonical(MainLanguage(s, locale)) {
	case lang.Chinese:
		return "完毕"
	case lang.Japanese:
		return "以上"
	}
	return "over"
}

// endTail is where word starts at the end of text (its trailing spaces
// and punctuation aside), or -1 when text does not end with it. Letters
// are compared without case and full-width forms; a word in a script
// with spaces (Latin) must follow a space or punctuation ("takeover" does
// not end with "over"), one in Chinese or Japanese need not (their text
// has no spaces).
func endTail(text, word string) int {
	w := []rune(foldEnd(strings.Join(strings.Fields(word), " ")))
	if len(w) == 0 {
		return -1
	}
	r := []rune(text)
	end := len(r)
	for end > 0 && endDelim(r[end-1]) {
		end--
	}
	start := end - len(w)
	if start < 0 || foldEnd(string(r[start:end])) != string(w) {
		return -1
	}
	if start > 0 && !endDelim(r[start-1]) && !cjk(w[0]) {
		return -1
	}
	return len(string(r[:start]))
}

// StripEnd takes word (and the punctuation and spaces before it) off the
// end of text; text unchanged when it does not end with word.
func StripEnd(text, word string) string {
	i := endTail(text, word)
	if i < 0 {
		return text
	}
	return strings.TrimRightFunc(text[:i], endDelim)
}

// endDelim: spaces, punctuation and symbols around the word.
func endDelim(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) }

// cjk: Han, kana (a word in a script written without spaces).
func cjk(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}

// foldEnd lowercases s and turns full-width ASCII into ASCII.
func foldEnd(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		}
		return unicode.ToLower(r)
	}, s)
}

// endWatch watches a recording for the end command: the live text's tail
// (heard) and the silence after the speech (audio, from the recording's
// PCM). Goroutine-safe: the stream's reader and the helper's reader call
// it. stop runs once, when both say so.
type endWatch struct {
	word string
	stop func()

	mu      sync.Mutex
	det     *vad.Detector
	samples int  // PCM samples heard
	last    int  // the sample where speech was last heard; -1: none yet
	tail    bool // the live text ends with the word
	fired   bool
}

// newEndWatch is a watch for word that calls stop.
func newEndWatch(word string, p vad.Params, stop func()) *endWatch {
	return &endWatch{word: word, stop: stop, det: vad.NewDetector(asr.SampleRate, p), last: -1}
}

// heard takes the live text so far.
func (w *endWatch) heard(text string) {
	w.mu.Lock()
	w.tail = endTail(text, w.word) >= 0
	w.mu.Unlock()
}

// audio takes the recording's next PCM, and stops the recording when the
// word was said and the silence after it lasted EndSilence.
func (w *endWatch) audio(pcm []byte) {
	w.mu.Lock()
	w.det.Write(pcm)
	w.samples += len(pcm) / 2
	if w.det.Speaking() {
		w.last = w.samples
	}
	due := !w.fired && w.tail && w.last >= 0 && (w.samples-w.last)*1000 >= EndSilence*asr.SampleRate
	if due {
		w.fired = true
	}
	w.mu.Unlock()
	if due {
		w.stop()
	}
}

// Fired reports whether the watch stopped the recording.
func (w *endWatch) Fired() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fired
}

// withoutEndWord is the recognised text raw without the end command's
// word at its end, when the command is on.
func (r *run) withoutEndWord(raw string) string {
	if !r.settings.Features.EndCommand.On {
		return raw
	}
	text := StripEnd(raw, EndWord(r.settings, r.m.cfg.Locale()))
	if text != raw {
		r.logf("info", "voice end command: the word taken off the end (stopped by it: %v)", r.endw != nil && r.endw.Fired())
	}
	return text
}
