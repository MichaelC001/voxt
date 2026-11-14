package announce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/core"
)

// site is a fake actnow.dev: it answers as the real endpoint does (ETag,
// Last-Modified, 304) and records the requests.
type site struct {
	mu       sync.Mutex
	items    []map[string]any
	status   int // forced status (0: normal)
	requests []*http.Request
}

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.Clone(context.Background()))
	if s.status != 0 {
		w.WriteHeader(s.status)
		return
	}
	body, _ := json.Marshal(map[string]any{"app": "voxtmini", "locale": r.URL.Query().Get("locale"), "notifications": s.items})
	etag := `"` + strings.Repeat("a", len(s.items)+1) + r.URL.Query().Get("locale") + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", "Sun, 15 Nov 2026 08:00:00 GMT")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (s *site) last() *http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[len(s.requests)-1]
}

type memDocs struct {
	mu   sync.Mutex
	docs map[string][]byte
	err  error
}

func (d *memDocs) Get(_ context.Context, key string) ([]byte, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return nil, false, d.err
	}
	v, ok := d.docs[key]
	return v, ok, nil
}

func (d *memDocs) Put(_ context.Context, key string, doc []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.docs == nil {
		d.docs = map[string][]byte{}
	}
	d.docs[key] = doc
	return nil
}

type posts struct {
	mu   sync.Mutex
	list []core.Notification
}

func (p *posts) post(n core.Notification) {
	p.mu.Lock()
	p.list = append(p.list, n)
	p.mu.Unlock()
}

func (p *posts) ids() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var ids []string
	for _, n := range p.list {
		ids = append(ids, n.ID)
	}
	return ids
}

func setup(t *testing.T, locale *string) (*site, *memDocs, *posts, func() *Poller) {
	t.Helper()
	s := &site{}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	docs, got := &memDocs{}, &posts{}
	poller := func() *Poller {
		return New(Config{URL: srv.URL + "/api/voxtmini/notifications", Version: "0.1.0-beta.2",
			Locale: func() string { return *locale }, HTTP: srv.Client, Docs: docs, Post: got.post})
	}
	return s, docs, got, poller
}

func TestCheckPostsEachOnce(t *testing.T) {
	ctx := context.Background()
	locale := "zh-Hans"
	s, _, got, poller := setup(t, &locale)
	s.items = []map[string]any{
		{"id": "new", "level": "warning", "title": "请更新", "body": " 旧版本即将停用。 ", "url": "https://actnow.dev/voxt", "publishedAt": "2026-11-15T08:00:00.000Z"},
		{"id": "old", "level": "info", "title": "欢迎", "body": "你好", "url": "voxtnew://settings", "publishedAt": "2026-11-10T00:00:00.000Z"},
		{"id": "", "title": "no id"},
		{"id": "untitled", "title": " "},
	}
	p := poller()
	n, err := p.Check(ctx)
	if err != nil || n != 2 {
		t.Fatalf("Check = %d, %v", n, err)
	}
	q := s.last().URL.Query()
	if q.Get("locale") != "zh-Hans" || q.Get("version") != "0.1.0-beta.2" || len(q) != 2 {
		t.Errorf("query %v", q)
	}
	if h := s.last().Header; h.Get("If-None-Match") != "" || h.Get("Cookie") != "" {
		t.Errorf("first request headers %v", h)
	}
	// Oldest first: the newest ends on top of the centre.
	want := []core.Notification{
		{ID: "announce/old", Severity: core.SeverityInfo, Title: "欢迎", Body: "你好",
			Time: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)},
		{ID: "announce/new", Severity: core.SeverityWarning, Title: "请更新", Body: "旧版本即将停用。", Link: "https://actnow.dev/voxt",
			Time: time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)},
	}
	for i, n := range got.list {
		if n.ID != want[i].ID || n.Severity != want[i].Severity || n.Title != want[i].Title || n.Body != want[i].Body ||
			n.Link != want[i].Link || !n.Time.Equal(want[i].Time) {
			t.Errorf("posted[%d] = %+v, want %+v", i, n, want[i])
		}
	}

	// Unchanged: a conditional request, 304, nothing posted.
	if n, err := p.Check(ctx); err != nil || n != 0 {
		t.Fatalf("again: %d, %v", n, err)
	}
	if h := s.last().Header; h.Get("If-None-Match") == "" || h.Get("If-Modified-Since") != "Sun, 15 Nov 2026 08:00:00 GMT" {
		t.Errorf("conditional headers %v", h)
	}

	// A new one on the site: only it is posted, by a new poller (a
	// restart) reading the state.
	s.items = append([]map[string]any{{"id": "newer", "title": "又一条", "publishedAt": "2026-11-16T00:00:00Z"}}, s.items...)
	if n, err := poller().Check(ctx); err != nil || n != 1 {
		t.Fatalf("after a restart: %d, %v", n, err)
	}
	if ids := got.ids(); len(ids) != 3 || ids[2] != "announce/newer" {
		t.Errorf("posted %v", ids)
	}

	// Another language is another URL: no validators sent, nothing posted
	// again.
	locale = "en"
	if n, err := poller().Check(ctx); err != nil || n != 0 {
		t.Fatalf("in English: %d, %v", n, err)
	}
	if h := s.last().Header; h.Get("If-None-Match") != "" {
		t.Errorf("validators of another URL sent: %v", h)
	}
}

