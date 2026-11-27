package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/actnowkit/feedback"
	"github.com/hehehai/actnowkit/feedback/feedbacktest"
	fbui "github.com/hehehai/actnowkit/feedbackui"

	"github.com/hehehai/VoxtMini/internal/crashlog"
	"github.com/hehehai/VoxtMini/internal/helper"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

// The words of the user the diagnostics must never carry: what was said
// and written, prompts, names of services, groups, microphones and files.
var feedbackPrivateWords = []string{
	"Berlin office", "Dear Anna", "Acme", "Quarterly", "pirate voice", "Spell Kubernetes", "Anna's AirPods",
	"Interview", "proxysecret", "My Whisper", "salary",
}

// fbTest is the app with its feedback over a fake actnow.dev: the app's
// window in tt, the feedback window (headless) in wt.
type fbTest struct {
	a    *app
	fs   *fakeSupervisor
	site *feedbacktest.Site
	tt   *ui.Tester
	wt   *ui.Tester
}

func (f *fbTest) frames() { frames(f.tt); frames(f.wt) }

// connectTestFeedback connects a's feedback to site, the window headless.
func connectTestFeedback(t *testing.T, a *app, site *feedbacktest.Site, logDir string) {
	t.Helper()
	a.fbOptions = fbui.Options{Headless: true}
	if _, err := a.newFeedback(feedbackOptions{Base: site.URL, Dir: filepath.Join(t.TempDir(), "feedback"), LogDir: logDir,
		Version: "0.1.0-beta.7", Client: site.Client()}); err != nil {
		t.Fatal(err)
	}
}

// newFeedbackTest is the app with the feedback over a fake site, logs
// and settings with the user's words in them, on a tall window.
func newFeedbackTest(t *testing.T) *fbTest {
	t.Helper()
	site := feedbacktest.New()
	t.Cleanup(site.Close)
	a, fs, _ := newTestApp(t)
	logDir := t.TempDir()
	os.WriteFile(filepath.Join(logDir, "voxtmini.log"), []byte(strings.Join([]string{
		`2026-11-16 10:00:00.000 INFO app: VoxtMini 0.1.0-beta.6 started`,
		`2026-11-16 10:00:01.000 INFO app: session 01J9: recognised "Meet at the Berlin office at nine" in 1.2s`,
		`2026-11-16 10:00:02.000 INFO app: session 01J9: composed "Dear Anna, the Quarterly numbers" in 800ms`,
		`2026-11-16 10:00:03.000 INFO app: session 01J9: selection: "salary review notes" via ax in 3 ms`,
		`2026-11-16 10:00:04.000 INFO app: session 01J9: app enhancement: Acme mail by url`,
		`2026-11-16 10:00:05.000 WARN app: session 01J9: recording from Anna's AirPods Pro: not connected`,
		`2026-11-16 10:00:06.000 ERROR app: history: could not save ~/Documents/Interview with Acme.m4a: disk full`,
		`2026-11-16 10:00:07.000 ERROR app: asr: Post "https://asr.example.com/acme/v1?key=1": EOF`,
	}, "\n")+"\n"), 0o600)
	os.WriteFile(filepath.Join(logDir, "llm.log"), []byte("2026-11-16 10:00:00.000 loaded Qwen3-1.7B; prompt \"Spell Kubernetes right\"\n"), 0o600)
	os.WriteFile(filepath.Join(logDir, "crash.log"), []byte("=== VoxtMini run 0.1.0-beta.6 2026-11-16 09:00:00 ===\npanic: runtime error: index out of range\n\ngoroutine 1 [running]:\nmain.main()\n"), 0o600)
	os.WriteFile(filepath.Join(logDir, "helper.log"), []byte("2026-11-16 10:00:00.000 [helper] ready\n"), 0o600)
	a.updateSettings(func(s *settings.Settings) {
		s.Features.Rewrite.Prompt = "Answer in a pirate voice"
		s.Providers.ASR = []settings.Provider{{ID: "openai", Name: "My Whisper", Endpoint: "https://asr.example.com/v1?api_key=sk-proxysecret", Prompt: "Spell Kubernetes right"}}
		s.Audio.DeviceNames = map[string]string{"BuiltIn": "Anna's AirPods Pro"}
		s.Network.Proxy = "http://river:proxysecret@127.0.0.1:7897"
	})
	connectTestFeedback(t, a, site, logDir)
	return &fbTest{a: a, fs: fs, site: site, tt: ui.NewTester(a.view, 1100, 1400), wt: ui.NewTester(a.fbWin.View, fbui.WindowWidth, fbui.WindowHeight)}
}

