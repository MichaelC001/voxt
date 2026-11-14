//go:build !darwin

package crashlog

// redirectStderr: only the Mac app is started with standard error at
// /dev/null.
func redirectStderr(string, Options) bool { return false }
