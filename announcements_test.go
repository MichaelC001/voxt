package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/hehehai/VoxtMini/internal/i18n"
)

// TestAnnouncements: the app's poller asks in the language shown and
// posts each announcement once as a card; "Learn more" opens its page in
// the browser.
func TestAnnouncements(t *testing.T) {
	a, _, tt := newTestApp(t)
	frames(tt)
	a.clearNotices() // the services' card
	a.version = "0.1.0-beta.2"
	var locale, version string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		locale, version = r.URL.Query().Get("locale"), r.URL.Query().Get("version")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"app":"voxtmini","locale":"zh-Hans","notifications":[
			{"id":"welcome","level":"info","title":"欢迎使用 VoxtMini","body":"三项功能：转录、翻译、转写。",
			 "url":"https://actnow.dev/voxt","publishedAt":"2026-11-15T08:00:00.000Z"}]}`))
	}))
	defer srv.Close()
	i18n.SetLocale("zh-Hans")
	t.Cleanup(func() { i18n.SetLocale("en") })

	p := a.announcer(srv.URL)
	if n, err := p.Check(context.Background()); err != nil || n != 1 {
		t.Fatalf("Check = %d, %v", n, err)
	}
	if locale != "zh-Hans" || version != "0.1.0-beta.2" {
		t.Errorf("asked locale %q version %q", locale, version)
	}
	frames(tt)
	if len(a.notices) != 1 || !has(tt, "欢迎使用 VoxtMini") || !has(tt, "三项功能：转录、翻译、转写。") {
		t.Fatalf("notices %+v %q", a.notices, firstTexts(tt))
	}
	click(t, tt, "了解更多")
	if !slices.Contains(tt.OpenedURLs(), "https://actnow.dev/voxt") {
		t.Errorf("opened %q", tt.OpenedURLs())
	}

	// Dismissed, it does not come back: not from this poller, nor after a
	// restart (a new poller over the same documents).
	a.dismissNotice(a.notices[0].ID)
	for _, p := range []interface {
		Check(context.Context) (int, error)
	}{p, a.announcer(srv.URL)} {
		if n, err := p.Check(context.Background()); err != nil || n != 0 {
			t.Errorf("again: %d, %v", n, err)
		}
	}
	if frames(tt); len(a.notices) != 0 {
		t.Errorf("notices %+v", a.notices)
	}
}
