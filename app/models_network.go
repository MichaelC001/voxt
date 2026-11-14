package main

// The Local models page's Network… dialog (voxtnew://models/network):
// how model downloads reach the Hub, two of the ActNow family's shared
// settings (family spec §9.1): the proxy (automatic: the environment's,
// else the Mac's; none; or a proxy's URL) and the download source
// (Hugging Face and hf-mirror.com raced, or one of them). While VoxtMini
// follows the family (the default), a change applies to every ActNow app
// (family.go writes it to the family); "Use a different setting for this
// app" keeps it to VoxtMini. A key whose value differed from the family's
// at the first start shows "Use theirs" once (spec §9.4).

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/netcheck"
	"github.com/hehehai/VoxtMini/internal/settings"
	famsettings "github.com/hehehai/actnowkit/settings"
)

// networkPath is the Network dialog's deep link (actnow.Links).
const networkPath = modelsPath + "/network"

// The proxy's modes, as the dialog's segments.
const (
	proxyAuto = iota
	proxyNone
	proxyCustom
)

// The download sources, as the dialog's segments (a list of URLs another
// app set is a fourth, shown while it is the value).
var networkSources = []string{famsettings.EndpointsAuto, famsettings.EndpointsOfficial, famsettings.EndpointsMirror}

// networkForm is the Network dialog's fields.
type networkForm struct {
	proxyMode int
	proxyURL  string
	proxyErr  string
	source    int    // index in networkSources, or len(networkSources): sourceURLs
	sourceURL string // a list of endpoints another app set ("" none); settings.json does not say which app
	// testing: a connection test runs; tested are its results, for
	// testedFor (the proxy and the source tested: a change hides them).
	testing   bool
	tested    []netcheck.Result
	testedFor [2]string
}

// openNetwork opens the Network dialog with the values VoxtMini uses.
func (a *app) openNetwork() {
	a.netForm = networkForm{}
	a.fillNetwork(actnow.KeyProxy)
	a.fillNetwork(actnow.KeyHFEndpoints)
	a.netOpen = true
}

// fillNetwork sets the form's fields of key to the value VoxtMini uses.
func (a *app) fillNetwork(key string) {
	f := &a.netForm
	v := a.fam.Effective(a.prefs, key)
	switch key {
	case actnow.KeyProxy:
		f.proxyErr = ""
		switch v {
		case famsettings.ProxyAuto:
			f.proxyMode, f.proxyURL = proxyAuto, ""
		case famsettings.ProxyDirect:
			f.proxyMode, f.proxyURL = proxyNone, ""
		default:
			f.proxyMode, f.proxyURL = proxyCustom, v
		}
	case actnow.KeyHFEndpoints:
		if v == "" {
			v = famsettings.EndpointsAuto
		}
		f.source, f.sourceURL = slices.Index(networkSources, v), ""
		if f.source < 0 {
			f.source, f.sourceURL = len(networkSources), v
		}
	}
}

// values are the form's proxy and download source (network.proxy and
// network.hfEndpoints values); false for a custom proxy that is not one.
func (f *networkForm) values() (proxy, source string, ok bool) {
	proxy = famsettings.ProxyAuto
	switch f.proxyMode {
	case proxyNone:
		proxy = famsettings.ProxyDirect
	case proxyCustom:
		v, err := famsettings.Normalize(actnow.KeyProxy, f.proxyURL)
		if err != nil || v == famsettings.ProxyAuto || v == famsettings.ProxyDirect {
			return "", "", false
		}
		proxy = v
	}
	source = f.sourceURL
	if f.source < len(networkSources) {
		source = networkSources[f.source]
	}
	return proxy, source, true
}

// formNetwork is the form's values, showing the proxy's error when it
// is not one.
func (a *app) formNetwork() (proxy, source string, ok bool) {
	proxy, source, ok = a.netForm.values()
	a.netForm.proxyErr = ""
	if !ok {
		a.netForm.proxyErr = i18n.T("Enter a proxy's address that starts with http://, https:// or socks5://, like http://127.0.0.1:7890.")
	}
	return proxy, source, ok
}

// saveNetwork checks the form and makes it VoxtMini's settings (the
// family's, for the keys VoxtMini follows); it reports whether it did.
func (a *app) saveNetwork() bool {
	proxy, source, ok := a.formNetwork()
	if !ok {
		return false
	}
	a.updateSettings(func(s *settings.Settings) {
		actnow.SetOwn(s, actnow.KeyProxy, proxy)
		actnow.SetOwn(s, actnow.KeyHFEndpoints, source)
	})
	return true
}

// testNetwork asks the update server and the form's download source
// through the form's proxy, before it is saved (settings_about_net.go,
// as the self-check).
func (a *app) testNetwork() {
	proxy, source, ok := a.formNetwork()
	if !ok || a.netForm.testing {
		return
	}
	a.netProbeSeq++
	seq, probe := a.netProbeSeq, newNetProbe(proxy, source)
	a.netForm.testing, a.netForm.tested = true, nil
	a.async(func() {
		rs := probe.run()
		a.post(func() {
			if seq != a.netProbeSeq || !a.netOpen {
				return // the dialog closed, or opened again
			}
			a.netForm.testing, a.netForm.tested, a.netForm.testedFor = false, rs, [2]string{proxy, source}
		})
	})
}

