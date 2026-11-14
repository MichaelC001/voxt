package main

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/netcheck"
	"github.com/hehehai/VoxtMini/internal/settings"
	famsettings "github.com/hehehai/actnowkit/settings"
)

// probed records the hosts asked and the proxy of the client asked
// through, answering with fail's hosts down.
type probed struct {
	urls  []string
	proxy string
	fail  []string
}

func (p *probed) reach(ctx context.Context, client *http.Client, urls []string) []netcheck.Result {
	p.urls, p.proxy = urls, ""
	req, _ := http.NewRequest(http.MethodHead, "https://huggingface.co", nil)
	if tr, ok := client.Transport.(*http.Transport); ok && tr.Proxy != nil {
		if u, err := tr.Proxy(req); err == nil && u != nil {
			p.proxy = u.String()
		}
	}
	out := fakeReach(ctx, client, urls)
	for i := range out {
		if slices.Contains(p.fail, out[i].Host) {
			out[i].OK, out[i].Latency, out[i].Err = false, 0, netcheck.ErrTimeout
		}
	}
	return out
}

func swapReach(t *testing.T, p *probed) {
	old := reachHosts
	reachHosts = p.reach
	t.Cleanup(func() { reachHosts = old })
}

func checkStatus(a *app) map[string]string {
	out := map[string]string{}
	for _, c := range a.checks {
		out[c.id] = c.status + " " + c.detail
	}
	return out
}

// TestSelfCheckNetworkDiskKeychain: the self-check asks the update
// server and the download source through the proxy VoxtMini uses;
// offline is a warning; a low disk under the model library and an
// unreadable Keychain are warnings too, and say no secret.
func TestSelfCheckNetworkDiskKeychain(t *testing.T) {
	a, _, _ := newTestApp(t)
	p := &probed{}
	swapReach(t, p)
	fam := a.fam
	a.updateSettings(func(s *settings.Settings) {
		fam.Follow(s, actnow.KeyProxy, false) // VoxtMini's own values
		fam.Follow(s, actnow.KeyHFEndpoints, false)
		actnow.SetOwn(s, actnow.KeyProxy, "http://127.0.0.1:7890")
		actnow.SetOwn(s, actnow.KeyHFEndpoints, famsettings.EndpointsMirror)
	})
	a.runSelfCheck()
	s := checkStatus(a)
	if !slices.Equal(p.urls, []string{"https://actnow.dev", famsettings.MirrorEndpoint}) || p.proxy != "http://127.0.0.1:7890" {
		t.Errorf("asked %v through %q", p.urls, p.proxy)
	}
	if s["net-updates"] != "pass actnow.dev 120 ms" || s["net-downloads"] != "pass hf-mirror.com 120 ms" {
		t.Errorf("online: %v", s)
	}
	if s["keychain"] != "pass Readable" || !strings.HasPrefix(s["models-disk"], "pass ") {
		t.Errorf("keychain, disk: %v", s)
	}

	// Offline: warnings, never failures.
	a.updateSettings(func(s *settings.Settings) { actnow.SetOwn(s, actnow.KeyHFEndpoints, famsettings.EndpointsAuto) })
	p.fail = []string{"actnow.dev", "huggingface.co", "hf-mirror.com"}
	a.runSelfCheck()
	s = checkStatus(a)
	if s["net-updates"] != "warn actnow.dev: no answer (timed out). Updates are not checked." ||
		s["net-downloads"] != "warn No download source answers (huggingface.co timed out · hf-mirror.com timed out): check the proxy in Models → Network." {
		t.Errorf("offline: %v", s)
	}
	p.fail = []string{"huggingface.co"} // one source is enough
	a.runSelfCheck()
	if s = checkStatus(a); s["net-downloads"] != "pass huggingface.co timed out · hf-mirror.com 120 ms" {
		t.Errorf("mirror only: %v", s)
	}

	oldDisk, oldKC := diskFree, keychainProbe
	t.Cleanup(func() { diskFree, keychainProbe = oldDisk, oldKC })
	diskFree = func(string) int64 { return 3 << 30 }
	keychainProbe = func() error { return errors.New("security: User interaction is not allowed.") }
	a.runSelfCheck()
	s = checkStatus(a)
	if s["models-disk"] != "warn Only 3.2 GB free: a model takes 1–5 GB." {
		t.Errorf("low disk: %v", s["models-disk"])
	}
	if s["keychain"] != "warn Cannot be read: remote services cannot use their keys (security: User interaction is not allowed.)." {
		t.Errorf("keychain: %v", s["keychain"])
	}
	for _, c := range a.checks {
		if c.status == "fail" && strings.HasPrefix(c.id, "net") {
			t.Errorf("a network failure: %+v", c)
		}
	}
}

// TestNetworkTestConnection: the dialog tests the form before it is
// saved, through its proxy; a change hides the results; a custom proxy
// that is not one is not tested.
func TestNetworkTestConnection(t *testing.T) {
	a, _, tt := newTestApp(t)
	p := &probed{fail: []string{"huggingface.co"}}
	swapReach(t, p)
	a.openURL("voxtnew://models/network")
	click(t, tt, "Custom")
	a.netForm.proxyURL = "ftp://127.0.0.1:21"
	click(t, tt, "Test connection")
	if p.urls != nil || !has(tt, "Enter a proxy's address that starts with http://, https:// or socks5://, like http://127.0.0.1:7890.") {
		t.Fatalf("an ftp proxy was tested: %v %q", p.urls, tt.Texts())
	}
	a.netForm.proxyURL = "socks5://127.0.0.1:1080"
	click(t, tt, "Test connection")
	if p.proxy != "socks5://127.0.0.1:1080" || len(p.urls) != 3 {
		t.Fatalf("tested %v through %q", p.urls, p.proxy)
	}
	frames(tt)
	if !hasExact(tt, "hf-mirror.com") || !hasExact(tt, "120 ms") || !hasExact(tt, "timed out") || has(tt, "Enter a proxy's address") {
		t.Fatalf("results: %q", tt.Texts())
	}
	if a.prefs.Network.Proxy != "" {
		t.Errorf("the test saved the proxy: %q", a.prefs.Network.Proxy)
	}
	click(t, tt, "hf-mirror.com only")
	if hasExact(tt, "timed out") {
		t.Errorf("results of another source: %q", tt.Texts())
	}
	click(t, tt, "Cancel")
	a.openNetwork()
	if a.netForm.tested != nil {
		t.Errorf("results kept: %+v", a.netForm.tested)
	}
}
