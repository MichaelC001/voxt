package main

// The self-check's network, model disk and Keychain lines (Settings →
// Diagnostics), and the probe the Network dialog's "Test connection"
// shares with it: the update server and the download source, through
// the proxy (internal/netcheck). Offline is a warning: VoxtMini works
// without the network once its models are in.

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/netcheck"
	"github.com/hehehai/VoxtMini/internal/secrets"
	"github.com/hehehai/VoxtMini/internal/uitext"
	famsettings "github.com/hehehai/actnowkit/settings"
)

// updateHost is the update server (and the product's site) the probe
// asks.
const updateHost = "https://actnow.dev"

// modelsDiskLow is the free space under which the model library's disk
// is a warning: a model takes 1–5 GB.
const modelsDiskLow = 5 << 30

// reachHosts asks hosts through client (netcheck.All; tests answer
// without the network).
var reachHosts = netcheck.All

// keychainProbe reads an item that is not there: whether the Keychain
// answers, without a value or a prompt (tests: no Keychain).
var keychainProbe = func() error {
	_, err := secrets.Secret("self-check", "").Get()
	return err
}

// netProbe is what a connection test asks: the client through the proxy
// and the URLs, the update server first, then the download source's
// endpoints.
type netProbe struct {
	client *http.Client
	urls   []string
}

// newNetProbe is the probe of a proxy (a network.proxy value) and a
// download source (a network.hfEndpoints value); $HF_ENDPOINT replaces
// the source, as for the downloads (networkFor).
func newNetProbe(proxy, source string) netProbe {
	eps := famsettings.Endpoints(source)
	if e := os.Getenv("HF_ENDPOINT"); e != "" {
		eps = []string{strings.TrimRight(e, "/")}
	}
	return netProbe{client: models.NewHTTPClient(models.ProxyFor(proxy, systemProxy)), urls: append([]string{updateHost}, eps...)}
}

// savedNetProbe is the probe of the proxy and the download source
// VoxtMini uses.
func (a *app) savedNetProbe() netProbe {
	if a.fam == nil {
		return newNetProbe(famsettings.ProxyAuto, famsettings.EndpointsAuto)
	}
	return newNetProbe(a.fam.Effective(a.prefs, actnow.KeyProxy), a.fam.Effective(a.prefs, actnow.KeyHFEndpoints))
}

// run asks the hosts (blocks: off the main thread).
func (p netProbe) run() []netcheck.Result {
	return reachHosts(context.Background(), p.client, p.urls)
}

// latencies is "host 120 ms · host timeout": each result in short.
func latencies(rs []netcheck.Result) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		if r.OK {
			parts[i] = r.Host + " " + i18n.T("%d ms", r.Latency.Milliseconds())
		} else {
			parts[i] = r.Host + " " + netReason(r)
		}
	}
	return strings.Join(parts, " · ")
}

// netReason is why a host did not answer, for the views: netcheck's
// timeout in words, else the system's reason as it is.
func netReason(r netcheck.Result) string {
	if r.Err == netcheck.ErrTimeout {
		return i18n.T("timed out")
	}
	return r.Err
}

// netChecks are the self-check's lines of a probe's results: the update
// server, then the download source (one of its endpoints is enough).
func netChecks(rs []netcheck.Result, add func(id, title, status, detail string)) {
	if len(rs) == 0 {
		return
	}
	up, dl := rs[0], rs[1:]
	title := i18n.T("Update server")
	if up.OK {
		add("net-updates", title, "pass", latencies(rs[:1]))
	} else {
		add("net-updates", title, "warn", i18n.T("%s: no answer (%s). Updates are not checked.", up.Host, netReason(up)))
	}
	title = i18n.T("Model downloads")
	if netcheck.AnyOK(dl) {
		add("net-downloads", title, "pass", latencies(dl))
	} else {
		add("net-downloads", title, "warn", i18n.T("No download source answers (%s): check the proxy in Models → Network.", latencies(dl)))
	}
}

// modelsDiskCheck is the self-check's line of the model library's disk
// (hub: its folder, "" unknown).
func modelsDiskCheck(hub string, add func(id, title, status, detail string)) {
	if hub == "" {
		return
	}
	title := i18n.T("Model library")
	free := diskFree(hub)
	if free > 0 && free < modelsDiskLow {
		add("models-disk", title, "warn", i18n.T("Only %s free: a model takes 1–5 GB.", uitext.HumanBytes(free)))
		return
	}
	add("models-disk", title, "pass", i18n.T("%s free", uitext.HumanBytes(free)))
}

// keychainCheck is the self-check's line of the Keychain, where remote
// services' keys are (a warning: only they need it).
func keychainCheck(add func(id, title, status, detail string)) {
	if err := keychainProbe(); err != nil {
		add("keychain", i18n.T("Keychain"), "warn", i18n.T("Cannot be read: remote services cannot use their keys (%v).", err))
		return
	}
	add("keychain", i18n.T("Keychain"), "pass", i18n.T("Readable"))
}
