package codex

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func stored(t *testing.T, tok Tokens) *memStore {
	t.Helper()
	return &memStore{v: tok.Marshal()}
}

func TestTokensRoundTrip(t *testing.T) {
	in := Tokens{IDToken: "i", AccessToken: "a", RefreshToken: "r", AccountID: "x", Email: "e", LastRefresh: time.Unix(5, 0).UTC()}
	out, err := Parse(in.Marshal())
	if err != nil || out != in {
		t.Fatalf("%+v %v", out, err)
	}
	if _, err := Parse("not json secret-token"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v", err)
	}
}

func TestHeadersFreshToken(t *testing.T) {
	is := newIssuer(t)
	access := accessToken("a0", is.now.Add(time.Hour))
	st := stored(t, Tokens{IDToken: idToken("acct-1", "me@x"), AccessToken: access, RefreshToken: "r0", LastRefresh: is.now})
	src := NewSource(st, is.cfg())
	h, err := src.Headers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Authorization":      "Bearer " + access,
		"ChatGPT-Account-ID": "acct-1",
		"originator":         "codex_cli_rs",
		"User-Agent":         "codex_cli_rs/0.0.0 (VoxtMini)",
	}
	for k, v := range want {
		if h[k] != v {
			t.Errorf("%s = %q, want %q", k, h[k], v)
		}
	}
	if _, n, _ := is.counts(); n != 0 {
		t.Errorf("refreshed a fresh token")
	}
}

func TestRefreshOnExpiry(t *testing.T) {
	is := newIssuer(t)
	st := stored(t, Tokens{AccessToken: accessToken("a0", is.now.Add(4*time.Minute)), RefreshToken: "r0", AccountID: "acct-1", LastRefresh: is.now})
	src := NewSource(st, is.cfg())
	h, err := src.Headers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	is.mu.Lock()
	body := is.refreshes[0]
	is.mu.Unlock()
	if body["grant_type"] != "refresh_token" || body["refresh_token"] != "r0" || body["client_id"] != DefaultClientID {
		t.Errorf("refresh body %v", body)
	}
	got := st.tokens(t)
	if got.RefreshToken != "r2" || h["Authorization"] != "Bearer "+got.AccessToken || got.AccountID != "acct-1" || !got.LastRefresh.Equal(is.now) {
		t.Errorf("stored %+v headers %v", got, h)
	}
	// Fresh now: no second refresh.
	if _, err := src.Headers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := is.counts(); n != 1 {
		t.Errorf("refreshes = %d", n)
	}
}

func TestRefreshOnAge(t *testing.T) {
	is := newIssuer(t)
	// No exp in the access token: the 8-day rule decides.
	young := Tokens{AccessToken: "opaque", RefreshToken: "r0", LastRefresh: is.now.Add(-7 * 24 * time.Hour)}
	if _, err := NewSource(stored(t, young), is.cfg()).Headers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := is.counts(); n != 0 {
		t.Fatalf("refreshed a 7-day-old sign-in")
	}
	old := young
	old.LastRefresh = is.now.Add(-9 * 24 * time.Hour)
	st := stored(t, old)
	if _, err := NewSource(st, is.cfg()).Headers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, n, _ := is.counts(); n != 1 || st.tokens(t).RefreshToken != "r2" {
		t.Fatalf("refreshes = %d, stored %+v", n, st.tokens(t))
	}
}

func TestRefreshSingleFlight(t *testing.T) {
	is := newIssuer(t)
	is.refreshDelay = 50 * time.Millisecond
	st := stored(t, Tokens{AccessToken: accessToken("a0", is.now.Add(-time.Minute)), RefreshToken: "r0"})
	src := NewSource(st, is.cfg())
	var wg sync.WaitGroup
	auths := make([]string, 16)
	errs := make([]error, 16)
	for i := range auths {
		wg.Go(func() {
			h, err := src.Headers(context.Background())
			auths[i], errs[i] = h["Authorization"], err
		})
	}
	wg.Wait()
	for i := range auths {
		if errs[i] != nil || auths[i] != auths[0] {
			t.Fatalf("caller %d: %q %v (first %q)", i, auths[i], errs[i], auths[0])
		}
	}
	if _, n, _ := is.counts(); n != 1 {
		t.Fatalf("refreshes = %d, want 1", n)
	}
}

func TestRefreshRefusedSignsOut(t *testing.T) {
	for _, c := range []struct {
		status int
		body   string
	}{
		{400, `{"error":"invalid_grant","error_description":"r0 is bad"}`},
		{401, `{"error":{"code":"refresh_token_reused"}}`},
		{401, ``},
		{403, `{"code":"refresh_token_expired"}`},
	} {
		is := newIssuer(t)
		is.refreshStatus, is.refreshBody = c.status, c.body
		st := stored(t, Tokens{AccessToken: accessToken("a0", is.now), RefreshToken: "r0"})
		src := NewSource(st, is.cfg())
		_, err := src.Headers(context.Background())
		if !errors.Is(err, ErrSignedOut) || errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "r0") {
			t.Errorf("%d %s: err = %v", c.status, c.body, err)
		}
		// Remembered: no second request for the same refresh token.
		if _, err := src.Headers(context.Background()); !errors.Is(err, ErrSignedOut) {
			t.Errorf("second call: %v", err)
		}
		if _, n, _ := is.counts(); n != 1 {
			t.Errorf("%d: refreshes = %d", c.status, n)
		}
	}
}