func TestFeedbackSettingsPanelOpensWindow(t *testing.T) {
	f := newFeedbackTest(t)
	f.a.navigate("/settings/feedback")
	f.frames()
	for _, s := range []string{"Your feedback", "Feedback you send from this Mac appears here.", "Open the feedback window",
		"Send feedback on the web", "Or open an issue on GitHub"} {
		if !has(f.tt, s) {
			t.Fatalf("no %q in %q", s, f.tt.Texts())
		}
	}
	if f.a.fbWin.IsOpen() {
		t.Fatal("open before asked")
	}
	click(t, f.tt, "Open the feedback window")
	f.frames()
	if !f.a.fbWin.IsOpen() || !has(f.wt, "Send Feedback") || !has(f.wt, "to the VoxtMini team · actnow.dev") {
		t.Fatalf("window: open %v, %q", f.a.fbWin.IsOpen(), f.wt.Texts())
	}
	click(t, f.wt, "Cancel")
	if f.a.fbWin.IsOpen() {
		t.Fatal("still open")
	}
}

func TestFeedbackWindowInChinese(t *testing.T) {
	f := newFeedbackTest(t)
	i18n.SetLocale("zh-Hans")
	f.a.sendFeedback()
	f.frames()
	for _, s := range []string{"发送反馈", "发给 VoxtMini 团队 · actnow.dev", "提问", "提交"} {
		if !has(f.wt, s) {
			t.Fatalf("no %q in %q", s, f.wt.Texts())
		}
	}
}

// VoxtMini has no licence: the email is always asked for, and the
// diagnostics go with the feedback; sent, Settings → Feedback lists it
// and a reply shows a dot.
func TestFeedbackSendAsksForTheEmail(t *testing.T) {
	f := newFeedbackTest(t)
	w := f.a.fbWin
	f.a.sendFeedback()
	w.SetForm(feedback.KindBug, "The overlay stays after a recording.", "", true, false)
	w.Send()
	f.frames()
	if !has(f.wt, "Enter an email so we can reply.") || f.site.Count("POST /api/feedback/v1/create") != 0 {
		t.Fatalf("sent without an email, or no problem shown: %q", f.wt.Texts())
	}
	w.SetForm("", "", "ann@example.com", true, false)
	w.Send()
	f.frames()
	if s := w.Snapshot(); s.Sent != "FB-1042" && s.Phase != feedback.Sent {
		t.Fatalf("not sent: %+v", s)
	}
	it := f.site.Items()["fb_1"]
	if it == nil || !it.Completed || it.Request.Email != "ann@example.com" || it.Request.App != "voxtmini" || it.Request.Channel != "beta" ||
		it.Request.LicenceToken != "" || it.Request.DeviceID != "" || it.Request.Context != nil {
		t.Fatalf("site %+v", it)
	}
	if len(it.Files) != 1 {
		t.Fatalf("the diagnostics not attached: %+v", it.Files)
	}
	f.a.navigate("/settings/feedback")
	if !has(f.tt, "The overlay stays after a recording.") || !has(f.tt, "Your feedback (1)") {
		t.Fatalf("panel %q", f.tt.Texts())
	}
	if f.a.fbUnread() != 0 || f.a.settingsDot("feedback") {
		t.Fatal("unread before a reply")
	}
	f.site.SupportReplies("fb_1", "Thanks, which macOS version is this?")
	w.Refresh()
	f.frames()
	if f.a.fbUnread() != 1 || !f.a.settingsDot("feedback") {
		t.Fatalf("unread %d", f.a.fbUnread())
	}
}