// netTestRows are the connection test's results while the form is what
// was tested: a mark, the host, the time or why it did not answer.
func (a *app) netTestRows(c *ui.Context) {
	t := c.Theme()
	f := &a.netForm
	ui.Row(c.Key("net-test")).Gap(px(t, sp2)).AlignItems(ui.Center).Children(func() {
		if ui.Button(c.Key("net-test-run"), i18n.T("Test connection")).Disabled(f.testing).Clicked() {
			a.testNetwork()
		}
		if f.testing {
			ui.Spinner(c)
		}
	})
	if f.testing || len(f.tested) == 0 {
		return
	}
	if proxy, source, ok := f.values(); !ok || [2]string{proxy, source} != f.testedFor {
		return
	}
	for _, r := range f.tested {
		mark, col, detail := icons.CheckCircle, t.Success, i18n.T("%d ms", r.Latency.Milliseconds())
		if !r.OK {
			mark, col, detail = icons.CloseCircle, t.Danger, netReason(r)
		}
		ui.Row(c.Key("net-test-" + r.Host)).Gap(px(t, sp2)).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, mark.Linear).Size(px(t, 14), px(t, 14)).TextColor(col).Shrink(0)
			ui.Text(c, r.Host).Width(px(t, 160)).SingleLine()
			caption(c, detail).Grow(1).MinWidth(0)
		})
	}
}

// followNetwork makes VoxtMini follow the family for key (on: "Use
// theirs", "Use the same setting…"), or keep its own value (off).
func (a *app) followNetwork(key string, on bool) {
	fam := a.fam
	a.updateSettings(func(s *settings.Settings) { fam.Follow(s, key, on) })
	a.fillNetwork(key)
}

// closeNetwork closes the dialog; the one-time notices of a different
// value were seen, so they go.
func (a *app) closeNetwork() {
	a.netOpen = false
	if slices.ContainsFunc(a.prefs.Family.Conflicts, isNetworkKey) {
		a.updateSettings(func(s *settings.Settings) {
			s.Family.Conflicts = slices.DeleteFunc(slices.Clone(s.Family.Conflicts), isNetworkKey)
		})
	}
}

func isNetworkKey(key string) bool { return key == actnow.KeyProxy || key == actnow.KeyHFEndpoints }

// networkDialog is the Network dialog.
// netHeading is a heading of the Network dialog with a ⓘ telling what
// the choice below it does.
func netHeading(c *ui.Context, title, tip string) {
	t := c.Theme()
	ui.Row(c.Key("net-h-" + title)).Gap(px(t, 6)).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontWeight(600)
		infoTip(c, tip)
	})
}

func (a *app) networkDialog(c *ui.Context) {
	if !a.netOpen {
		return
	}
	t := c.Theme()
	f := &a.netForm
	open := true
	dialogFrame(c, "net-dlg", &open, i18n.T("Network"), i18n.T("How model downloads reach Hugging Face"), 540, func() {
		proxyTip := i18n.T("The proxy in the Mac's network settings, or the one the environment names.")
		if f.proxyMode == proxyNone {
			proxyTip = i18n.T("Downloads connect to the server directly.")
		}
		netHeading(c, i18n.T("Proxy"), proxyTip)
		segmentedFit(c.Key("net-proxy"), &f.proxyMode, i18n.T("Automatic"), i18n.T("No proxy"), i18n.T("Custom")).Label(i18n.T("Proxy"))
		if f.proxyMode != proxyAuto && f.proxyMode != proxyNone {
			ui.TextInput(c.Key("net-proxy-url"), &f.proxyURL).Label(i18n.T("Proxy address")).Placeholder("http://127.0.0.1:7890").Font("monospace").Width(px(t, 320))
			if f.proxyErr != "" {
				caption(c, f.proxyErr).TextColor(t.Danger)
			}
		}
		a.familyLine(c, actnow.KeyProxy, i18n.T("Other ActNow apps use a different proxy."), a.followNetwork, nil)

		ui.Box(c).Height(px(t, sp2)).Shrink(0)
		sourceTip := i18n.T("Hugging Face and its mirror hf-mirror.com, whichever answers first. Every file is checked, so either is safe.")
		if f.source > 0 && f.source < len(networkSources) {
			sourceTip = i18n.T("Only this server: when it cannot be reached, downloads fail.")
		}
		netHeading(c, i18n.T("Download source"), sourceTip)
		labels := []string{i18n.T("Automatic"), i18n.T("Hugging Face only"), i18n.T("hf-mirror.com only")}
		if f.sourceURL != "" {
			labels = append(labels, i18n.T("Other"))
		}
		segmentedFit(c.Key("net-source"), &f.source, labels...).Label(i18n.T("Download source"))
		if f.source >= len(networkSources) {
			caption(c, i18n.T("Custom servers: %s", f.sourceURL))
		}
		a.familyLine(c, actnow.KeyHFEndpoints, i18n.T("Other ActNow apps use a different download source."), a.followNetwork, nil)

		ui.Box(c).Height(px(t, sp2)).Shrink(0)
		a.netTestRows(c)
	}, func() {
		ui.Spacer(c)
		if ui.Button(c.Key("net-cancel"), i18n.T("Cancel")).Clicked() {
			open = false
		}
		if primaryButton(c.Key("net-save"), i18n.T("Save")).Clicked() && a.saveNetwork() {
			open = false
		}
	})
	if !open {
		a.closeNetwork()
	}
}

// keySuffix is a shared key as a part of an element key.
func keySuffix(key string) string { return strings.ReplaceAll(key, ".", "-") }