func TestRefreshTransient(t *testing.T) {
	is := newIssuer(t)
	is.refreshStatus, is.refreshBody = 503, `oops`
	st := stored(t, Tokens{AccessToken: accessToken("a0", is.now), RefreshToken: "r0"})
	src := NewSource(st, is.cfg())
	if _, err := src.Headers(context.Background()); !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrSignedOut) {
		t.Fatalf("503: %v", err)
	}
	// Network down: a closed issuer.
	is.srv.Close()
	if _, err := src.Headers(context.Background()); !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrSignedOut) {
		t.Fatalf("closed: %v", err)
	}
}

func TestSignedOut(t *testing.T) {
	is := newIssuer(t)
	if _, err := NewSource(&memStore{}, is.cfg()).Headers(context.Background()); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("empty store: %v", err)
	}
	if _, err := NewSource(&memStore{v: "{"}, is.cfg()).Headers(context.Background()); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("corrupt store: %v", err)
	}
}

func TestSaveAndSignOut(t *testing.T) {
	is := newIssuer(t)
	st := &memStore{}
	src := NewSource(st, is.cfg())
	if err := src.Save(context.Background(), Tokens{IDToken: idToken("acct-9", "z@x"), AccessToken: accessToken("a", is.now.Add(time.Hour)), RefreshToken: "rx"}); err != nil {
		t.Fatal(err)
	}
	cur, _ := src.Current(context.Background())
	if cur.AccountID != "acct-9" || cur.Email != "z@x" || st.tokens(t).AccountID != "acct-9" {
		t.Errorf("current %+v", cur)
	}
	if err := src.SignOut(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v, _ := st.Get(); v != "" {
		t.Errorf("store after sign-out: %q", v)
	}
	is.mu.Lock()
	rv := is.revokes
	is.mu.Unlock()
	if len(rv) != 1 || rv[0]["token"] != "rx" || rv[0]["token_type_hint"] != "refresh_token" || rv[0]["client_id"] != DefaultClientID {
		t.Errorf("revokes %v", rv)
	}
	if _, err := src.Headers(context.Background()); !errors.Is(err, ErrSignedOut) {
		t.Errorf("after sign-out: %v", err)
	}
}

func TestSaveFailureRevokes(t *testing.T) {
	is := newIssuer(t)
	st := &memStore{fail: errors.New("keychain refused")}
	src := NewSource(st, is.cfg())
	err := src.Save(context.Background(), Tokens{AccessToken: accessToken("a", is.now.Add(time.Hour)), RefreshToken: "rz"})
	if err == nil {
		t.Fatal("saved into a failing store")
	}
	is.mu.Lock()
	rv := is.revokes
	is.mu.Unlock()
	if len(rv) != 1 || rv[0]["token"] != "rz" {
		t.Errorf("unsaved sign-in not revoked: %v", rv)
	}
	if _, err := src.Headers(context.Background()); !errors.Is(err, ErrSignedOut) {
		t.Errorf("after a failed save: %v", err)
	}
}

func TestRefreshKeptWhenStoreFails(t *testing.T) {
	is := newIssuer(t)
	st := stored(t, Tokens{AccessToken: accessToken("a0", is.now), RefreshToken: "r0"})
	st.fail = errors.New("keychain locked")
	src := NewSource(st, is.cfg())
	if _, err := src.Headers(context.Background()); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.fail = nil
	st.mu.Unlock()
	if _, err := src.Headers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st.tokens(t).RefreshToken != "r2" {
		t.Fatalf("rotated refresh token not saved on retry: %+v", st.tokens(t))
	}
}

func TestHeadersCancelledWhileWaiting(t *testing.T) {
	is := newIssuer(t)
	is.refreshDelay = 300 * time.Millisecond
	src := NewSource(stored(t, Tokens{AccessToken: accessToken("a0", is.now), RefreshToken: "r0"}), is.cfg())
	go src.Headers(context.Background())
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := src.Headers(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestPermanent(t *testing.T) {
	for _, c := range []struct {
		r    rejection
		want bool
	}{
		{rejection{400, "invalid_grant"}, true},
		{rejection{400, "invalid_request"}, false},
		{rejection{401, ""}, true},
		{rejection{500, ""}, false},
		{rejection{429, ""}, false},
		{rejection{400, "refresh_token_invalidated"}, true},
	} {
		if got := permanent(&c.r); got != c.want {
			t.Errorf("%+v: %v", c.r, got)
		}
	}
}