// TestFeedbackDiagnosticsHaveNoUserText: what the diagnostics send holds
// the logs, the settings and the helper's lines, and nothing the user
// said, wrote or named.
func TestFeedbackDiagnosticsHaveNoUserText(t *testing.T) {
	f := newFeedbackTest(t)
	f.a.sendFeedback()
	f.a.fbWin.SetForm("", "", "ann@example.com", true, false)
	f.a.fbWin.ShowWhatIsSent(false)
	p := f.a.fbWin.Snapshot().Sheet
	if !strings.Contains(p, `"id": "voxtmini"`) || !strings.Contains(p, "voxtmini.log") || !strings.Contains(p, "helper.log") {
		t.Fatalf("preview %s", p)
	}
	for _, w := range []string{`"Helper"`, `"settings"`, `"rewrite"`, "asr.example.com", "127.0.0.1:7897", "disk full", "recognised", "llm.log", "index out of range"} {
		if !strings.Contains(p, w) {
			t.Errorf("no %s in the diagnostics: %s", w, p)
		}
	}
	for _, w := range feedbackPrivateWords {
		if strings.Contains(p, w) {
			t.Errorf("the diagnostics hold %q: %s", w, p)
		}
	}
}

func TestFeedbackExtra(t *testing.T) {
	got := feedbackExtra("VoxtMini: 1.0\nHelper: 0.1.0 (ready)\nData folder: /Users/ann/x (free 3 GB)\nMyGo: v0.5.5\nCheck data: fail Cannot write in ~/Documents/Acme notes: denied\nRecent log lines:\n")
	if len(got) != 3 || got["Helper"] != "0.1.0 (ready)" || got["MyGo"] != "v0.5.5" || strings.Contains(got["Check data"], "Acme") {
		t.Errorf("%v", got)
	}
}

// Help → Send Feedback, the palette and the menu bar open the window;
// without the feedback connected they go to Settings → Feedback.
func TestSendFeedbackMenuPaletteAndTray(t *testing.T) {
	f := newFeedbackTest(t)
	for _, c := range f.a.commands() {
		if c.id == "send-feedback" {
			if !c.enabled {
				t.Fatal("disabled")
			}
			c.run()
		}
	}
	if !f.a.fbWin.IsOpen() {
		t.Fatal("the palette's command did not open the window")
	}
	f.a.fbWin.Close()
	var tray func(*mygo.MenuItem, *mygo.Window)
	for _, it := range f.a.trayItems() {
		if it.Label == "Send Feedback" {
			tray = it.Click
		}
	}
	if tray == nil {
		t.Fatal("no Send Feedback in the menu bar")
	}
	tray(nil, nil)
	if !f.a.fbWin.IsOpen() {
		t.Fatal("the menu bar did not open the window")
	}

	a, _, tt := newTestApp(t) // no Manager
	var opened []string
	old := openExternal
	openExternal = func(u string) { opened = append(opened, u) }
	t.Cleanup(func() { openExternal = old })
	a.sendFeedback()
	if !a.settingsOpen || a.shownPath() != "/settings/feedback" || !has(tt, "Send feedback on the web") || !has(tt, "Open GitHub") {
		t.Fatalf("path %s texts %q", a.shownPath(), tt.Texts())
	}
	click(t, tt, "Open GitHub")
	if len(opened) != 1 || opened[0] != issuesURL {
		t.Errorf("opened %v", opened)
	}
}

// fbFailure is a failure point of VoxtMini: how to show it, and its
// report's code, kind and place.
type fbFailure struct {
	name, code, kind, where string
	show                    func(t *testing.T, f *fbTest)
}

