package privnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/hehehai/VoxtMini/internal/core"
)

func TestPlainOK(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "[::1]": true, "localhost": true, "a.localhost": true,
		"10.0.0.1": true, "10.255.255.255": true, "9.255.255.255": false, "11.0.0.1": false,
		"172.16.0.1": true, "172.31.255.255": true, "172.15.255.255": false, "172.32.0.1": false,
		"192.168.1.20": true, "192.169.0.1": false,
		"100.64.0.1": true, "100.88.1.2": true, "100.127.255.255": true, "100.63.255.255": false, "100.128.0.1": false,
		"169.254.10.1": true, "fe80::1": true, "fe80::1%en0": true, "fd7a:115c:a1e0::1": true, "fc00::1": true,
		"2001:4860::8888": false, "fe00::1": false, "::ffff:10.1.2.3": true, "::ffff:8.8.8.8": false,
		"8.8.8.8": false, "0.0.0.0": false, "::": false, "": false,
		"gpubox": true, "gpubox.local": true, "gpubox.tail1234.ts.net": true, "box.lan": true, "api.openai.com": true, // names: checked at dial time
	} {
		if got := PlainOK(host); got != want {
			t.Errorf("PlainOK(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestDirect(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "127.0.0.1": true, "10.1.2.3": true, "100.100.1.1": true, "fd00::1": true,
		"gpubox": true, "gpubox.local": true, "GPUBOX.LOCAL.": true,
		"8.8.8.8": false, "api.openai.com": false, "box.tail1.ts.net": false,
	} {
		if got := Direct(host); got != want {
			t.Errorf("Direct(%q) = %v, want %v", host, got, want)
		}
	}
	if IsLoopback("10.0.0.1") || !IsLoopback("LOCALHOST") {
		t.Error("IsLoopback")
	}
}

// fakeResolver answers every name with its addresses.
type fakeResolver map[string][]string

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ips, ok := f[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	var out []net.IPAddr
	for _, s := range ips {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	return out, nil
}

// recordDial records the addresses dialled and connects every one to
// the test server.
type recordDial struct {
	target string
	got    []string
}

func (d *recordDial) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	d.got = append(d.got, addr)
	return (&net.Dialer{}).DialContext(ctx, network, d.target)
}

func client(d *recordDial, r Resolver, proxy func(*http.Request) (*url.URL, error)) *http.Client {
	return &http.Client{CheckRedirect: core.CheckRedirect, Transport: Guard(&http.Transport{DialContext: d.dial, Proxy: proxy}, r)}
}

func TestGuardPlain(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		io.WriteString(w, "ok")
	}))
	defer srv.Close()
	target := strings.TrimPrefix(srv.URL, "http://")
	_, port, _ := net.SplitHostPort(target)
	res := fakeResolver{
		"gpubox.tail1.ts.net": {"100.88.1.2"},
		"evil.example":        {"203.0.113.9"},
		"mixed.example":       {"192.168.1.5", "198.51.100.7"},
	}
	for _, tc := range []struct {
		url, dialled string
		err          error
	}{
		{"http://gpubox.tail1.ts.net:" + port + "/v1", "100.88.1.2:" + port, nil},
		{"http://10.0.0.7:" + port + "/v1", "10.0.0.7:" + port, nil},
		{"http://localhost:" + port + "/v1", "127.0.0.1:" + port, nil},
		{"http://evil.example:" + port + "/v1", "", ErrPublic},
		{"http://mixed.example:" + port + "/v1", "", ErrPublic},
		{"http://8.8.8.8:" + port + "/v1", "", ErrPublic},
	} {
		d := &recordDial{target: target}
		before := hits.Load()
		req, _ := http.NewRequest(http.MethodGet, tc.url, nil)
		req.Header.Set("Authorization", "Bearer sk-secret")
		resp, err := client(d, res, nil).Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		switch {
		case tc.err != nil && !errors.Is(err, tc.err):
			t.Errorf("%s: err %v, want %v", tc.url, err, tc.err)
		case tc.err != nil && (len(d.got) > 0 || hits.Load() != before):
			t.Errorf("%s: dialled %v", tc.url, d.got)
		case tc.err == nil && (err != nil || len(d.got) != 1 || d.got[0] != tc.dialled):
			t.Errorf("%s: err %v, dialled %v, want %s", tc.url, err, d.got, tc.dialled)
		}
	}
}

// TestGuardHTTPSUnchecked: https is dialled by name, as before (TLS
// checks the host).
func TestGuardHTTPSUnchecked(t *testing.T) {
	d := &recordDial{target: "127.0.0.1:1"}
	req, _ := http.NewRequest(http.MethodGet, "https://evil.example/v1", nil)
	client(d, fakeResolver{}, nil).Do(req)
	if len(d.got) != 1 || d.got[0] != "evil.example:443" {
		t.Errorf("dialled %v", d.got)
	}
}

// TestGuardProxy: plain requests and private hosts go direct; https to a
// public host keeps the proxy.
func TestGuardProxy(t *testing.T) {
	var proxied []string
	proxy := func(r *http.Request) (*url.URL, error) {
		proxied = append(proxied, r.URL.Host)
		return url.Parse("http://127.0.0.1:1")
	}
	tr := Guard(&http.Transport{Proxy: proxy}, fakeResolver{}).(guard).t
	for raw, want := range map[string]bool{
		"http://gpubox.tail1.ts.net:3425/v1": false, "http://10.0.0.1/v1": false,
		"https://192.168.1.2/v1": false, "https://gpubox/v1": false, "https://box.local/v1": false, "https://localhost/v1": false,
		"https://api.openai.com/v1": true,
	} {
		proxied = nil
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		u, err := tr.Proxy(req)
		if err != nil || (u != nil) != want || len(proxied) > 0 != want {
			t.Errorf("%s: proxy %v (%v), want proxied %v", raw, u, err, want)
		}
	}
}

// TestGuardRedirect: plain to plain on another private host is still
// refused (core.CheckRedirect), as is a downgrade.
func TestGuardRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.9.9.9/v1", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	d := &recordDial{target: strings.TrimPrefix(srv.URL, "http://")}
	_, err := client(d, fakeResolver{}, nil).Get(srv.URL)
	if !errors.Is(err, core.ErrRedirect) || len(d.got) != 1 {
		t.Errorf("err %v, dialled %v", err, d.got)
	}
}

func TestLocalNetwork(t *testing.T) {
	unreach := func(ip string) DialFunc {
		return func(context.Context, string, string) (net.Conn, error) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP(ip), Port: 80},
				Err: fmt.Errorf("connect: %w", syscall.EHOSTUNREACH)}
		}
	}
	for ip, want := range map[string]bool{"100.88.1.2": true, "192.168.1.2": true, "203.0.113.9": false} {
		_, err := guardDial(unreach(ip), fakeResolver{})(withPlain(context.Background()), "tcp", ip+":80")
		if ip == "203.0.113.9" {
			_, err = guardDial(unreach(ip), fakeResolver{})(context.Background(), "tcp", ip+":443")
		}
		if errors.Is(err, ErrLocalNetwork) != want || !errors.Is(err, syscall.EHOSTUNREACH) {
			t.Errorf("%s: %v", ip, err)
		}
	}
}
