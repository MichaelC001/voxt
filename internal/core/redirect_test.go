package core

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestCheckRedirect: a redirect to another host is refused before any
// request reaches it, keys and all; one on the same host is followed.
func TestCheckRedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
		case "/here":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/ok":
			if r.Header.Get("x-api-key") == "" {
				w.WriteHeader(http.StatusUnauthorized)
			}
		}
	}))
	defer srv.Close()
	c := &http.Client{CheckRedirect: CheckRedirect}
	get := func(path string) (*http.Response, error) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("x-api-key", "sk-secret")
		return c.Do(req)
	}
	if _, err := get("/away"); !errors.Is(err, ErrRedirect) || !strings.Contains(err.Error(), "another host") {
		t.Errorf("cross-host: %v", err)
	}
	if elsewhere.Load() != 0 {
		t.Error("the other host was reached")
	}
	if resp, err := get("/here"); err != nil || resp.StatusCode != http.StatusOK {
		t.Errorf("same host: %v %v", resp, err)
	}
	if _, err := get("/loop"); !errors.Is(err, ErrRedirect) {
		t.Errorf("loop: %v", err)
	}
}

func TestNoDowngrade(t *testing.T) {
	req := func(u string) *http.Request { r, _ := http.NewRequest(http.MethodGet, u, nil); return r }
	if err := NoDowngrade(req("http://a.example/x"), []*http.Request{req("https://a.example/")}); !errors.Is(err, ErrRedirect) {
		t.Errorf("https → http: %v", err)
	}
	if err := NoDowngrade(req("https://b.example/x"), []*http.Request{req("https://a.example/")}); err != nil {
		t.Errorf("https → https elsewhere: %v", err)
	}
	if err := CheckRedirect(req("http://a.example/x"), []*http.Request{req("https://a.example/")}); !errors.Is(err, ErrRedirect) {
		t.Errorf("CheckRedirect https → http: %v", err)
	}
}