var fbFailures = []fbFailure{
	{"session", codeSession, "transcribe_failed", "History › Entry details", func(t *testing.T, f *fbTest) {
		e := store.Entry{ID: "01FAILED00000000000000000A", Kind: store.KindTranscribe, CreatedAt: time.Now().Add(-time.Hour), Status: store.StatusFailed,
			AppBundle: "com.apple.Safari", AppName: "Safari", RawText: "the salary review", Error: "asr: no result within 30s"}
		if err := f.a.lib.store.History().Put(t.Context(), e); err != nil {
			t.Fatal(err)
		}
		f.a.navigate("/history")
		f.frames()
		f.a.openHistoryEntry(e)
	}},
	{"try", codeTry, "session_asr", "Transcribe › Try", func(t *testing.T, f *fbTest) {
		f.a.navigate("/transcribe")
		f.frames()
		s := trySnapshot(f.a, session.Failed)
		s.Problem = &session.Problem{Code: session.ProblemASR, Err: errors.New("omni: the model stopped")}
		f.a.sessionChanged(s)
	}},
	{"helper", codeHelper, "helper_crashed", "Sidebar › Helper card", func(t *testing.T, f *fbTest) {
		f.a.navigate("/history")
		f.fs.set(helper.Status{State: helper.Failed, Reason: helper.FailCrashed, Err: "exit status 2"})
	}},
	{"model", codeModel, kindModelDownload, "Models › Model card", func(t *testing.T, f *fbTest) {
		f.a.downloadModel(models.LLMFast) // a step of the report
		f.a.modelStatus[models.LLMFast] = models.Status{Total: 1, Err: errors.New("connection reset")}
		f.a.navigate(modelsPath)
		f.a.mdTab = 1
		f.tt.SetSize(1100, 4000)
	}},
	{"crash", codeCrash, kindCrash, "Crash card", func(t *testing.T, f *fbTest) {
		path := filepath.Join(t.TempDir(), crashlog.CrashFile)
		os.WriteFile(path, []byte("=== VoxtMini run 2026-11-14T10:00:00Z VoxtMini 0.1.0 pid 42 ===\npanic: runtime error: index out of range [3] with length 3\n\ngoroutine 1 [running]:\nmain.main()\n"), 0o600)
		f.a.tellCrash(&crashlog.Run{Path: path, Previous: "2026-11-14T10:00:00Z VoxtMini 0.1.0 pid 42"})
		f.a.navigate("/home")
	}},
	{"history", codeHistory, kindHistoryRead, "History", func(t *testing.T, f *fbTest) {
		f.a.navigate("/history")
		f.frames()
		f.a.hist.err = "database is locked"
	}},
}

// Every failure point shows "Report this problem"; a click opens the
// window filled with its report, without the user's words.
func TestFailuresShowReportLink(t *testing.T) {
	for _, fl := range fbFailures {
		t.Run(fl.name, func(t *testing.T) {
			f := newFeedbackTest(t)
			f.a.step("start_transcribe")
			fl.show(t, f)
			f.frames()
			label := "Report this problem"
			if fl.name == "crash" {
				label = "Send feedback" // the crash card's own link
			}
			click(t, f.tt, label)
			if !f.a.fbWin.IsOpen() {
				t.Fatal("the window did not open")
			}
			f.frames()
			all := strings.Join(f.wt.Texts(), "\n")
			want := []string{"Report this problem", fl.code, "log lines)", "Include this error report", fl.kind, fl.where}
			if fl.name != "crash" { // the run that crashed took its steps with it
				want = append(want, "Start transcribing")
			}
			if fl.name == "model" {
				want = append(want, "Download a model")
			}
			for _, s := range want {
				if !strings.Contains(all, s) {
					t.Errorf("no %q in the window: %q", s, f.wt.Texts())
				}
			}
			for _, w := range feedbackPrivateWords {
				if strings.Contains(all, w) {
					t.Errorf("the user's words in the report: %q", w)
				}
			}
			if s := f.a.fbWin.Snapshot(); !strings.HasPrefix(s.Report, "ER-") || !s.Include || s.Kind != feedback.KindBug {
				t.Errorf("prefill %+v", s)
			}
		})
	}
}

