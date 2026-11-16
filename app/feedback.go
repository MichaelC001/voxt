package main

// In-app feedback (wiring): actnowkit's feedback.Manager over actnow.dev
// (VOXTMINI_FEEDBACK_SERVER for a development site, in development
// builds only: dev_overrides.go), through the family's network setting
// (the downloads' proxy), with diagnostics of VoxtMini's, the helper's
// and the language models' logs, the last crash (crash_report.go), the
// settings as the diagnostic bundle has them (scrubSettings) and the
// lines of Settings → About's diagnostics. VoxtMini has no licence: the
// site never knows who writes, so the email is always asked for, and no
// device ID is sent. Settings → Feedback (feedback_page.go,
// feedback_view.go) draws it; the GitHub issue (Settings → About →
// Report a bug, the crash card's Report) stays.

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/crashlog"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/logs"
	"github.com/hehehai/VoxtMini/internal/logscrub"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/feedback"
)

// feedbackSite is where feedback goes; feedbackSiteEnv overrides it (a
// development copy of actnow.dev) in development builds.
const (
	feedbackSite    = "https://actnow.dev"
	feedbackSiteEnv = "VOXTMINI_FEEDBACK_SERVER"
)

// feedbackOptions are what differs between the app and tests.
type feedbackOptions struct {
	Base    string // the site ("": feedbackSiteEnv, else feedbackSite)
	Dir     string // the outbox and the history ("": the data folder's feedback/)
	LogDir  string // the logs ("": a.logDir)
	Version string
	Client  *http.Client // nil: through the proxy setting
}

// newFeedback makes the Manager and hands it to Settings → Feedback.
func (a *app) newFeedback(o feedbackOptions) (*feedback.Manager, error) {
	base := o.Base
	if base == "" && devOverrides() {
		base = os.Getenv(feedbackSiteEnv)
	}
	if base == "" {
		base = feedbackSite
	}
	dir := o.Dir
	if dir == "" {
		dir = filepath.Join(a.dataDir(), "feedback")
	}
	version := o.Version
	if version == "" {
		version = "0.0.0-dev"
	}
	channel := "release"
	if strings.Contains(version, "-") {
		channel = "beta"
	}
	client := o.Client
	if client == nil {
		client = feedbackClient(a.remoteProxy()) // remote_page.go
	}
	logDir := o.LogDir
	if logDir == "" {
		logDir = a.logDir
	}
	cc := feedback.CollectConfig{
		LogDir:    logDir,
		Logs:      []feedback.LogSpec{{Name: logs.FileName}, {Name: "helper.log"}, {Name: "llm.log"}},
		CrashFile: crashlog.CrashFile,
		Settings:  a.fbSnapSettings,
		Extra:     a.fbSnapExtra,
		Scrub:     logscrub.Line,
	}
	if logDir == "" {
		cc.Logs, cc.CrashFile = nil, ""
	}
	m, err := feedback.New(feedback.Config{
		App: actnow.AppID, Version: version, Build: buildRevision(), Channel: channel,
		BaseURL:    base,
		Dir:        dir,
		HTTPClient: client,
		Locale:     i18n.Locale,
		Collect:    cc,
		Log:        func(s string) { log.Print(s) },
	})
	if err != nil {
		return nil, err
	}
	a.fbUse(m)
	return m, nil
}

// connectFeedback makes the Manager of this run and sends what the last
// one left in the outbox (after the window opens: a.version is read).
func (a *app) connectFeedback() {
	m, err := a.newFeedback(feedbackOptions{Version: a.version})
	if err != nil {
		log.Printf("feedback: %v", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.fbStop = cancel
	go m.Resume(ctx)
}

// closeFeedback stops sending (the app quits; the outbox stays).
func (a *app) closeFeedback() {
	if a.fbStop != nil {
		a.fbStop()
	}
	if a.fbUnsub != nil {
		a.fbUnsub()
		a.fbUnsub = nil
	}
}

// feedbackClient goes through the family's proxy rule as it is at each
// request; the uploads may take minutes.
func feedbackClient(rule func() func(*http.Request) (*url.URL, error)) *http.Client {
	return &http.Client{Timeout: 15 * time.Minute, Transport: &http.Transport{
		Proxy:                 func(r *http.Request) (*url.URL, error) { return rule()(r) },
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ForceAttemptHTTP2:     true,
	}}
}

// buildRevision is the commit this build was made from ("" unknown).
func buildRevision() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				return s.Value[:12]
			}
		}
	}
	return ""
}

// feedbackSettings are the settings as the feedback's diagnostics carry
// them: the diagnostic bundle's (settings_about_scrub.go: prompts and
// extra bodies as their length, URLs without secrets), and the user's
// names of the remote services and the microphones masked.
func feedbackSettings(s settings.Settings) json.RawMessage {
	masked := func(list []settings.Provider) []settings.Provider {
		out := make([]settings.Provider, len(list))
		for i, p := range list {
			if p.Name != "" {
				p.Name = logscrub.Mask
			}
			out[i] = p
		}
		return out
	}
	s.Providers.ASR, s.Providers.LLM = masked(s.Providers.ASR), masked(s.Providers.LLM)
	if len(s.Audio.DeviceNames) > 0 {
		names := make(map[string]string, len(s.Audio.DeviceNames))
		for id := range s.Audio.DeviceNames {
			names[id] = logscrub.Mask
		}
		s.Audio.DeviceNames = names
	}
	data := scrubSettings(s)
	if !json.Valid(data) {
		return nil
	}
	return data
}

// feedbackExtraKeys are the lines of the diagnostics (diagnosticsText)
// the feedback adds; the machine, the folders and the language are the
// feedback's own, and the log lines come from the files.
var feedbackExtraKeys = []string{"Helper", "Helper protocol", "Go", "MyGo", "Models installed", "Models loaded", "Language model process"}

// feedbackExtra picks the helper's, the models', the versions' and the
// self-check's lines of the diagnostics ("Key: value" per line),
// scrubbed as log lines are.
func feedbackExtra(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "Check ") {
			out[k] = logscrub.Line(v)
			continue
		}
		for _, x := range feedbackExtraKeys {
			if k == x {
				out[k] = logscrub.Line(v)
			}
		}
	}
	return out
}

// sendFeedback opens Settings → Feedback (the Help menu's and the
// palette's Send Feedback).
func (a *app) sendFeedback() { a.navigate("/settings/feedback") }
