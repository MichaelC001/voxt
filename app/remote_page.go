package main

// The remote services on the Models page (models_view.go;
// docs/remote-models.md; Auralink's Model services page): the speech
// (ASR) and language (LLM) services the user may send audio and text to,
// instead of a local model. A card per service (its state, model, saved
// key and a connection test), below the local models of the same kind;
// a card opens the service's dialog (remote_dialog.go), its context menu
// hides it (models_hide.go). A feature uses a remote service only when
// the user picks it on the feature's page (feature_models.go): automatic
// is always a local model. /remote, the Remote models page's path until
// 2026-11-12, opens the Models page on its Remote filter.
//
// Keys live in the Keychain only (internal/secrets, behind remote.Keys):
// the page reads their masks ("sk-…wxyz") in the background, never a key.
// What to build and test is internal/providers'.

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/logs"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/remote"
	"github.com/hehehai/VoxtMini/internal/secrets"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// remotePath was the Remote models page's path: the Models page's Remote
// filter now (modelsRoute).
const remotePath = "/remote"

// keychain is the macOS Keychain (internal/secrets).
type keychain struct{}

func (keychain) Get(account string) (string, error) { return secrets.Secret(account, "").Get() }
func (keychain) Set(account, label, value string) error {
	return secrets.Secret(account, label).Set(value)
}

// newKeyring is the keyring a new app uses; tests swap in a memory one.
var newKeyring = func() remote.Keyring { return keychain{} }

// remoteState is the remote services' state on the Models page.
type remoteState struct {
	// rkeys are the services' keys (also a.local's, internal/remote).
	rkeys *remote.Keys
	// rmKeyErr are the accounts whose key could not be read, and why.
	rmKeyErr map[string]string
	// rmTests are the last connection tests, by account.
	rmTests map[string]providerTest
	rmDlg   providerDialog
}

// providerTest is a connection test running or done.
type providerTest struct {
	running bool
	res     providers.TestResult
}

// remoteAccounts are the Keychain accounts of every service set up.
func remoteAccounts(s settings.Settings) []string {
	var out []string
	for _, p := range s.Providers.ASR {
		out = append(out, providers.Account(providers.ASR, p.ID))
	}
	for _, p := range s.Providers.LLM {
		out = append(out, providers.Account(providers.LLM, p.ID))
	}
	return out
}

// loadRemoteKeys reads accounts' keys in the background (their masks
// and whether one is saved: a service is ready once its key is read).
func (a *app) loadRemoteKeys(accounts []string) {
	if len(accounts) == 0 || a.rkeys == nil {
		return
	}
	keys := a.rkeys
	a.async(func() {
		errs := map[string]string{}
		for _, acc := range accounts {
			if _, err := keys.Get(acc); err != nil {
				errs[acc] = err.Error()
			}
		}
		a.post(func() {
			if a.rmKeyErr == nil {
				a.rmKeyErr = map[string]string{}
			}
			for _, acc := range accounts {
				delete(a.rmKeyErr, acc)
				if e, ok := errs[acc]; ok {
					a.rmKeyErr[acc] = e
				}
			}
			a.checkModels() // shell_notices.go: a remote service may be ready now
		})
	})
}

// keyMask is account's key's mask ("" none) and whether it was read.
func (a *app) keyMask(account string) (string, bool) {
	if a.rkeys == nil {
		return "", true
	}
	return a.rkeys.Mask(account)
}

// hasKey says whether account holds a key, as last read (memory only).
func (a *app) hasKey(account string) bool { return a.rkeys != nil && a.rkeys.Has(account) }

// ---- The cards ----

