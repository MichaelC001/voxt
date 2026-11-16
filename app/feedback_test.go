package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/family"
	"github.com/hehehai/actnowkit/feedback"
)

// The words of the user the diagnostics must never carry: what was said
// and written, prompts, names of services, groups, microphones and files.
var feedbackPrivateWords = []string{
	"Berlin office", "Dear Anna", "Acme", "Quarterly", "pirate voice", "Spell Kubernetes", "Anna's AirPods",
	"Interview", "proxysecret", "My Whisper", "salary",
}

// newFeedbackTest is the app with Settings → Feedback over a fake site,
// logs with the user's words in them, on a tall window.
func newFeedbackTest(t *testing.T) (*app, *fakeFeedbackSite, *ui.Tester) {
	t.Helper()
	site := newFakeFeedbackSite(t)
	a, _, _ := newTestApp(t)
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
	if _, err := a.newFeedback(feedbackOptions{Base: site.srv.URL, Dir: filepath.Join(t.TempDir(), "feedback"), LogDir: logDir,
		Version: "0.1.0-beta.7", Client: site.srv.Client()}); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.view, 1100, 2400)
	a.navigate("/settings/feedback")
	frames(tt)
	return a, site, tt
}

func TestFeedbackSendAsksForTheEmail(t *testing.T) {
	a, site, tt := newFeedbackTest(t)
	for _, s := range []string{"New feedback", "Your feedback", "Send feedback on the web", "Or open an issue on GitHub"} {
		if !has(tt, s) {
			t.Fatalf("no %q: %q", s, tt.Texts())
		}
	}
	a.fbForm.body = "The overlay stays after a recording."
	a.fbSend()
	frames(tt)
	if !has(tt, "Enter an email so we can reply.") || len(site.created()) != 0 {
		t.Fatalf("sent without an email, or no problem shown: %q", tt.Texts())
	}
	a.fbForm.email = "not an email"
	a.fbSend()
	if !has(tt, "This email doesn't look right.") {
		t.Fatalf("no problem shown for a bad email: %q", tt.Texts())
	}
	a.fbForm.email = "ann@example.com"
	a.fbSend()
	frames(tt)
	if a.fbSt.Phase != feedback.Sent {
		t.Fatalf("phase %s err %v", a.fbSt.Phase, a.fbSt.Err)
	}
	if !has(tt, "Thanks! Your feedback FB-1001 was sent. We'll reply by email and here.") {
		t.Fatalf("no number shown: %q", tt.Texts())
	}
	reqs := site.created()
	if len(reqs) != 1 || reqs[0].Email != "ann@example.com" || reqs[0].App != "voxtmini" || reqs[0].Channel != "beta" ||
		reqs[0].LicenceToken != "" || reqs[0].DeviceID != "" {
		t.Fatalf("create %+v", reqs)
	}
	if len(reqs[0].Attachments) != 1 || reqs[0].Attachments[0].Role != feedback.RoleDiagnostics || len(site.puts) != 1 {
		t.Fatalf("the diagnostics not attached: %+v puts %v", reqs[0].Attachments, site.puts)
	}
	if a.fbForm.body != "" || a.fbForm.email != "" || !a.fbForm.diagnostics {
		t.Errorf("the form not cleared after sending: %+v", a.fbForm)
	}
	if len(a.fbHistory) != 1 || a.fbHistory[0].Number != "FB-1001" || !has(tt, "The overlay stays after a recording.") {
		t.Errorf("history %+v", a.fbHistory)
	}
}

