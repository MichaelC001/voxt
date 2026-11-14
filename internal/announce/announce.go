// Package announce brings VoxtMini's announcements from actnow.dev
// (GET /api/voxtmini/notifications, actnow-www docs/announcements.md) into
// the notification centre, each once. It replaces Voxt's
// VoxtNotificationStore (voxt.actnow.dev/api/app/notifications) without its
// unread page: an announcement is a card like any other notification.
//
// A Poller checks at start (after Delay) and every Every: it asks for the
// UI's language and the app's version (the server filters by both),
// conditionally (If-None-Match / If-Modified-Since of the last answer), and
// posts what it has not posted before. It sends nothing else and keeps
// nothing but the ids it posted and the last answer's validators, in the
// document "announcements". Failures are logged only: an announcement
// that did not come is no problem for the user.
//
// Imports: core.
package announce

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/hehehai/VoxtMini/internal/core"
)

// defaultClient fetches without a client of the app's: as
// http.DefaultClient, but never redirected from https to http.
var defaultClient = &http.Client{CheckRedirect: core.NoDowngrade}

// DefaultURL is the announcements' endpoint.
const DefaultURL = "https://actnow.dev/api/voxtmini/notifications"

// DocKey is the document of the poller's state.
const DocKey = "announcements"

// IDPrefix starts the ID of an announcement's notification: the centre
// replaces a notification of the same ID, so one announcement is one card.
const IDPrefix = "announce/"

// maxSeen is how many posted ids are kept (oldest go first): far more
// than the site ever publishes.
const maxSeen = 500

// maxBody limits the answer read.
const maxBody = 256 << 10

// Documents is where the state is kept; store.Documents satisfies it.
type Documents interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Put(ctx context.Context, key string, doc []byte) error
}

// Config is a Poller's.
type Config struct {
	URL     string                           // DefaultURL when ""
	Version string                           // the app's ("" in dev builds: every announcement)
	Locale  func() string                    // the UI's language now ("en", "zh-Hans", "ja")
	HTTP    func() *http.Client              // a client through the family's proxy now; nil: defaultClient
	Docs    Documents                        // nil: memory only
	Post    func(n core.Notification)        // the notification centre's Post
	Logf    func(format string, args ...any) // nil: silent
	Delay   time.Duration                    // before the first check; default 15 s
	Every   time.Duration                    // between checks; default 6 h
}

// Poller checks for announcements; Check and Run are not for concurrent use.
type Poller struct {
	cfg    Config
	loaded bool
	st     state
}

// state is the document.
type state struct {
	Seen []string `json:"seen"` // ids posted, oldest first
	// The last 200 answer's request URL and validators: sent again only
	// for the same URL (language and version).
	URL          string `json:"url,omitempty"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
}

// item is one announcement in the answer.
type item struct {
	ID          string    `json:"id"`
	Level       string    `json:"level"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"publishedAt"`
}

// New returns a poller; nothing runs until Run or Check.
func New(cfg Config) *Poller {
	if cfg.URL == "" {
		cfg.URL = DefaultURL
	}
	if cfg.Delay == 0 {
		cfg.Delay = 15 * time.Second
	}
	if cfg.Every == 0 {
		cfg.Every = 6 * time.Hour
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Poller{cfg: cfg}
}

// Run checks after Delay, then every Every, until ctx ends.
func (p *Poller) Run(ctx context.Context) {
	wait := p.cfg.Delay
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = p.cfg.Every
		if _, err := p.Check(ctx); err != nil && ctx.Err() == nil {
			p.cfg.Logf("announcements: %v", err)
		}
	}
}

// Check asks once and posts the announcements not posted before, oldest
// first (the newest ends on top); it returns how many it posted.
func (p *Poller) Check(ctx context.Context) (int, error) {
	if err := p.load(ctx); err != nil {
		return 0, err // posting without the ids posted could post them again
	}
	reqURL, err := p.requestURL()
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if p.st.URL == reqURL {
		if p.st.ETag != "" {
			req.Header.Set("If-None-Match", p.st.ETag)
		}
		if p.st.LastModified != "" {
			req.Header.Set("If-Modified-Since", p.st.LastModified)
		}
	}
	client := defaultClient
	if p.cfg.HTTP != nil {
		client = p.cfg.HTTP()
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		return 0, nil
	case http.StatusOK:
	default:
		return 0, fmt.Errorf("%s: %s", reqURL, resp.Status)
	}
	var answer struct {
		Notifications []item `json:"notifications"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&answer); err != nil {
		return 0, fmt.Errorf("%s: %w", reqURL, err)
	}
	posted := 0
	for _, it := range slices.Backward(answer.Notifications) {
		it.ID, it.Title = strings.TrimSpace(it.ID), strings.TrimSpace(it.Title)
		if it.ID == "" || it.Title == "" || slices.Contains(p.st.Seen, it.ID) {
			continue
		}
		p.cfg.Post(notification(it))
		p.st.Seen = append(p.st.Seen, it.ID)
		posted++
	}
	if n := len(p.st.Seen); n > maxSeen {
		p.st.Seen = slices.Clone(p.st.Seen[n-maxSeen:])
	}
	p.st.URL, p.st.ETag, p.st.LastModified = reqURL, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified")
	return posted, p.save(ctx)
}

// requestURL is the endpoint with the language and the version.
func (p *Poller) requestURL() (string, error) {
	u, err := url.Parse(p.cfg.URL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	locale := "en"
	if p.cfg.Locale != nil {
		locale = p.cfg.Locale()
	}
	q.Set("locale", locale)
	if p.cfg.Version != "" {
		q.Set("version", p.cfg.Version)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// notification is an announcement's card: a warning when the site says
// so, its link when it is a web page.
func notification(it item) core.Notification {
	n := core.Notification{ID: IDPrefix + it.ID, Severity: core.SeverityInfo, Title: it.Title,
		Body: strings.TrimSpace(it.Body), Time: it.PublishedAt}
	if it.Level == "warning" {
		n.Severity = core.SeverityWarning
	}
	if strings.HasPrefix(it.URL, "https://") {
		n.Link = it.URL
	}
	return n
}

// load reads the state once; a document that does not decode is
// replaced (logged).
func (p *Poller) load(ctx context.Context) error {
	if p.loaded || p.cfg.Docs == nil {
		return nil
	}
	data, ok, err := p.cfg.Docs.Get(ctx, DocKey)
	if err != nil {
		return err
	}
	p.loaded = true
	if ok {
		if err := json.Unmarshal(data, &p.st); err != nil {
			p.st = state{}
			p.cfg.Logf("announcements: %v", err)
		}
	}
	return nil
}

func (p *Poller) save(ctx context.Context) error {
	if p.cfg.Docs == nil {
		return nil
	}
	data, err := json.Marshal(p.st)
	if err != nil {
		return err
	}
	return p.cfg.Docs.Put(ctx, DocKey, data)
}
