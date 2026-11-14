package netcheck

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// TestReach: an answer below 500 is OK, 500 and up is not, nor a closed
// port; the reasons are short.
func TestReach(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound) // still an answer
	}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + l.Addr().String()
	l.Close()

	rs := All(context.Background(), http.DefaultClient, []string{ok.URL, bad.URL, closed})
	if !rs[0].OK || rs[0].Err != "" || rs[0].Host != ok.Listener.Addr().String() {
		t.Errorf("ok: %+v", rs[0])
	}
	if rs[1].OK || rs[1].Err != "502 Bad Gateway" {
		t.Errorf("502: %+v", rs[1])
	}
	if rs[2].OK || rs[2].Err != "connection refused" {
		t.Errorf("closed: %+v", rs[2])
	}
	if !AnyOK(rs) || AnyOK(rs[1:]) {
		t.Error("AnyOK")
	}
}

// TestReachProxyAndTimeout: the request goes through the client's proxy;
// a host that does not answer in time says "timeout".
func TestReachProxyAndTimeout(t *testing.T) {
	var asked atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Store(r.URL.String()) // a proxy gets the absolute URL
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	if r := Reach(context.Background(), client, "http://hub.example/"); !r.OK || r.Host != "hub.example" {
		t.Fatalf("through the proxy: %+v", r)
	}
	if got, _ := asked.Load().(string); got != "http://hub.example/" {
		t.Errorf("proxy asked %q", got)
	}

	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer slow.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if r := Reach(ctx, http.DefaultClient, slow.URL); r.OK || r.Err != "timeout" {
		t.Errorf("slow: %+v", r)
	}
}