func TestFeedbackHistoryUnreadAndReply(t *testing.T) {
	a, site, tt := newFeedbackTest(t)
	a.fbForm.body = "VoxtMini stops after an hour of dictation."
	a.fbForm.email = "ann@example.com"
	a.fbForm.diagnostics = false
	a.fbSend()
	frames(tt)
	if len(a.fbHistory) != 1 || len(site.created()[0].Attachments) != 0 {
		t.Fatalf("history %+v (phase %s err %v)", a.fbHistory, a.fbSt.Phase, a.fbSt.Err)
	}
	if a.fbUnread() != 0 || a.settingsDot("feedback") {
		t.Fatal("unread before any reply")
	}
	site.supportReplies("fb_1", "Thanks, which macOS version is this?")
	a.fbRefresh() // a click: the history is below the dialog's fold
	frames(tt)
	if a.fbUnread() != 1 || !a.settingsDot("feedback") || !hasPrefix(tt, "New reply · FB-1001 · Waiting for you · Problem") {
		t.Fatalf("unread %d texts %q", a.fbUnread(), tt.Texts())
	}
	a.fbOpenThread("fb_1")
	if a.fbThread == nil || !has(tt, "Thanks, which macOS version is this?") {
		t.Fatalf("thread %+v err %q texts %q", a.fbThread, a.fbThreadErr, tt.Texts())
	}
	if a.fbUnread() != 0 || a.settingsDot("feedback") {
		t.Fatal("still unread once read")
	}
	a.fbReplyText = "macOS 15.4 on an M3."
	click(t, tt, "Reply")
	if a.fbThread == nil || len(a.fbThread.Messages) != 2 || a.fbThread.Messages[1].Body != "macOS 15.4 on an M3." {
		t.Fatalf("thread after the reply %+v", a.fbThread)
	}
	a.fbThreadOpen = false
	a.fbRefresh()
	if a.fbUnread() != 0 {
		t.Errorf("the user's reply counts as unread")
	}
}

