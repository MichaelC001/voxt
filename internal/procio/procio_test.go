package procio

import (
	"bufio"
	"io"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/core"
)

// TestReadLine: lines up to max come back whole, across the reader's
// buffer; a longer one comes back empty and the next one is whole.
func TestReadLine(t *testing.T) {
	long := strings.Repeat("x", 100)
	r := bufio.NewReaderSize(strings.NewReader("short\n"+long+"\n"+strings.Repeat("y", 40)+"\nlast"), 16)
	var got []string
	for {
		line, err := ReadLine(r, 50)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(line))
	}
	want := []string{"short", "", strings.Repeat("y", 40), "last"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %q", got)
	}
}

func TestTail(t *testing.T) {
	tl := NewTail(2)
	for _, l := range []string{"a", "b", "c"} {
		tl.Add(l)
	}
	if tl.String() != "b\nc" {
		t.Errorf("%q", tl.String())
	}
}

func TestStderrLevel(t *testing.T) {
	for line, want := range map[string]core.Severity{
		"Fatal error: x": core.SeverityError, "an ERROR": core.SeverityError,
		"warning: slow": core.SeverityWarning, "loaded": core.SeverityInfo,
	} {
		if got := StderrLevel(line); got != want {
			t.Errorf("%q: %v, want %v", line, got, want)
		}
	}
}
