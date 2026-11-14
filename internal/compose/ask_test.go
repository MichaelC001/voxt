package compose

import (
	"strings"
	"testing"
)

func TestAsk(t *testing.T) {
	in := AskInput{MainLanguage: "zh", Kind: "rewrite", Created: "2026-11-14 09:30",
		Result: "周四下午开会。", Source: "meeting thursday", Instruct: "translate to Chinese", Question: " When is it? "}
	m := Ask(in)
	if len(m) != 2 || m[0].Role != "system" || !strings.Contains(m[0].Content, "追问助手") || !strings.Contains(m[0].Content, "用简体中文回答") {
		t.Fatalf("%q", m)
	}
	if strings.Contains(m[0].Content, "周四") || strings.Contains(m[0].Content, "{{") {
		t.Errorf("the entry or a variable in the system prompt: %q", m[0].Content)
	}
	u := m[1].Content
	for _, s := range []string{`<entry kind="rewrite" created="2026-11-14 09:30">`, "<selection>\nmeeting thursday\n</selection>", "<instruction>\ntranslate to Chinese", "<result>\n周四下午开会。\n</result>", "</entry>\n\nWhen is it?"} {
		if !strings.Contains(u, s) {
			t.Errorf("user message lacks %q: %q", s, u)
		}
	}

	// Earlier turns: the entry once, in the first question.
	in.MainLanguage, in.Source, in.Instruct = "", "", ""
	in.Turns = []AskTurn{{"Who?", "The team."}, {"Where?", "Online."}}
	m = Ask(in)
	if len(m) != 6 || m[1].Role != "user" || m[2].Content != "The team." || m[5].Content != "When is it?" || !strings.Contains(m[0].Content, "Reply in English") {
		t.Fatalf("%q", m)
	}
	if !strings.HasPrefix(m[1].Content, "<entry") || !strings.HasSuffix(m[1].Content, "Who?") || strings.Contains(m[3].Content, "<entry") || strings.Contains(m[1].Content, "<selection>") {
		t.Errorf("turns: %q", m)
	}
}
