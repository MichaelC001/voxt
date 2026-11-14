// Package crashlog keeps what a crash of VoxtMini leaves behind. Opened
// from the Finder, the Dock or at login, the app's standard error is
// /dev/null, where the Go runtime writes a crash's trace: without this
// package an unrecovered panic or a fatal error left nothing to read.
//
// Start, as the process begins:
//   - points the runtime's crash output (an unrecovered panic on any
//     goroutine, a fatal error such as a stack overflow or a concurrent
//     map write, a signal it caught) at crash.log in the logs folder too
//     (debug.SetCrashOutput), after a header line for the run (time,
//     version, pid). The output is the runtime's: the error and the
//     goroutines' stacks, no environment;
//   - on the Mac, points standard error at stderr.log when it is
//     /dev/null (not a terminal or a file the one who started it chose):
//     native warnings and the lines logged before the log opens.
//
// No recover() wraps the app's goroutines: a panic recovered there would
// leave the app running in a state nobody planned for; the crash and its
// report on the next start are the honest outcome.
//
// The next start tells whether the last run crashed (Run.Previous): a run
// that ends without crashing leaves only its header, which the next start
// takes away again, so crash.log holds the runs that crashed, and Last
// reads the latest one's output for a report. Both files are the user's
// alone (0600) and kept small: crash.log moves to crash.log.1 past 2 MB at
// a start, stderr.log to stderr.log.1 past 8 MB. The diagnostic bundle
// takes them with the other logs, redacted (settings_about.go).
//
// Port of Inboxly's internal/crashlog (same author), without its SIGABRT
// for macOS's own crash report: the trace in crash.log is what a report
// needs.
//
// Imports: none of VoxtMini's.
package crashlog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// The files, in the logs folder.
const (
	CrashFile  = "crash.log"
	StderrFile = "stderr.log"
)

// crashCap is crash.log's size past which a start moves it to crash.log.1.
var crashCap int64 = 2 << 20

// header starts each run's part of crash.log; seen ends the part of a
// run whose crash the next start told (it is told once).
const (
	header = "=== VoxtMini run "
	seen   = "--- told at the next start "
)

// Options says where and for which VoxtMini.
type Options struct {
	Dir     string // the logs folder
	Version string // "" dev
	// Stderr: standard error to stderr.log when it is /dev/null (darwin).
	Stderr bool
	// TestCrash crashes the process on purpose a moment after Start, to
	// check what a crash leaves end to end: "panic" an unrecovered panic,
	// "overflow" a stack overflow (development builds: main reads
	// TestCrashEnv only there).
	TestCrash string
}

// TestCrashEnv is the variable development builds read into
// Options.TestCrash.
const TestCrashEnv = "VOXTMINI_TEST_CRASH"

// Run is the running process's crash log.
type Run struct {
	// Path is crash.log's.
	Path string
	// Previous describes the last run when it crashed ("": it did not, or
	// there was none): its header's time, version and pid.
	Previous string
	// Stderr: standard error goes to stderr.log.
	Stderr bool
}

// Start points the runtime's crash output at crash.log in o.Dir and says
// whether the last run crashed. Once, as the process begins.
func Start(o Options) (*Run, error) {
	if o.Dir == "" {
		return nil, errors.New("crashlog: no folder")
	}
	if err := os.MkdirAll(o.Dir, 0o755); err != nil {
		return nil, err
	}
	r := &Run{Path: filepath.Join(o.Dir, CrashFile)}
	prev, err := previous(r.Path)
	if err != nil {
		return nil, err
	}
	r.Previous = prev
	rotate(r.Path, crashCap)
	f, err := openPrivate(r.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close() // SetCrashOutput keeps a copy of the descriptor
	now := time.Now()
	line := header + describe(o, now) + " ===\n"
	if prev != "" {
		line = seen + now.Format(time.RFC3339) + " ---\n" + line
	}
	if !endsLine(f) { // a crash's output cut off mid-line
		line = "\n" + line
	}
	if _, err := io.WriteString(f, line); err != nil {
		return nil, err
	}
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		return nil, err
	}
	if o.Stderr {
		r.Stderr = redirectStderr(filepath.Join(o.Dir, StderrFile), o)
	}
	if o.TestCrash != "" {
		os.Unsetenv(TestCrashEnv) // not for the processes VoxtMini starts
		time.AfterFunc(testCrashAfter, func() { crashNow(o.TestCrash) })
	}
	return r, nil
}