func TestCheckFailsQuietly(t *testing.T) {
	ctx := context.Background()
	locale := "en"
	s, docs, got, poller := setup(t, &locale)
	s.items = []map[string]any{{"id": "a", "title": "A"}}

	s.status = http.StatusInternalServerError
	if _, err := poller().Check(ctx); err == nil {
		t.Error("500: no error")
	}
	s.status = 0
	docs.err = errors.New("disk")
	if n, err := poller().Check(ctx); err == nil || n != 0 {
		t.Errorf("documents unreadable: %d, %v (must not post without the ids posted)", n, err)
	}
	docs.err = nil
	docs.Put(ctx, DocKey, []byte("{not json"))
	if n, err := poller().Check(ctx); err != nil || n != 1 {
		t.Errorf("broken document: %d, %v", n, err)
	}
	if len(got.list) != 1 {
		t.Errorf("posted %v", got.ids())
	}

	// Not JSON, and no server at all.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html>")) }))
	defer bad.Close()
	if _, err := New(Config{URL: bad.URL, Post: got.post}).Check(ctx); err == nil {
		t.Error("HTML: no error")
	}
	bad.Close()
	if _, err := New(Config{URL: bad.URL, Post: got.post}).Check(ctx); err == nil {
		t.Error("no server: no error")
	}
}

func TestSeenIsBounded(t *testing.T) {
	locale := "en"
	s, docs, _, poller := setup(t, &locale)
	for i := range maxSeen + 20 {
		s.items = append(s.items, map[string]any{"id": fmt.Sprintf("id-%d", i), "title": "t"})
	}
	if n, err := poller().Check(context.Background()); err != nil || n != maxSeen+20 {
		t.Fatalf("Check = %d, %v", n, err)
	}
	var st state
	json.Unmarshal(docs.docs[DocKey], &st)
	if len(st.Seen) != maxSeen {
		t.Errorf("kept %d ids", len(st.Seen))
	}
}

func TestRunChecksAtStartAndThenEvery(t *testing.T) {
	locale := "en"
	s, _, got, _ := setup(t, &locale)
	s.items = []map[string]any{{"id": "a", "title": "A"}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	p := New(Config{URL: srv.URL, Post: got.post, Delay: time.Millisecond, Every: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		n := len(s.requests)
		s.mu.Unlock()
		if n >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d requests", n)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if ids := got.ids(); len(ids) != 1 {
		t.Errorf("posted %v", ids)
	}
}
