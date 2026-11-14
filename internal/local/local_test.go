package local

import (
	"context"
	"errors"
	"testing"
)

func TestChoose(t *testing.T) {
	list := []Model{{Kind: ASR, ID: "a", Name: "A"}, {Kind: ASR, ID: "b", Name: "B"}}
	for _, c := range []struct {
		want, got string
	}{{"b", "b"}, {" b ", "b"}, {"", "a"}, {"gone", "a"}} {
		if m, ok := Choose(list, c.want); !ok || m.ID != c.got {
			t.Errorf("Choose(%q) = %+v %v, want %s", c.want, m, ok, c.got)
		}
	}
	if m, ok := Choose(nil, "a"); ok {
		t.Errorf("Choose of nothing = %+v", m)
	}
	// A remote service only when chosen: automatic, or a choice that is
	// gone, is the first local model, or none.
	r := Model{Kind: ASR, ID: "remote:x", Remote: true}
	mixed := []Model{r, list[1]}
	for _, c := range []struct{ want, got string }{{"remote:x", "remote:x"}, {"", "b"}, {"remote:gone", "b"}} {
		if m, ok := Choose(mixed, c.want); !ok || m.ID != c.got {
			t.Errorf("Choose(mixed, %q) = %+v %v, want %s", c.want, m, ok, c.got)
		}
	}
	if m, ok := Choose([]Model{r}, ""); ok {
		t.Errorf("automatic picked a remote service: %+v", m)
	}
	if l := Locals(mixed); len(l) != 1 || l[0].ID != "b" {
		t.Errorf("Locals %v", l)
	}
}

func TestNotInstalled(t *testing.T) {
	var s Services = NotInstalled{}
	if l := s.Installed(ASR); len(l) != 0 {
		t.Errorf("installed %v", l)
	}
	p, _, err := s.ASR(context.Background(), "")
	if p != nil || !errors.Is(err, ErrNotInstalled) || err.Error() != "local: model not installed: no asr model" {
		t.Errorf("ASR %v %v", p, err)
	}
	c, _, err := s.LLM(context.Background(), "qwen")
	if c != nil || !errors.Is(err, ErrNotInstalled) || err.Error() != `local: model not installed: llm model "qwen"` {
		t.Errorf("LLM %v %v", c, err)
	}
}
