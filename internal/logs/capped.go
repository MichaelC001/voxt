package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// CappedMax is the size at which a CappedFile starts again (10 MB).
const CappedMax = 10 << 20

// CappedFile is a child process's stderr log (helper.log, llm.log): lines
// appended to path, which moves to path.1 (the one previous file kept)
// once it passes max bytes, at its opening and while it is written, so
// that a process that runs for weeks in an error loop does not fill the
// disk. Safe for concurrent use; a nil *CappedFile writes nothing.
type CappedFile struct {
	path string
	max  int64

	mu   sync.Mutex
	f    *os.File
	size int64
}

// OpenCapped opens path for appending, capped at max bytes (0:
// CappedMax); nil when it cannot.
func OpenCapped(path string, max int64) *CappedFile {
	if max <= 0 {
		max = CappedMax
	}
	c := &CappedFile{path: path, max: max}
	if st, err := os.Stat(path); err == nil && st.Size() > max {
		os.Rename(path, path+".1")
	}
	if !c.open() {
		return nil
	}
	return c
}

func (c *CappedFile) open() bool {
	os.MkdirAll(filepath.Dir(c.path), 0o755)
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false
	}
	c.f, c.size = f, 0
	if st, err := f.Stat(); err == nil {
		c.size = st.Size()
	}
	return true
}

// Println appends line and a newline, starting a new file first when the
// line would take this one past its cap.
func (c *CappedFile) Println(line string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f == nil {
		return
	}
	if c.size > 0 && c.size+int64(len(line))+1 > c.max {
		c.f.Close()
		os.Rename(c.path, c.path+".1")
		if !c.open() {
			c.f = nil
			return
		}
	}
	n, _ := fmt.Fprintln(c.f, line)
	c.size += int64(n)
}

// Close closes the file.
func (c *CappedFile) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f != nil {
		c.f.Close()
		c.f = nil
	}
}
