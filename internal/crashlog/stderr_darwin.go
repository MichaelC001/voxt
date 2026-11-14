//go:build darwin

package crashlog

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// stderrCap is stderr.log's size past which it is moved to stderr.log.1.
var stderrCap int64 = 8 << 20

// redirectStderr points standard error at path when it is /dev/null (the
// app opened from the Finder, the Dock, at login), and keeps the file
// under stderrCap while the app runs; a terminal or a file the one who
// started it chose keeps it. Whether it did.
func redirectStderr(path string, o Options) bool {
	if !stderrIsNull() || !reopenStderr(path, o) {
		return false
	}
	go func() {
		for range time.Tick(5 * time.Minute) {
			if st, err := os.Stat(path); err == nil && st.Size() > stderrCap {
				reopenStderr(path, o) // moves it to .1; the runtime and AppKit write on to fd 2
			}
		}
	}()
	return true
}

// stderrIsNull reports whether fd 2 is /dev/null (or not open).
func stderrIsNull() bool {
	st, err := os.Stderr.Stat()
	if err != nil {
		return true
	}
	null, err := os.Stat(os.DevNull)
	return err == nil && os.SameFile(st, null)
}

// reopenStderr opens path (moved to .1 first when over the cap) as fd 2.
func reopenStderr(path string, o Options) bool {
	rotate(path, stderrCap)
	f, err := openPrivate(path)
	if err != nil {
		return false
	}
	defer f.Close()
	fmt.Fprintf(f, "=== standard error of %s ===\n", describe(o, time.Now()))
	return syscall.Dup2(int(f.Fd()), 2) == nil
}
