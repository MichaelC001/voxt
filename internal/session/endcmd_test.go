package session

import (
	"testing"

	"github.com/hehehai/VoxtMini/internal/settings"
)

func TestStripEnd(t *testing.T) {
	for _, tc := range []struct{ text, word, want string }{
		{"Send it now, over.", "over", "Send it now"},
		{"Send it now OVER", "over", "Send it now"},
		{"send it now ｏｖｅｒ。", "over", "send it now"},
		{"over", "over", ""},
		{"It's a takeover", "over", "It's a takeover"},
		{"over and out", "over", "over and out"},
		{"Done, that's all.", "that's  all", "Done"},
		{"今天的会议到此为止完毕。", "完毕", "今天的会议到此为止"},
		{"今天的会议，完毕", "完毕", "今天的会议"},
		{"本日の報告は以上です", "以上", "本日の報告は以上です"},
		{"本日の報告は以上。", "以上", "本日の報告は"},
		{"hello", "", "hello"},
		{"", "over", ""},
	} {
		if got := StripEnd(tc.text, tc.word); got != tc.want {
			t.Errorf("StripEnd(%q, %q) = %q, want %q", tc.text, tc.word, got, tc.want)
		}
	}
}

func TestEndWord(t *testing.T) {
	s := settings.Defaults()
	for _, tc := range []struct{ main, locale, word, want string }{
		{"", "en", "", "over"},
		{"", "zh-Hans", "", "完毕"},
		{"ja", "en", "", "以上"},
		{"zh", "en", " roger ", "roger"},
	} {
		s.General.MainLanguage, s.Features.EndCommand.Word = tc.main, tc.word
		if got := EndWord(s, tc.locale); got != tc.want {
			t.Errorf("EndWord(%q, %q, %q) = %q, want %q", tc.main, tc.locale, tc.word, got, tc.want)
		}
	}
}

// TestEndCommand: the word at the end of the live text and a second of
// quiet stop the recording by themselves; the word leaves the result.
// Without the word, or with the command off, the recording goes on.
func TestEndCommand(t *testing.T) {
	const spoken = 10 // frames of 100 ms, then quiet
	for _, tc := range []struct {
		name, partial, final string
		on, auto             bool
		want                 string
	}{
		{"word", "hello over", "Hello, over.", true, true, "Hello"},
		{"no word", "hello", "Hello.", true, false, "Hello."},
		{"off", "hello over", "Hello, over.", false, false, "Hello, over."},
		{"manual stop strips it too", "hello over to you", "Hello, over.", true, false, "Hello"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, &fakeStreamer{partial: tc.partial, final: tc.final}, nil, func(_ *Config, s *settings.Settings) {
				s.Features.EndCommand = settings.EndCommand{On: tc.on, Word: "over"}
				s.Features.Transcribe.Enhance = false
				s.Dictionary.Enabled = false
			})
			h.audio.speaks = func(n int) bool { return n < spoken }
			id := h.start()
			if !tc.auto {
				h.speak(id, 40) // 3 s of quiet: still recording
			}
			end := h.rec.wait(t)
			if end.State != Done || end.Result != tc.want {
				t.Fatalf("end %s %q (%+v)", end.State, end.Result, end.Problem)
			}
			if !tc.auto {
				return
			}
			// The speech, the detector's hangover and a second of quiet.
			if n := h.audio.recorded(); n < spoken+10 || n > spoken+30 {
				t.Errorf("stopped after %d frames", n)
			}
		})
	}
}
