package main

// A crash of the app (internal/crashlog): the runtime writes its trace to
// crash.log in the logs folder, and the next start shows a card, "VoxtMini
// quit unexpectedly", whose Report saves the diagnostic bundle (crash.log
// with the other logs, redacted) and opens a new GitHub issue with the
// crash's first lines filled in, and whose Send feedback opens Settings →
// Feedback with the diagnostics on (they carry the crash, feedback.go);
// × dismisses it. Nothing is sent by itself. The helper's crashes have
// their own card (checkHelper).

import (
	"io"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/crashlog"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/actnowkit/redact"
)

// crashState is the last run's crash, as the next start found it.
type crashState struct {
	// crashLog is crash.log's path ("": no crash log this run).
	crashLog string
}

// The crash's card: its ID, and the link of its action (cardAction).
const (
	noticeCrash     = "crash"
	linkCrashReport = "action:crash-report"
)

// crashExcerpt is how much of the crash's output a report's issue takes:
// the error and the crashing goroutine's stack, within a URL's length.
const crashExcerpt = 2500

// startCrashLog points the runtime's crash output at crash.log in dir and
// says whether the last run crashed (nil: no crash log); main calls it as
// the app starts, before anything else may crash.
func startCrashLog(dir string) *crashlog.Run {
	if dir == "" {
		return nil
	}
	o := crashlog.Options{Dir: dir, Version: mygo.App.Version(), Stderr: true}
	if devOverrides() { // a published build crashes only by itself
		o.TestCrash = os.Getenv(crashlog.TestCrashEnv)
	}
	r, err := crashlog.Start(o)
	if err != nil {
		log.Printf("crash log: %v", err)
		return nil
	}
	return r
}

// consoleOut is where the log's lines go besides the log files: standard
// error, unless it is crashlog's stderr.log (the lines are in the log).
func consoleOut(r *crashlog.Run) io.Writer {
	if r != nil && r.Stderr {
		return io.Discard
	}
	return os.Stderr
}

// tellCrash shows the crash card when the last run crashed; main calls it
// once the notifications are open.
func (a *app) tellCrash(r *crashlog.Run) {
	if r == nil {
		return
	}
	a.crashLog = r.Path
	if r.Previous == "" {
		return
	}
	log.Printf("crash: the last run crashed (see %s): %s", crashlog.CrashFile, r.Previous)
	a.notes.Post(core.Notification{ID: noticeCrash, Severity: core.SeverityWarning,
		Title: i18n.T("VoxtMini quit unexpectedly"), Body: i18n.T("A report helps fix it. Nothing is sent until you submit it."),
		Link: linkCrashReport})
}

// openExternal opens a web address in the browser (a test seam).
var openExternal = func(u string) { go mygo.Shell.OpenExternal(u) }

// reportCrash is the crash card's Report: the card goes, the bundle is
// saved where the user says, then the issue opens, the crash's first
// lines in it.
func (a *app) reportCrash() {
	a.dismissNotice(noticeCrash)
	path := a.crashLog
	a.saveBundle(func() {
		a.async(func() {
			run, out := "", ""
			if path != "" {
				run, out = crashlog.Last(path, crashExcerpt)
			}
			u := a.crashReportURL(run, out)
			a.post(func() { openExternal(u) })
		})
	})
}

// crashFeedback is the crash card's Send feedback: the card goes, and
// Settings → Feedback opens on a problem with the diagnostics on.
func (a *app) crashFeedback() {
	a.dismissNotice(noticeCrash)
	a.noticesOpen = false
	a.fbForm.kind = 0 // feedback.KindBug
	a.fbForm.diagnostics = true
	a.sendFeedback()
}

// homeRE is a home folder in a path: a name the issue, which is public,
// does not need.
var homeRE = regexp.MustCompile(`/(Users|home)/[^/\s]+`)

// crashReportURL is a new GitHub issue for a crash: what the user did,
// the crash and the diagnostics, redacted (secrets, home folders).
func (a *app) crashReportURL(run, output string) string {
	diag := a.diagnosticsText()
	if len(diag) > 3000 {
		diag = diag[:3000]
	}
	var b strings.Builder
	b.WriteString("### What were you doing when it quit?\n\n\n### Crash\n\n")
	if run != "" {
		b.WriteString(redact.Text(run) + "\n\n")
	}
	b.WriteString("```\n")
	for line := range strings.Lines(output) {
		b.WriteString(redact.Text(strings.TrimRight(line, "\n")) + "\n")
	}
	b.WriteString("```\n\nPlease attach the diagnostic bundle VoxtMini saved (it has crash.log).\n\n### Diagnostics\n\n```\n" + diag + "```\n")
	body := homeRE.ReplaceAllString(b.String(), "~")
	q := url.Values{"title": {"Crash: VoxtMini quit unexpectedly"}, "body": {body}, "labels": {"bug"}}
	return issuesURL + "?" + q.Encode()
}
