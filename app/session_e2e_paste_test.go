package main

import (
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/deliver"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestSessionPasteChecked: every paste is checked (verifyMs). The focused
// field not changing after ⌘V (10-20, Arc: "pasted, nothing there, no
// word") leaves the result in the answer card saying so, the history
// says card and the log says what the field read; with secure input on in
// another app, the card says that. A check that cannot tell (a web area)
// is an insert, as before. The card's Insert checks again and says why
// when it fails once more.
func TestSessionPasteChecked(t *testing.T) {
	for _, tc := range []struct {
		name, opts string
		inserted   bool
		reason     deliver.Reason
		why        string
		again      string // the card's Insert's message
	}{
		{name: "inserted", opts: "paste-check=inserted", inserted: true},
		{name: "unknown", opts: "paste-check=unknown", inserted: true},
		// 11-13, Arc: the first ⌘V went nowhere; the helper's second one went in.
		{name: "inserted by the second ⌘V", opts: "paste-check=retried", inserted: true},
		{name: "unchanged", opts: "paste-check=unchanged", reason: deliver.ReasonUnconfirmed,
			why:   "Not inserted: the text is on the clipboard. Press ⌘V, or insert again.",
			again: "Not inserted: the text is on the clipboard, press ⌘V."},
		{name: "secure input", opts: "paste-check=unchanged,secure-input=Terminal", reason: deliver.ReasonSecureInput,
			why:   "Secure input is on in another app: the text is on the clipboard. Press ⌘V, or insert again.",
			again: "Secure input is on in Terminal: the text is on the clipboard, press ⌘V."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := script(t, hotkeyLine(0, "transcribe/0", proto.PhaseTrigger), hotkeyLine(400, "transcribe/0", proto.PhaseTrigger))
			a := newSessionApp(t, tc.opts+","+ev, func(s *settings.Settings) {
				s.Features.Transcribe.Enhance = false
			}, &fakeLocal{ASRP: fakeStreamer{final: "你好"}})
			a.pump("the end", func() bool { return a.sess.State.Ended() })
			end := a.sess
			p := a.pasted()
			if end.State != session.Done || len(p) != 1 || p[0].VerifyMS != deliver.DefaultVerifyMS || p[0].RetryMS != deliver.DefaultRetryMS {
				t.Fatalf("end %+v, pasted %+v", end, p)
			}
			if !strings.Contains(a.logText(), "delivery: focus AXTextArea in Slack") {
				t.Errorf("the log has no delivery line:\n%s", a.logText())
			}
			if tc.opts == "paste-check=retried" && !strings.Contains(a.logText(), "⌘V posted, again after 500 ms, check inserted") {
				t.Errorf("the log does not say ⌘V went twice:\n%s", a.logText())
			}
			if tc.inserted {
				if !end.Inserted || end.Card || a.answer.open {
					t.Errorf("end %+v, card %v", end, a.answer.open)
				}
				return
			}
			if end.Inserted || !end.Card || end.Delivery != tc.reason || !a.answer.open || a.answer.why != tc.why {
				t.Fatalf("end %+v, card %+v", end, a.answer)
			}
			a.pump("the card in the history", func() bool { es := a.history(); return len(es) == 1 && es[0].Delivery == "card" })
			a.insertAnswer()
			a.pump("the second paste", func() bool { return !a.answer.inserting && len(a.pasted()) == 2 })
			if !a.answer.open || a.answer.msg != tc.again {
				t.Errorf("after Insert: open %v, msg %q", a.answer.open, a.answer.msg)
			}
		})
	}
}

// logText is the app's log so far, one entry a line.
func (s *sessionApp) logText() string {
	var b strings.Builder
	for _, e := range s.logs.Tail(1000, "") {
		b.WriteString(e.Message + "\n")
	}
	return b.String()
}
