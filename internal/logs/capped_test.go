package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCappedFile: a file past its cap at the opening moves to .1; while
// written, it moves again before a line would pass the cap, keeping one
// previous file; nil writes nothing.
func TestCappedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "helper.log")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(strings.Repeat("x", 200)+"\n"), 0o644)
	c := OpenCapped(path, 100)
	if c == nil {
		t.Fatal("not opened")
	}
	if b, _ := os.ReadFile(path + ".1"); len(b) != 201 {
		t.Errorf(".1 has %d bytes", len(b))
	}
	line := strings.Repeat("a", 39) // 40 bytes a line
	for range 2 {
		c.Println(line)
	}
	if b, _ := os.ReadFile(path); len(b) != 80 {
		t.Errorf("%d bytes before the cap", len(b))
	}
	c.Println("third " + line[6:]) // 40 more: past 100
	if b, _ := os.ReadFile(path); string(b) != "third "+line[6:]+"\n" {
		t.Errorf("after the cap: %q", b)
	}
	if b, _ := os.ReadFile(path + ".1"); len(b) != 80 {
		t.Errorf(".1 after the cap: %d bytes", len(b))
	}
	for range 100 {
		c.Println(line)
	}
	c.Close()
	c.Println("closed") // nothing, no panic
	for _, p := range []string{path, path + ".1"} {
		if st, err := os.Stat(p); err != nil || st.Size() > 100 {
			t.Errorf("%s: %v %v", p, st.Size(), err)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "sub", "*")); len(m) != 2 {
		t.Errorf("files %v", m)
	}
	var none *CappedFile
	none.Println("x")
	none.Close()
}
