package deliver

import (
	"context"
	"testing"

	"github.com/hehehai/VoxtMini/internal/helper/proto"
)

func TestSanitize(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"plain text", "plain text"},
		{"中文，日本語のテキスト", "中文，日本語のテキスト"},
		{"tab\tand\nlines", "tab\tand\nlines"},
		{"windows\r\nline\rends", "windows\nline\nends"},
		{"echo hi\x1b[201~rm -rf ~\n", "echo hirm -rf ~\n"},  // bracketed paste's end marker
		{"\x1b[200~x\x1b[201~", "x"},                         // and its start
		{"red \x1b[31mtext\x1b[0m", "red text"},              // SGR
		{"title\x1b]0;evil\x07 after", "title after"},        // OSC to BEL
		{"link\x1b]8;;https://x\x1b\\ok", "linkok"},          // OSC to ST
		{"esc\x1bc reset", "esc reset"},                      // ESC + one byte
		{"lone esc\x1b", "lone esc"},                         // at the end
		{"c1 \u009b201~csi \u0085nel", "c1 csi nel"},         // C1 CSI and NEL
		{"nul\x00 bel\x07 bs\x08 del\x7f", "nul bel bs del"}, // C0 and DEL
		{"zero​width‍join \U0001F468‍\U0001F469", "zero​width‍join \U0001F468‍\U0001F469"},
		{"\x1b[", ""},
		{"csi\x1b[12\nnext", "csi\nnext"}, // a line break ends a broken sequence
	} {
		if got := Sanitize(c.in); got != c.want {
			t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if !Terminal("com.googlecode.iterm2") || Terminal("com.apple.TextEdit") {
		t.Error("Terminal")
	}
}

// TestInsertSanitizes: what is pasted is sanitized; into a terminal it
// keeps no trailing line break, and text with a line break gets no auto
// key there.
func TestInsertSanitizes(t *testing.T) {
	term := proto.App{PID: 1702, BundleID: "com.apple.Terminal", Name: "Terminal"}
	enter := []proto.KeyPress{{Key: 36}} // Return
	posted := proto.PasteResult{Posted: true, Restored: true}
	for _, c := range []struct {
		name   string
		target proto.App
		text   string
		want   string
		press  bool
		reason Reason
	}{
		{name: "escapes dropped", target: slack, text: "hi\x1b[201~\r\n", want: "hi\n", press: true},
		{name: "terminal, one line", target: term, text: "ls -la\n\n", want: "ls -la", press: true},
		{name: "terminal, two lines", target: term, text: "echo a\nrm -rf ~\n", want: "echo a\nrm -rf ~"},
		{name: "terminal, line breaks only", target: term, text: "\n\r\n", reason: ReasonEmpty},
		{name: "control characters only", target: slack, text: "\x1b[0m\x07", reason: ReasonEmpty},
	} {
		t.Run(c.name, func(t *testing.T) {
			io := &fakeIO{focus: []proto.FocusResult{{PID: c.target.PID, Role: "AXTextArea", Editable: true}}, paste: posted}
			res := Insert(context.Background(), io, Request{Text: c.text, Target: c.target, Press: enter})
			if res.Reason != c.reason {
				t.Fatalf("reason %q, want %q", res.Reason, c.reason)
			}
			if c.reason != ReasonNone {
				if len(io.pasted) != 0 {
					t.Errorf("pasted %+v", io.pasted)
				}
				return
			}
			if len(io.pasted) != 1 || io.pasted[0].Text != c.want || (len(io.pasted[0].Press) > 0) != c.press {
				t.Errorf("pasted %+v, want %q press %v", io.pasted, c.want, c.press)
			}
		})
	}
}
