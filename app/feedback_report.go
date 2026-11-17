package main

// "Report this problem" beside the failures VoxtMini shows, and the
// recent actions the error reports carry (actnowkit's Capture and Step).
// The codes (E-<area>-<number>) are VoxtMini's own and stable across
// versions: the site groups the reports by them. Where is a place in the
// app, never the user's content (no text, file, service or app name).

import (
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/actnowkit/feedback"
	fbui "github.com/hehehai/actnowkit/feedbackui"

	"github.com/hehehai/VoxtMini/internal/i18n"
)

// VoxtMini's error codes and kinds of the failures with a report link.
const (
	codeSession = "E-SES-0001" // a dictation failed (History → its details)
	codeTry     = "E-TRY-0001" // a try failed (a feature's page, Home, the setup guide)
	codeHelper  = "E-HLP-0001" // the helper stopped (its card in the sidebar)
	codeModel   = "E-MDL-0001" // a local model's download failed (Models → its card)
	codeCrash   = "E-CRS-0001" // VoxtMini quit unexpectedly (the crash card, next start)
	codeHistory = "E-HIS-0001" // the history could not be read

	kindModelDownload = "model_download_failed"
	kindCrash         = "app_crashed"
	kindHistoryRead   = "history_read_failed"
)

// failure is a failure shown: key names it among the others (an entry's
// or a session's id: a new key or a new message is a new report), the
// rest is its context.
type failure struct {
	key, title, code, kind, where, message string
}

// fbReport is the failure's report: captured off the main thread the
// first time the failure shows (Capture reads the log and writes the
// report: the build pass does no I/O), nil until it is ready and without
// the feedback.
func (a *app) fbReport(f failure) *feedback.Report {
	m := a.fbMgr
	if m == nil {
		return nil
	}
	k := f.key + "\x00" + f.code + "\x00" + f.kind + "\x00" + f.message
	a.fbRepMu.Lock()
	rep, seen := a.fbReports[k]
	if !seen {
		if a.fbReports == nil || len(a.fbReports) > 200 {
			a.fbReports = map[string]*feedback.Report{}
		}
		a.fbReports[k] = nil // being captured
	}
	a.fbRepMu.Unlock()
	if seen {
		return rep
	}
	a.async(func() {
		var err error
		if f.message != "" {
			err = errors.New(f.message)
		}
		rep, cerr := m.Capture(feedback.ErrorContext{Title: f.title, Code: f.code, Kind: f.kind, Where: f.where, Err: err})
		if rep == nil || cerr != nil && rep.ID == "" {
			return // no link for this failure
		}
		a.post(func() {
			a.fbRepMu.Lock()
			defer a.fbRepMu.Unlock()
			if _, ok := a.fbReports[k]; ok && a.fbMgr == m {
				a.fbReports[k] = rep
			}
		})
	})
	a.fbRepMu.Lock()
	defer a.fbRepMu.Unlock()
	return a.fbReports[k] // set already when async ran at once (tests)
}

// reportLink draws "Report this problem" for f: a click opens the
// feedback window with its report. Nothing without the feedback.
func (a *app) reportLink(c *ui.Context, f failure) {
	if a.fbWin == nil {
		return
	}
	rep := a.fbReport(f)
	if rep == nil {
		return
	}
	fbui.ReportLink(c.Key("report:"+f.code+":"+f.key), a.fbWin, rep)
}

// step records a user's action (its name only: "try_transcribe") for the
// reports (main thread).
func (a *app) step(name string) {
	if m := a.fbMgr; m != nil {
		m.Step(name)
	}
}

// feedbackStepName shows a step of an error report in the app's language
// ("": the window's own wording).
func feedbackStepName(s string) string {
	switch s {
	case "start_transcribe":
		return i18n.T("Start transcribing")
	case "start_translate":
		return i18n.T("Start translating")
	case "start_rewrite":
		return i18n.T("Start rewriting")
	case "try_transcribe", "try_translate", "try_rewrite":
		return i18n.T("Start a try")
	case "retry_entry":
		return i18n.T("Retry an entry")
	case "restart_helper":
		return i18n.T("Restart the helper")
	case "download_model":
		return i18n.T("Download a model")
	case "check_updates":
		return i18n.T("Check for updates")
	}
	return ""
}