// TestFeedbackDiagnosticsHaveNoUserText: what the diagnostics send holds
// the logs, the settings and the helper's lines, and nothing the user
// said, wrote or named.
func TestFeedbackDiagnosticsHaveNoUserText(t *testing.T) {
	a, _, tt := newFeedbackTest(t)
	if !has(tt, "See what is sent") {
		t.Fatalf("no preview button: %q", tt.Texts())
	}
	a.fbOpenPreview()
	p := a.fbPreview
	if !a.fbPreviewOpen || !strings.Contains(p, `"id": "voxtmini"`) || !strings.Contains(p, "voxtmini.log") || !strings.Contains(p, "helper.log") {
		t.Fatalf("preview %v %s", a.fbPreviewOpen, p)
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

func TestSendFeedbackCommand(t *testing.T) {
	a, _, tt := newTestApp(t)
	var opened []string
	old := openExternal
	openExternal = func(u string) { opened = append(opened, u) }
	t.Cleanup(func() { openExternal = old })
	var cmd *command
	for _, c := range a.commands() {
		if c.id == "send-feedback" {
			cmd = &c
		}
	}
	if cmd == nil || !cmd.enabled {
		t.Fatal("no Send Feedback in the palette")
	}
	cmd.run()
	if !a.settingsOpen || a.shownPath() != "/settings/feedback" || !has(tt, "Send feedback on the web") {
		t.Fatalf("path %s texts %q", a.shownPath(), tt.Texts())
	}
	if !has(tt, "Open GitHub") {
		t.Fatal("no GitHub issue")
	}
	openExternal(issuesURL) // the button is below the dialog's fold
	if len(opened) != 1 || opened[0] != issuesURL {
		t.Errorf("opened %v", opened)
	}
}

// kitMessage is a reference text of actnowkit as VoxtMini's catalogue
// keys it: {app} is VoxtMini, {site} actnow.dev, {number} {email}
// {size} %s, {max} %d.
func kitMessage(s string) string {
	return strings.NewReplacer("{app}", "VoxtMini", "{site}", "actnow.dev", "{number}", "%s", "{email}", "%s", "{size}", "%s", "{max}", "%d").Replace(s)
}

// TestFeedbackTextsMatchKit: the section says, in English and in
// Chinese, what actnowkit's reference texts say (the family's apps word
// them alike), the labels without their "…".
func TestFeedbackTextsMatchKit(t *testing.T) {
	defer i18n.SetLocale("en")
	texts := []family.Text{
		feedback.TextNew, feedback.TextHistory, feedback.TextOnTheWeb,
		feedback.TextViewOnWeb, feedback.TextKind, feedback.TextKindBug, feedback.TextKindIdea, feedback.TextBodyPlaceholder,
		feedback.TextEmail, feedback.TextEmailPlaceholder, feedback.TextAttachments, feedback.TextAddImages,
		feedback.TextAddVideo, feedback.TextAttachHint, feedback.TextDiagnostics, feedback.TextPreview,
		feedback.TextPrivacyLink, feedback.TextPreparing, feedback.TextSending, feedback.TextSent,
		feedback.TextFailed, feedback.TextRetry, feedback.TextNoHistory, feedback.TextReply, feedback.TextYou, feedback.TextNewReply,
		feedback.TextStatusTriaged, feedback.TextStatusWaitingUser, feedback.TextProblemBodyShort, feedback.TextProblemBodyLong,
		feedback.TextProblemEmailRequired, feedback.TextProblemEmailInvalid, feedback.TextProblemTooManyImages,
		feedback.TextProblemImage, feedback.TextProblemVideo, feedback.TextProblemVideoTooLarge, feedback.TextRateLimited,
	}
	// VoxtMini's labels have no trailing "…" (AGENTS.md), actnowkit's
	// buttons do: compared without it.
	labels := map[string]bool{feedback.TextAddImages.EN: true, feedback.TextAddVideo.EN: true, feedback.TextPreview.EN: true}
	for _, tx := range texts {
		if labels[tx.EN] {
			tx.EN, tx.ZH = strings.TrimSuffix(tx.EN, "…"), strings.TrimSuffix(tx.ZH, "…")
		}
		en := kitMessage(tx.EN)
		i18n.SetLocale("zh-Hans")
		zh := i18n.T(en)
		i18n.SetLocale("en")
		if zh == en {
			t.Errorf("%q is not in the catalogue", en)
			continue
		}
		if want := kitMessage(tx.ZH); zh != want {
			t.Errorf("%q: VoxtMini says %q, actnowkit %q", en, zh, want)
		}
	}
	i18n.SetLocale("zh-Hans")
	for _, c := range []struct {
		got  string
		want family.Text
	}{
		{feedbackKindText(feedback.KindQuestion), feedback.TextKindQuestion}, {feedbackKindText(feedback.KindOther), feedback.TextKindOther},
		{feedbackStatusText(feedback.StatusNew), feedback.TextStatusNew}, {feedbackStatusText(feedback.StatusInProgress), feedback.TextStatusInProgress},
		{feedbackStatusText(feedback.StatusResolved), feedback.TextStatusResolved}, {feedbackStatusText(feedback.StatusClosed), feedback.TextStatusClosed},
		{i18n.X("feedback", "Description"), feedback.TextBody}, {i18n.X("feedback", "Send"), feedback.TextSend},
		{i18n.X("feedback", "Discard"), feedback.TextDiscard}, {i18n.X("feedback author", "Support"), feedback.TextSupport},
	} {
		if c.got != c.want.ZH {
			t.Errorf("%q: VoxtMini says %q, actnowkit %q", c.want.EN, c.got, c.want.ZH)
		}
	}
}

// connectFeedbackShots gives the snapshots' Settings → Feedback a fake
// site with one feedback answered, and a form being written.
func connectFeedbackShots(t *testing.T, a *app) {
	t.Helper()
	site := newFakeFeedbackSite(t)
	if _, err := a.newFeedback(feedbackOptions{Base: site.srv.URL, Dir: filepath.Join(t.TempDir(), "feedback"), LogDir: t.TempDir(),
		Version: "0.1.0", Client: site.srv.Client()}); err != nil {
		t.Fatal(err)
	}
	a.fbForm = feedbackForm{body: "Rewrite answers in English when I speak Chinese.", email: "ann@example.com"}
	a.fbSend()
	site.supportReplies("fb_1", "Thanks! Which language model is set for Rewrite?")
	a.fbRefresh()
	a.fbMgr.Reset()
	a.fbForm = feedbackForm{kind: 0, body: "The overlay stays on screen after a recording ends.", diagnostics: true}
}