// providerCards lists kind's services: those set up (mine) first, then
// the rest, in a grid of cards.
func (a *app) providerCards(c *ui.Context, kind providers.Kind, mine, rest []providers.Preset) {
	t := c.Theme()
	room := min(a.pageWidth(c), px(t, 880)) - px(t, 2*sp6) - 1 // the column's padding
	// Cards at least 260 wide: two columns in the default window (800
	// wide) and the smallest.
	cols := max(1, int((room+px(t, sp3))/(px(t, 260)+px(t, sp3))))
	cardW := (room - px(t, sp3)*float32(cols-1)) / float32(cols)
	grid := func(key string, list []providers.Preset) {
		ui.Row(c.Key(key)).Wrap().Gap(px(t, sp3)).AlignItems(ui.Start).Children(func() {
			for _, pr := range list {
				a.providerCard(c, kind, pr, cardW)
			}
		})
	}
	tip := i18n.T("A service gets your recordings or text only when a feature uses it.")
	if len(mine) > 0 {
		sectionTitleTip(c, i18n.T("Remote services you set up"), tip)
		grid("rm-mine-"+string(kind), mine)
	}
	if len(rest) > 0 {
		title := i18n.T("More remote services")
		if len(mine) == 0 {
			title = i18n.T("Remote services")
		}
		sectionTitleTip(c, title, tip)
		grid("rm-rest-"+string(kind), rest)
	}
}

// providerState is a card's state line: its dot's colour and text.
func (a *app) providerState(c *ui.Context, kind providers.Kind, p settings.Provider, configured bool) (ui.Color, string) {
	k := tok(c.Theme())
	acc := providers.Account(kind, p.ID)
	if _, read := a.keyMask(acc); configured && !read {
		return k.G[5], i18n.T("Checking…")
	}
	switch providers.Missing(kind, p, configured, a.hasKey(acc)) {
	case providers.MissingSetup:
		return k.G[5], i18n.T("Not set up")
	case providers.MissingOff:
		return k.G[5], i18n.T("Off")
	case providers.MissingKey:
		return k.Warning, i18n.T("No key")
	case providers.MissingEndpoint:
		return k.Warning, i18n.T("No endpoint")
	}
	if tr, ok := a.rmTests[acc]; ok && !tr.running && tr.res.Err != nil {
		return k.Danger, i18n.T("Test failed")
	}
	return k.Success, i18n.T("Ready")
}

// providerCard is a service's card: its state dot, name and model; once
// set up, its key's mask, its last test and a Test button. A click opens
// its dialog.
func (a *app) providerCard(c *ui.Context, kind providers.Kind, pr providers.Preset, w float32) {
	t := c.Theme()
	k := tok(t)
	p, configured := providers.Setup(a.prefs, kind, pr.ID)
	name := uitext.ProviderName(kind, p)
	acc := providers.Account(kind, p.ID)
	dot, state := a.providerState(c, kind, p, configured)
	missing := providers.Missing(kind, p, configured, a.hasKey(acc))
	hideKey := settings.HiddenRemote(string(kind), pr.ID)
	hidden := a.prefs.Models.IsHidden(hideKey)
	b := ui.ButtonBase(c.Key("rm-card-"+acc)).Width(w).Padding(px(t, sp3), px(t, sp4)).Radius(px(t, radiusCard)).Border(1, k.Line).
		Label(name).Tooltip(i18n.T("Set up %s", name)).AlignItems(ui.Stretch).Shrink(0).
		ContextMenu(func(m *ui.Menu) { a.providerMenu(m, kind, pr.ID, p, configured, missing, hideKey, name) })
	if hidden {
		b.Opacity(0.6)
	}
	if b.Hovered() {
		b.Background(k.Subtle)
	}
	b.Children(func() {
		ui.Column(c).Gap(px(t, 6)).Grow(1).MinWidth(0).Children(func() {
			// The name on one line (all of it in the tooltip), then ⋯.
			ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).MinHeight(px(t, 22)).Children(func() {
				statusDot(c, dot)
				truncText(c, name).FontWeight(600).Grow(1).Shrink(1)
				if hidden {
					tagChip(c, i18n.T("Hidden"))
				}
				moreMenu(c.Key("rm-more-"+acc), i18n.T("More actions for %s", name), func(m *ui.Menu) {
					a.providerMenu(m, kind, pr.ID, p, configured, missing, hideKey, name)
				})
			})
			model := providers.Model(kind, p)
			if model == "" {
				model = i18n.T("Model not chosen")
			}
			// One caption line: the mode, the model, then (set up) its key
			// (an icon; its mask in the tooltip, a problem in words) and its
			// test's result or what is missing.
			ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).MinHeight(px(t, 20)).Children(func() {
				tagChip(c, uitext.ModeLabel(pr))
				truncText(c, model).FontSize(px(t, fsCaption)).TextColor(k.Text2).Shrink(1)
				ui.Box(c).Grow(1)
				if !configured {
					return // the rest shows once it is set up
				}
				mask, read := a.keyMask(acc)
				switch {
				case mask != "":
					iconMark(c.Key("rm-key-"+acc), icons.KeyMinimalisticSquare, k.Text2, mask)
				case !read:
				case a.rmKeyErr[acc] != "":
					statusMark(c, icons.KeyMinimalisticSquare, k.Warning, i18n.T("Could not read the key")).Shrink(1)
				case missing == providers.MissingKey:
					statusMark(c, icons.KeyMinimalisticSquare, k.Warning, i18n.T("No key")).Shrink(0)
				default:
					iconMark(c.Key("rm-key-"+acc), icons.KeyMinimalisticSquare, k.Text3, i18n.T("No key needed"))
				}
				switch missing {
				case "":
					if tr, ok := a.rmTests[acc]; ok {
						testStatus(c, tr, false)
					}
				case providers.MissingKey:
				default:
					ui.Text(c, state).FontSize(px(t, fsCaption)).TextColor(k.Text3).SingleLine().Shrink(0)
				}
			})
		})
	})
	if b.Clicked() {
		a.openProviderDialog(kind, pr.ID)
	}
}

