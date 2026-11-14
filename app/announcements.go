package main

// Announcements (internal/announce): news from actnow.dev
// (GET https://actnow.dev/api/voxtmini/notifications) as cards in the
// notification centre, each once, in the language VoxtMini shows. A check
// at start and every six hours, through the family's proxy; it sends the
// language and the app's version, nothing else, and fails silently (a
// line in the log). VOXTMINI_ANNOUNCEMENTS_URL points it at another
// server (a development site); set to "" it turns them off.

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/announce"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
)

// announceState is the poller's.
type announceState struct {
	stopAnnounce func() // nil: not running
}

// startAnnouncements starts the poller; main calls it when the app is
// ready (the family's network settings are known).
func (a *app) startAnnouncements() {
	endpoint := announce.DefaultURL
	if v, ok := os.LookupEnv("VOXTMINI_ANNOUNCEMENTS_URL"); ok {
		endpoint = strings.TrimSpace(v)
	}
	if endpoint == "" {
		return
	}
	p := a.announcer(endpoint)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Run(ctx)
	}()
	a.stopAnnounce = func() { cancel(); <-done }
}

// announcer is the poller of endpoint, for this app: its version (set by
// openWindow), the language it shows, the family's proxy, its documents
// and notification centre.
func (a *app) announcer(endpoint string) *announce.Poller {
	proxy := a.remoteProxy() // remote_page.go: the family's network.proxy now
	return announce.New(announce.Config{
		URL:     endpoint,
		Version: a.version,
		Locale:  i18n.Locale,
		HTTP:    func() *http.Client { return models.NewHTTPClient(proxy()) },
		Docs:    a.lib.store.Documents(),
		Post:    a.notes.Post, // never blocks; the shell hears NotificationPosted
		Logf:    log.Printf,
	})
}

// stopAnnouncements stops the poller (on quit, before the store closes).
func (a *app) stopAnnouncements() {
	if a.stopAnnounce != nil {
		a.stopAnnounce()
		a.stopAnnounce = nil
	}
}

// isWebLink reports whether a card's link is a web page (an
// announcement's "Learn more"), which the browser opens.
func isWebLink(link string) bool { return strings.HasPrefix(link, "https://") }

// openWeb opens a web page in the browser (MyGo's services in tests).
func (a *app) openWeb(c *ui.Context, u string) {
	if a.win == nil {
		c.Services().OpenURL(u)
		return
	}
	go mygo.Shell.OpenExternal(u)
}
