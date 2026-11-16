package logscrub

import (
	"strings"
	"testing"
)

func TestLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{`session 01J9: recognised "Meet Anna at the Berlin office" in 1.2s`, `session 01J9: recognised "…" in 1.2s`},
		{`session 01J9: composed "Dear Anna, …" in 800ms`, `session 01J9: composed "…" in 800ms`},
		{`session 01J9: recognised 31 chars in 1.2s`, `session 01J9: recognised 31 chars in 1.2s`},
		{`session 01J9: app enhancement: Client Acme emails by url`, `session 01J9: app enhancement: … by url`},
		{`session 01J9: auto key "Cmd+Enter" of Client Acme: no such key`, `session 01J9: auto key "…" of …: no such key`},
		{`session 01J9: recording from Anna's AirPods Pro: not connected`, `session 01J9: recording from …: not connected`},
		{`session 01J9: address of com.google.Chrome read in 12ms (mail.acme.com)`, `session 01J9: address of com.google.Chrome read in 12ms (mail.acme.com)`},
		{`updates: GET https://actnow.dev/updates/voxtmini/beta?x=1 503`, `updates: GET https://actnow.dev/updates/voxtmini/beta?… 503`},
		{`asr: Post "https://asr.example.com/v1/audio/transcriptions?key=1": EOF`, `asr: Post "https://asr.example.com/…": EOF`},
		{`history: saved ~/Documents/VoxtMini recordings/Interview.m4a`, `history: saved ~/Documents/…/….m4a`},
		{`history: could not save ~/Documents/Interview with Anna.m4a: disk full`, `history: could not save ~/Documents/….m4a: disk full`},
		{`omni: loading ~/.cache/huggingface/hub/models--mlx-community--Qwen3-ASR-0.6B-8bit/snapshots/0123abcd4567/config.json`, `omni: loading ~/.cache/huggingface/hub/models--mlx-community--Qwen3-ASR-0.6B-8bit/snapshots/0123abcd4567/config.json`},
		{`data folder ~/Library/Application Support/VoxtMini/logs/voxtmini.log`, `data folder ~/Library/Application Support/VoxtMini/logs/voxtmini.log`},
	}
	for _, c := range cases {
		if got := Line(c.in); got != c.want {
			t.Errorf("Line(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestLineLeavesNoUserText(t *testing.T) {
	in := `session 01: selection: "Quarterly numbers for Acme" via ax in 3 ms; wrote "Acme report draft" in 1s; ` +
		`app enhancement: Acme Mail by bundle; recording from Acme Mic: gone; open https://docs.example.com/acme/q3/plan?id=7`
	out := Line(in)
	for _, w := range []string{"Acme", "acme", "Quarterly", "report", "plan"} {
		if strings.Contains(out, w) {
			t.Errorf("%q still has %q", out, w)
		}
	}
	if !strings.Contains(out, "https://docs.example.com/…") {
		t.Errorf("the link's site is gone: %q", out)
	}
}
