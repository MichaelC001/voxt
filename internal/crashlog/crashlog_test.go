package crashlog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// childEnv makes TestMain a run of the app: Start in the folder it names,
// then a clean end or the crash TestCrashEnv asks for.
const childEnv = "VOXTMINI_TEST_CRASHLOG_DIR"

func TestMain(m *testing.M) {
	if dir := os.Getenv(childEnv); dir != "" {
		testCrashAfter = 10 * time.Millisecond
		crash := os.Getenv(TestCrashEnv)
		r, err := Start(Options{Dir: dir, Version: "1.2.3", TestCrash: crash})
		if err != nil {
			fmt.Println("start:", err)
			os.Exit(9)
		}
		fmt.Printf("previous=%q\n", r.Previous)
		if crash == "" {
			os.Exit(0)
		}
		time.Sleep(time.Minute) // the crash comes first
		os.Exit(8)
	}
	os.Exit(m.Run())
}

// run starts the "app" once in dir; crash "" ends it cleanly. Its output
// and exit status.
func run(t *testing.T, dir, crash string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), childEnv+"="+dir, "VOXTMINI_TEST_SECRET=hunter2-in-the-environment")
	if crash != "" {
		cmd.Env = append(cmd.Env, TestCrashEnv+"="+crash)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

// TestCrashLog: runs that end cleanly leave nothing in crash.log; one that
// panics leaves its header and the runtime's output (never the
// environment), which the next start tells once and Last reads.
func TestCrashLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CrashFile)
	read := func() string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if out, code := run(t, dir, ""); code != 0 || !strings.Contains(out, `previous=""`) {
		t.Fatalf("first run: %d %s", code, out)
	}
	if s := read(); strings.Count(s, header) != 1 || !strings.Contains(s, "VoxtMini 1.2.3 pid ") {
		t.Fatalf("after a clean run: %q", s)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("crash.log is %v", st.Mode().Perm())
	}
	if run, _ := Last(path, 1000); run != "" {
		t.Fatalf("Last after a clean run: %q", run)
	}
	out, code := run(t, dir, "panic")
	if code != 2 || !strings.Contains(out, `previous=""`) {
		t.Fatalf("the crash: %d %s", code, out)
	}
	s := read()
	if strings.Count(s, header) != 1 || !strings.Contains(s, "panic: crashlog: a test crash (VOXTMINI_TEST_CRASH=panic)") || !strings.Contains(s, "goroutine ") {
		t.Fatalf("after the crash: %q", s)
	}
	if strings.Contains(s, "hunter2") {
		t.Fatalf("the environment is in crash.log: %q", s)
	}
	crashed := strings.TrimSuffix(strings.TrimPrefix(strings.SplitN(s, "\n", 2)[0], header), " ===")
	if out, code := run(t, dir, ""); code != 0 || !strings.Contains(out, fmt.Sprintf("previous=%q", crashed)) {
		t.Fatalf("the start after the crash: %d %s (want %q)", code, out, crashed)
	}
	if out, code := run(t, dir, ""); code != 0 || !strings.Contains(out, `previous=""`) {
		t.Fatalf("told twice: %d %s", code, out)
	}
	if s := read(); !strings.HasPrefix(s, header+crashed) || strings.Count(s, header) != 2 || strings.Count(s, seen) != 1 {
		t.Fatalf("at the end: %q", s)
	}
	// Last: the crashed run, its output without the start's mark, cut.
	r, o := Last(path, 40)
	if r != crashed || !strings.HasPrefix(o, "panic: crashlog: a test crash") || len(o) != 40 {
		t.Fatalf("Last: %q %q", r, o)
	}
}

// TestCrashLogOverflow: a stack overflow is in crash.log too.
func TestCrashLogOverflow(t *testing.T) {
	dir := t.TempDir()
	if out, code := run(t, dir, "overflow"); code != 2 {
		t.Fatalf("the overflow: %d %.2000s", code, out)
	}
	b, _ := os.ReadFile(filepath.Join(dir, CrashFile))
	if s := string(b); !strings.Contains(s, "runtime.newstack") || !strings.Contains(s, "crashlog.overflow") {
		t.Fatalf("crash.log: %.3000s", s)
	}
}

// TestCrashLogCaps: a crash.log over the cap moves to crash.log.1 at the
// start; a header cut off is no crash; output cut mid-line still lets the
// next header start a line.
func TestCrashLogCaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, CrashFile)
	old := crashCap
	crashCap = 64
	t.Cleanup(func() { crashCap = old })
	os.WriteFile(path, []byte(header+"x ===\n"+strings.Repeat("goroutine 1\n", 20)), 0o644)
	prev, err := previous(path)
	if err != nil || prev != "" { // the header is past what is read
		t.Fatalf("previous %q %v", prev, err)
	}
	rotate(path, crashCap)
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatal("not moved:", err)
	}
	crashCap = old
	os.WriteFile(path, []byte(header+"a ===\npanic: cut"), 0o644)
	if prev, _ := previous(path); prev != "a" {
		t.Fatalf("previous %q", prev)
	}
	f, _ := openPrivate(path)
	if endsLine(f) {
		t.Fatal("ends a line")
	}
	f.Close()
}