// Sent from a report's link: the request has the context, the
// diagnostics the report, and neither holds the user's words.
func TestFeedbackWithReportUploaded(t *testing.T) {
	f := newFeedbackTest(t)
	f.a.step("start_transcribe")
	fbFailures[0].show(t, f)
	f.frames()
	click(t, f.tt, "Report this problem")
	f.a.fbWin.SetForm("", "Dictation fails after 30 seconds.", "ann@example.com", true, true)
	f.a.fbWin.Send()
	f.frames()
	if !has(f.wt, "FB-1042") && f.a.fbWin.Snapshot().Phase != feedback.Sent {
		t.Fatalf("not sent: %+v %q", f.a.fbWin.Snapshot(), f.wt.Texts())
	}
	it := f.site.Items()["fb_1"]
	if it == nil || !it.Completed {
		t.Fatalf("site %+v", it)
	}
	ctx := it.Request.Context
	if ctx == nil || ctx.Code != codeSession || ctx.Kind != "transcribe_failed" || ctx.Where != "History › Entry details" ||
		!strings.HasPrefix(ctx.ReportID, "ER-") || it.Request.Kind != feedback.KindBug || len(ctx.Steps) == 0 || ctx.Steps[0] != "start_transcribe" {
		t.Fatalf("context %+v kind %s", ctx, it.Request.Kind)
	}
	var diag struct {
		Report *feedback.Report `json:"report"`
	}
	for _, file := range it.Files {
		if file.Meta.Role == feedback.RoleDiagnostics {
			if err := json.Unmarshal(file.Data, &diag); err != nil {
				t.Fatal(err)
			}
		}
	}
	if diag.Report == nil || diag.Report.ID != ctx.ReportID || len(diag.Report.Logs) == 0 || !strings.Contains(diag.Report.Error, "no result within 30s") {
		t.Fatalf("diagnostics report %+v", diag.Report)
	}
	data, _ := json.Marshal(it.Request)
	rep, _ := json.Marshal(diag.Report)
	for _, w := range feedbackPrivateWords {
		if strings.Contains(string(data), w) || strings.Contains(string(rep), w) {
			t.Errorf("the user's words %q are sent", w)
		}
	}
}

// The window follows Settings → Appearance, not only the system's.
func TestFeedbackWindowFollowsAppearance(t *testing.T) {
	f := newFeedbackTest(t)
	f.a.sendFeedback()
	f.a.updateSettings(func(s *settings.Settings) { s.Appearance.Theme = "dark" })
	f.frames()
	if c := f.wt.Image().RGBAAt(5, 300); c.R > 40 {
		t.Fatalf("not dark: %v", c)
	}
	f.a.updateSettings(func(s *settings.Settings) { s.Appearance.Theme = "light" })
	f.wt.SetDark(true)
	f.frames()
	if c := f.wt.Image().RGBAAt(5, 300); c.R < 200 {
		t.Fatalf("not light: %v", c)
	}
}

// connectFeedbackShots gives the snapshots' Settings → Feedback a fake
// site with one feedback answered.
func connectFeedbackShots(t *testing.T, a *app) {
	t.Helper()
	site := feedbacktest.New()
	t.Cleanup(site.Close)
	connectTestFeedback(t, a, site, t.TempDir())
	w := a.fbWin
	w.Open(feedback.Prefill{})
	w.SetForm(feedback.KindBug, "Rewrite answers in English when I speak Chinese.", "ann@example.com", true, false)
	w.Send()
	site.SupportReplies("fb_1", "Thanks! Which language model is set for Rewrite?")
	w.Refresh()
	w.Close()
}

// TestFeedbackSiteOverride: VOXTMINI_FEEDBACK_SERVER points a development
// build at another site; a published build ignores it.
func TestFeedbackSiteOverride(t *testing.T) {
	a, _, _ := newTestApp(t)
	t.Setenv(feedbackSiteEnv, "http://127.0.0.1:3100")
	old := devOverrides
	t.Cleanup(func() { devOverrides = old })
	for _, c := range []struct {
		dev  bool
		want string
	}{{true, "http://127.0.0.1:3100/feedback"}, {false, feedbackSite + "/feedback"}} {
		devOverrides = func() bool { return c.dev }
		m, err := a.newFeedback(feedbackOptions{Dir: filepath.Join(t.TempDir(), "feedback"), Version: "0.1.0"})
		if err != nil {
			t.Fatal(err)
		}
		if u := m.WebURL(); !strings.HasPrefix(u, c.want) {
			t.Errorf("dev %v: %s", c.dev, u)
		}
	}
}