// describe is the run: when, which VoxtMini, which process.
func describe(o Options, at time.Time) string {
	v := o.Version
	if v == "" {
		v = "dev"
	}
	return fmt.Sprintf("%s VoxtMini %s pid %d, %s %s/%s",
		at.Format(time.RFC3339), v, os.Getpid(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// previous reads crash.log at path: the header of its last run, when
// something follows it (that run crashed) and its crash was not told yet;
// a last run that left only its header is taken away (it ended cleanly).
func previous(path string) (string, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	from, data, err := tail(f)
	if err != nil {
		return "", err
	}
	at := bytes.LastIndex(data, []byte("\n"+header)) + 1 // 0: at the start, or none
	if !bytes.HasPrefix(data[at:], []byte(header)) {
		return "", nil // not ours, or a header cut off: nothing to say
	}
	line, rest, _ := bytes.Cut(data[at:], []byte("\n"))
	if len(bytes.TrimSpace(rest)) == 0 {
		return "", f.Truncate(from + int64(at))
	}
	if bytes.Contains(rest, []byte("\n"+seen)) || bytes.HasPrefix(rest, []byte(seen)) {
		return "", nil
	}
	return strings.TrimSuffix(strings.TrimSpace(string(line[len(header):])), " ==="), nil
}

// tail reads the last crashCap bytes of f: where they start, and them.
func tail(f *os.File) (int64, []byte, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, nil, err
	}
	from := max(0, st.Size()-crashCap)
	data := make([]byte, st.Size()-from)
	if _, err := f.ReadAt(data, from); err != nil && !errors.Is(err, io.EOF) {
		return 0, nil, err
	}
	return from, data, nil
}

// Last is the latest crash in crash.log at path: its run's description
// and the runtime's output, at most limit bytes of it ("": none). I/O.
func Last(path string, limit int) (run, output string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	_, data, err := tail(f)
	if err != nil {
		return "", ""
	}
	// The runs, newest last; the newest with something after its header.
	parts := bytes.Split(data, []byte(header))
	for i := len(parts) - 1; i >= 1; i-- {
		line, rest, _ := bytes.Cut(parts[i], []byte("\n"))
		if j := bytes.Index(rest, []byte(seen)); j >= 0 {
			rest = rest[:j]
		}
		rest = bytes.TrimSpace(rest)
		if len(rest) == 0 {
			continue
		}
		if len(rest) > limit {
			rest = rest[:limit]
		}
		return strings.TrimSuffix(strings.TrimSpace(string(line)), " ==="), string(rest)
	}
	return "", ""
}

// rotate moves the file at path to path.1 when it is over limit.
func rotate(path string, limit int64) {
	if st, err := os.Stat(path); err == nil && st.Size() > limit {
		os.Rename(path, path+".1")
	}
}

// endsLine reports whether f is empty or ends with a newline.
func endsLine(f *os.File) bool {
	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return true
	}
	b := make([]byte, 1)
	n, _ := f.ReadAt(b, st.Size()-1)
	return n == 0 || b[0] == '\n'
}

// openPrivate opens path to append, readable by the user alone.
func openPrivate(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	f.Chmod(0o600) // made before under another umask
	return f, nil
}

// testCrashAfter is how long after Start a test crash comes.
var testCrashAfter = 3 * time.Second

// crashNow crashes the process as kind says.
func crashNow(kind string) {
	if kind == "overflow" {
		debug.SetMaxStack(64 << 20) // soon
		overflow(0)
	}
	panic("crashlog: a test crash (" + TestCrashEnv + "=" + kind + ")")
}

// overflow calls itself until the stack runs out.
func overflow(n int) int {
	var pad [128]byte
	pad[n%len(pad)] = byte(n)
	return overflow(n+1) + int(pad[0])
}
