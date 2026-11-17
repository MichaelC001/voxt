package main

import (
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/crashlog"
)

// TestCrashCard: a crash of the last run shows a card; its Report saves
// the bundle (with crash.log) and opens a GitHub issue with the crash's
// first lines, redacted; × dismisses it. A clean last run shows nothing.
func TestCrashCard(t *testing.T) {
	a, _, tt := newTestApp(t)
	logDir := t.TempDir()
	a.logDir = logDir
	path := filepath.Join(logDir, crashlog.CrashFile)
	trace := "=== VoxtMini run 2026-11-14T10:00:00Z VoxtMini 0.1.0 pid 42, go1.27 darwin/arm64 ===\n" +
		"panic: runtime error: index out of range [3] with length 3\n\ngoroutine 1 [running]:\nmain.(*app).view(0x1)\n\t/Users/someone/src/VoxtMini/shell.go:12 +0x1\n" +
		"--- told at the next start 2026-11-14T10:01:00Z ---\n=== VoxtMini run 2026-11-14T10:01:00Z VoxtMini 0.1.0 pid 43, go1.27 darwin/arm64 ===\n"
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}

	a.tellCrash(&crashlog.Run{Path: path}) // the last run ended cleanly
	if slices.ContainsFunc(a.notices, func(n core.Notification) bool { return n.ID == noticeCrash }) {
		t.Fatal("a card for a clean run")
	}
	a.tellCrash(&crashlog.Run{Path: path, Previous: "2026-11-14T10:00:00Z VoxtMini 0.1.0 pid 42"})
	a.navigate("/home")
	see(t, tt, "VoxtMini quit unexpectedly")

	var opened []string
	old := openExternal
	openExternal = func(u string) { opened = append(opened, u) }
	t.Cleanup(func() { openExternal = old })
	bundleDir = t.TempDir()
	t.Cleanup(func() { bundleDir = "" })
	click(t, tt, "Report")
	if has(tt, "VoxtMini quit unexpectedly") {
		t.Error("the card stayed after Report")
	}
	if ents, _ := os.ReadDir(bundleDir); len(ents) != 1 || !strings.HasSuffix(ents[0].Name(), ".zip") {
		t.Errorf("bundle: %v", ents)
	}
	if len(opened) != 1 {
		t.Fatalf("opened %v", opened)
	}
	u, err := url.Parse(opened[0])
	if err != nil || !strings.HasPrefix(opened[0], issuesURL+"?") {
		t.Fatalf("issue %s", opened[0])
	}
	body := u.Query().Get("body")
	for _, want := range []string{"index out of range [3]", "main.(*app).view", "pid 42", "### Diagnostics"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "someone") || strings.Contains(body, "told at the next start") {
		t.Errorf("body not redacted or cut:\n%s", body)
	}

	// Dismissed, it stays away.
	a.tellCrash(&crashlog.Run{Path: path, Previous: "x"})
	see(t, tt, "VoxtMini quit unexpectedly")
	a.dismissNotice(noticeCrash)
	if has(tt, "VoxtMini quit unexpectedly") {
		t.Error("not dismissed")
	}
}

// TestCrashCardSendFeedback: without the feedback connected, the crash
// card's Send feedback opens Settings → Feedback (the web form), and the
// card goes. With it, the feedback window opens with the crash's report
// (feedback_test.go, TestFailuresShowReportLink/crash).
func TestCrashCardSendFeedback(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.tellCrash(&crashlog.Run{Path: filepath.Join(t.TempDir(), crashlog.CrashFile), Previous: "2026-11-14T10:00:00Z VoxtMini 0.1.0 pid 42"})
	a.navigate("/home")
	see(t, tt, "VoxtMini quit unexpectedly")
	click(t, tt, "Send feedback")
	if a.shownPath() != "/settings/feedback" || !has(tt, "Send feedback on the web") {
		t.Fatalf("path %s", a.shownPath())
	}
	if slices.ContainsFunc(a.notices, func(n core.Notification) bool { return n.ID == noticeCrash }) {
		t.Error("the card stayed after Send feedback")
	}
}

func TestCrashFirstLine(t *testing.T) {
	if got := crashFirstLine("\n=== VoxtMini run x ===\npanic: boom\n\ngoroutine 1\n"); got != "panic: boom" {
		t.Errorf("%q", got)
	}
}
