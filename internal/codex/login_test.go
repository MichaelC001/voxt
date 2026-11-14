package codex

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// browser plays the issuer's redirect: it reads the authorize URL and
// calls the callback with the given query (state "" = the right one).
type browser struct {
	t      *testing.T
	is     *issuer
	opened chan *url.URL
}

func newBrowser(t *testing.T, is *issuer) *browser {
	return &browser{t: t, is: is, opened: make(chan *url.URL, 1)}
}

func (b *browser) open(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	b.is.mu.Lock()
	b.is.challenge = u.Query().Get("code_challenge")
	b.is.mu.Unlock()
	b.opened <- u
	return nil
}

func (b *browser) callback(auth *url.URL, q url.Values) (int, string) {
	b.t.Helper()
	if !q.Has("state") {
		q.Set("state", auth.Query().Get("state"))
	}
	resp, err := http.Get(auth.Query().Get("redirect_uri") + "?" + q.Encode())
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

type loginResult struct {
	t   Tokens
	err error
}

func startLogin(ctx context.Context, cfg Config, b *browser) chan loginResult {
	res := make(chan loginResult, 1)
	go func() {
		t, err := Login(ctx, cfg, b.open)
		res <- loginResult{t, err}
	}()
	return res
}

func wait(t *testing.T, res chan loginResult) loginResult {
	t.Helper()
	select {
	case r := <-res:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("Login did not return")
		return loginResult{}
	}
}

func TestLogin(t *testing.T) {
	is := newIssuer(t)
	b := newBrowser(t, is)
	res := startLogin(context.Background(), is.cfg(), b)
	auth := <-b.opened

	q := auth.Query()
	want := map[string]string{
		"response_type":              "code",
		"client_id":                  DefaultClientID,
		"scope":                      "openid profile email offline_access",
		"code_challenge_method":      "S256",
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"originator":                 "codex_cli_rs",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("authorize %s = %q, want %q", k, q.Get(k), v)
		}
	}
	if auth.Path != "/oauth/authorize" || !strings.HasPrefix(auth.String(), is.srv.URL) {
		t.Errorf("authorize URL %s", auth)
	}
	redirect, _ := url.Parse(q.Get("redirect_uri"))
	if redirect.Hostname() != "127.0.0.1" || redirect.Path != "/auth/callback" {
		t.Errorf("redirect_uri %s", redirect)
	}
	if len(q.Get("state")) < 40 || q.Get("code_challenge") == "" {
		t.Errorf("state %q challenge %q", q.Get("state"), q.Get("code_challenge"))
	}

	status, body := b.callback(auth, url.Values{"code": {"good-code"}})
	if status != 200 || !strings.Contains(body, "You can close this tab") {
		t.Errorf("browser got %d %q", status, body)
	}
	r := wait(t, res)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.t.AccessToken == "" || r.t.RefreshToken != "r1" || r.t.IDToken == "" {
		t.Errorf("tokens %+v", r.t)
	}
	if r.t.AccountID != "acct-1" || r.t.Email != "me@example.com" || !r.t.LastRefresh.Equal(is.now) {
		t.Errorf("account %q email %q last %v", r.t.AccountID, r.t.Email, r.t.LastRefresh)
	}

	ex := is.exchanges[0]
	if ex.Get("grant_type") != "authorization_code" || ex.Get("client_id") != DefaultClientID ||
		ex.Get("redirect_uri") != q.Get("redirect_uri") || len(ex.Get("code_verifier")) < 43 {
		t.Errorf("exchange %v", ex)
	}
	// The listener is gone.
	if _, err := http.Get(q.Get("redirect_uri")); err == nil {
		t.Error("callback still answers after Login returned")
	}
}

func TestLoginStateMismatchKeepsWaiting(t *testing.T) {
	is := newIssuer(t)
	b := newBrowser(t, is)
	res := startLogin(context.Background(), is.cfg(), b)
	auth := <-b.opened

	status, _ := b.callback(auth, url.Values{"code": {"good-code"}, "state": {"forged"}})
	if status != http.StatusBadRequest {
		t.Errorf("forged state: %d", status)
	}
	if n, _, _ := is.counts(); n != 0 {
		t.Errorf("forged state exchanged the code")
	}
	select {
	case r := <-res:
		t.Fatalf("Login ended on a forged state: %v", r.err)
	default:
	}
	b.callback(auth, url.Values{"code": {"good-code"}})
	if r := wait(t, res); r.err != nil {
		t.Fatal(r.err)
	}
}

func TestLoginErrorParam(t *testing.T) {
	is := newIssuer(t)
	b := newBrowser(t, is)
	res := startLogin(context.Background(), is.cfg(), b)
	auth := <-b.opened
	b.callback(auth, url.Values{"error": {"access_denied"}, "error_description": {"no"}})
	r := wait(t, res)
	var ce *CallbackError
	if !errors.As(r.err, &ce) || ce.Code != "access_denied" || ce.Description != "no" {
		t.Fatalf("err = %v", r.err)
	}
}

func TestLoginExchangeRefused(t *testing.T) {
	is := newIssuer(t)
	b := newBrowser(t, is)
	res := startLogin(context.Background(), is.cfg(), b)
	auth := <-b.opened
	status, body := b.callback(auth, url.Values{"code": {"bad-code"}})
	if status != 200 || !strings.Contains(body, "failed") {
		t.Errorf("browser got %d %q", status, body)
	}
	r := wait(t, res)
	if r.err == nil || !strings.Contains(r.err.Error(), "invalid_grant") || strings.Contains(r.err.Error(), "bad-code") {
		t.Fatalf("err = %v", r.err)
	}
}

func TestLoginCancel(t *testing.T) {
	is := newIssuer(t)
	b := newBrowser(t, is)
	ctx, cancel := context.WithCancel(context.Background())
	res := startLogin(ctx, is.cfg(), b)
	auth := <-b.opened
	cancel()
	if r := wait(t, res); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("err = %v", r.err)
	}
	if _, err := http.Get(auth.Query().Get("redirect_uri")); err == nil {
		t.Error("callback still answers after cancel")
	}
}

func TestLoginOpenFails(t *testing.T) {
	is := newIssuer(t)
	_, err := Login(context.Background(), is.cfg(), func(string) error { return errors.New("no browser") })
	if err == nil || !strings.Contains(err.Error(), "no browser") {
		t.Fatalf("err = %v", err)
	}
}

func TestListenPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	_, err = Login(context.Background(), Config{Port: port}, func(string) error {
		t.Error("opened the browser without a listener")
		return nil
	})
	if !errors.Is(err, ErrPortInUse) {
		t.Fatalf("err = %v", err)
	}
}
