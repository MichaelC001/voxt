// Package procio is the plumbing of the child processes VoxtMini
// supervises over stdio, the helper (internal/helper) and voxtmini-llm
// (internal/llmrt): reading their protocol's lines, their stderr log and
// the last lines of it they wrote.
package procio

import (
	"bufio"
	"strings"
	"sync"

	"github.com/hehehai/VoxtMini/internal/core"
)

// ReadLine reads one line of at most max bytes; a longer one is skipped
// (an empty line comes back in its place).
func ReadLine(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	skip := false
	for {
		chunk, isPrefix, err := r.ReadLine()
		if !skip && len(buf)+len(chunk) <= max {
			buf = append(buf, chunk...)
		} else {
			buf, skip = nil, true
		}
		if err != nil || !isPrefix {
			return buf, err
		}
	}
}

// StderrLevel guesses the level of a free-form stderr line.
func StderrLevel(line string) core.Severity {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "fatal") || strings.Contains(l, "error"):
		return core.SeverityError
	case strings.Contains(l, "warning"):
		return core.SeverityWarning
	}
	return core.SeverityInfo
}

// Tail keeps the last lines added to it. Goroutine-safe.
type Tail struct {
	mu    sync.Mutex
	max   int
	lines []string
}

// NewTail keeps the last max lines.
func NewTail(max int) *Tail { return &Tail{max: max} }

// Add keeps line, dropping the oldest past max.
func (t *Tail) Add(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines = append(t.lines, line)
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
}

// String is the lines kept, one per line.
func (t *Tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, "\n")
}