// providerMenu is a service card's ⋯ and context menu: its dialog,
// a connection test once it is set up, hiding it.
func (a *app) providerMenu(m *ui.Menu, kind providers.Kind, id string, p settings.Provider, configured bool, missing, hideKey, name string) {
	label := i18n.T("Set up")
	if configured {
		label = i18n.T("Settings")
	}
	if m.Item(label).Chosen() {
		a.openProviderDialog(kind, id)
	}
	if configured && missing == "" {
		if m.Item(i18n.T("Test")).Disabled(a.rmTests[providers.Account(kind, id)].running).Chosen() {
			a.testProvider(kind, p)
		}
	}
	a.hideMenu(m, hideKey, name) // models_hide.go
}

// remoteProxy is the family's network.proxy rule now (any goroutine:
// the settings and the family's shared settings are read from memory).
func (a *app) remoteProxy() func() func(*http.Request) (*url.URL, error) {
	fam, st := a.fam, a.settings
	return func() func(*http.Request) (*url.URL, error) {
		if fam == nil || st == nil {
			return models.ProxyFor("", systemProxy)
		}
		return models.ProxyFor(fam.Effective(st.Get(), actnow.KeyProxy), systemProxy)
	}
}

// testProvider runs a connection test of a saved setup.
func (a *app) testProvider(kind providers.Kind, p settings.Provider) {
	acc := providers.Account(kind, p.ID)
	if a.rmTests == nil {
		a.rmTests = map[string]providerTest{}
	}
	a.rmTests[acc] = providerTest{running: true}
	keys, proxy, logs := a.rkeys, a.remoteProxy(), a.logs
	a.async(func() {
		key, err := keys.Get(acc)
		res := providers.TestResult{Err: err, At: time.Now()}
		if err == nil {
			res = runProviderTest(context.Background(), kind, p, providers.Env{Key: key, Proxy: proxy()})
		}
		logTest(logs, kind, p, res, key)
		a.post(func() { a.rmTests[acc] = providerTest{res: res} })
	})
}

// logTest writes a connection test's outcome to the app log (any
// goroutine): the service, model, time and error, never the key.
func logTest(r *logs.Ring, kind providers.Kind, p settings.Provider, res providers.TestResult, key string) {
	if r == nil {
		return
	}
	level, msg := providers.TestLog(kind, p, res, key)
	r.Append(core.LogEntry{Time: time.Now(), Level: core.Severity(level), Source: "app", Message: msg})
}

// runProviderTest is providers.Test; tests swap it for a fake.
var runProviderTest = providers.Test
