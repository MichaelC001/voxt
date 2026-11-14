package main

// API keys shared with the ActNow family (internal/actnow's credentials;
// family spec §8). A service either keeps its key in VoxtMini's own
// Keychain item (asr.<id>, llm.<id>) or names the family's key of its
// vendor (settings.Provider.CredentialRef), which lives once in the login
// Keychain ("ActNow" / "vendor.<id>") for every app that uses it: a key
// changed in one app changes in all (spec §8.3). The service dialog
// offers the family's key without reading it, and reads it only once the
// user chose "Use this key" (credentials.Store.Use). Simpler than
// Auralink's: no own copy of a shared key, and Remove stops using it here
// (deleting it only when no other app uses it) instead of asking.

import (
	"context"
	"errors"
	"log"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/remote"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/credentials"
)

// familyKeychain is the family's Keychain (the login one through
// /usr/bin/security); tests swap in actnow.FakeKeychain.
var familyKeychain = func() credentials.Keychain { return credentials.Keychain{} }

// famCreds is the family's keys, nil outside the family (set once with
// the models, read on any goroutine).
type famCreds = atomic.Pointer[credentials.Store]

// keyRing is the services' keyring: VoxtMini's own items, then the
// family's keys the services name (actnow.Ring).
func (a *app) keyRing() remote.Keyring {
	if a.fam != nil {
		a.famKeys.Store(a.fam.Credentials(familyKeychain()))
	}
	st := a.settings
	return actnow.Ring{
		Own: newKeyring(),
		Refs: func() map[string]string {
			if st == nil {
				return nil
			}
			return keyRefs(st.Get())
		},
		Shared: func() actnow.SharedKeys {
			if cs := a.famKeys.Load(); cs != nil {
				return cs
			}
			return nil
		},
	}
}

// keyRefs are the vendors of the services that use a family key, by
// account.
func keyRefs(s settings.Settings) map[string]string {
	out := map[string]string{}
	for _, kind := range []providers.Kind{providers.ASR, providers.LLM} {
		for _, p := range providerList(s, kind) {
			if v := actnow.RefVendor(p); v != "" {
				out[providers.Account(kind, p.ID)] = v
			}
		}
	}
	return out
}

func providerList(s settings.Settings, kind providers.Kind) []settings.Provider {
	if kind == providers.ASR {
		return s.Providers.ASR
	}
	return s.Providers.LLM
}

// sharedKeysChanged follows a change of the family's keys (another app
// saved, replaced or deleted one): the services on family keys are read
// again, and the open dialog's offer.
func (a *app) sharedKeysChanged() {
	var accounts []string
	for acc := range keyRefs(a.prefs) {
		accounts = append(accounts, acc)
	}
	slices.Sort(accounts)
	a.loadRemoteKeys(accounts)
	if a.rmDlg.open {
		a.loadDialogFamily()
	}
}

// ---- The dialog ----

// dialogFamily is the service dialog's part about the family's keys.
type dialogFamily struct {
	vendor string             // the family vendor of the service's key, "" for none
	offer  *credentials.Entry // the family's key of the vendor, as last read
	using  bool               // VoxtMini agreed to use it (among its users)
	others []string           // the other apps using it, by name
	ref    string             // the setup's CredentialRef as the dialog has it
	share  bool               // "Let other ActNow apps on this Mac use this key"
	chosen bool               // "Use this key" was clicked in this dialog
	joined bool               // … which made VoxtMini a user (undone on Cancel)
	other  bool               // "Enter another key": the offer set aside
	read   bool               // the offer was read once
}

// dialogVendor is the family vendor of the dialog's service as its form
// is now ("" for none, or outside the family).
func (a *app) dialogVendor() string {
	d := &a.rmDlg
	if a.famKeys.Load() == nil {
		return ""
	}
	v, ok := actnow.Vendor(providers.Account(d.kind, d.id), strings.TrimSpace(d.endpoint))
	if !ok {
		return ""
	}
	return v.ID
}

// loadDialogFamily reads what the family has of the dialog's vendor, in
// the background: its key's hint and users, never the key.
func (a *app) loadDialogFamily() {
	d := &a.rmDlg
	d.fam.vendor = a.dialogVendor()
	cs := a.famKeys.Load()
	if d.fam.vendor == "" || cs == nil {
		return
	}
	kind, id, vendor := d.kind, d.id, d.fam.vendor
	a.async(func() {
		e, ok := cs.Offer(vendor)
		others := cs.OtherUsers(vendor)
		a.post(func() {
			d := &a.rmDlg
			if !d.open || d.kind != kind || d.id != id || d.fam.vendor != vendor {
				return
			}
			d.fam.offer, d.fam.read = nil, true
			if ok {
				d.fam.offer = &e
			}
			d.fam.using = ok && slices.Contains(e.Users, cs.BundleID)
			d.fam.others = nil
			for _, b := range others {
				if n := actnow.AppName(b); !slices.Contains(d.fam.others, n) {
					d.fam.others = append(d.fam.others, n)
				}
			}
		})
	})
}

// offerShown: the dialog offers the family's key in place of the key
// field (spec §8.3): the service has no key of its own and uses none of
// the family's yet, the family has one, the user did not set the offer
// aside or type a key.
func (a *app) offerShown(mask string) bool {
	d := &a.rmDlg
	f := d.fam
	return f.offer != nil && f.ref == "" && mask == "" && !f.chosen && !f.other && !d.removeKey && strings.TrimSpace(d.key) == ""
}

// shareShown: the checkbox that shares the key shows when another ActNow
// app is installed and there is a key to share.
func (a *app) shareShown(mask string) bool {
	d := &a.rmDlg
	if d.fam.vendor == "" || a.famKeys.Load() == nil || d.removeKey || !a.familyOthers() {
		return false
	}
	return strings.TrimSpace(d.key) != "" || mask != "" || d.fam.chosen
}

// useSharedKey is "Use this key": VoxtMini joins the key's users (the
// user's consent); the service uses it once saved, and a test before
// that reads it.
func (a *app) useSharedKey() {
	d := &a.rmDlg
	cs, vendor, kind, id := a.famKeys.Load(), d.fam.vendor, d.kind, d.id
	if cs == nil || vendor == "" {
		return
	}
	wasUsing := d.fam.using
	a.async(func() {
		_, err := cs.Use(context.Background(), vendor)
		a.post(func() {
			d := &a.rmDlg
			if !d.open || d.kind != kind || d.id != id {
				return
			}
			if err != nil {
				log.Printf("family: using the shared %s key: %v", vendor, err)
				d.fam.offer = nil
				d.err, d.errMore = sharedKeyError(), err.Error()
				return
			}
			d.fam.chosen, d.fam.using, d.fam.share = true, true, true
			d.fam.joined = d.fam.joined || !wasUsing
			d.key, d.removeKey = "", false
			if e := d.fam.offer; e != nil && e.AppID != "" && d.appID == "" {
				d.appID = e.AppID // Doubao speech: its access token's App ID
			}
		})
	})
}

// dialogClosed undoes "Use this key" when the dialog closes unsaved.
func (a *app) dialogClosed(f dialogFamily) {
	cs := a.famKeys.Load()
	if !f.joined || cs == nil || a.vendorInUse(f.vendor, "") {
		return
	}
	a.async(func() {
		if err := cs.StopUsing(context.Background(), f.vendor); err != nil {
			log.Printf("family: %v", err)
		}
	})
}

// vendorInUse reports whether a saved service other than account uses
// the family's key of vendor (DashScope's speech and language services
// share one).
func (a *app) vendorInUse(vendor, account string) bool {
	for acc, v := range keyRefs(a.prefs) {
		if v == vendor && acc != account {
			return true
		}
	}
	return false
}

// ---- Saving ----

// keyPlan is what saving the dialog does with the service's key: decided
// on the main thread (dialogKeyPlan), run on a goroutine; it returns the
// CredentialRef the saved setup keeps.
type keyPlan struct {
	acc, label string
	p          settings.Provider
	keys       *remote.Keys
	cs         *credentials.Store // nil outside the family
	oldVendor  string             // the family key the saved setup used
	oldInUse   bool               // another service of VoxtMini uses oldVendor's key
	vendor     string             // the family vendor of the service now ("" none)
	share      bool               // the key is (to be) the family's
	typed      string
	remove     bool
	chosen     bool // "Use this key"
}

// dialogKeyPlan is the plan of saving p from the dialog.
func (a *app) dialogKeyPlan(p settings.Provider, label string) keyPlan {
	d := &a.rmDlg
	saved, _ := providers.Setup(a.prefs, d.kind, p.ID)
	acc := providers.Account(d.kind, p.ID)
	old := actnow.RefVendor(saved)
	pl := keyPlan{acc: acc, label: label, p: p, keys: a.rkeys, cs: a.famKeys.Load(),
		oldVendor: old, oldInUse: old != "" && a.vendorInUse(old, acc), vendor: a.dialogVendor(),
		typed: strings.TrimSpace(d.key), remove: d.removeKey, chosen: d.fam.chosen}
	switch {
	case pl.cs == nil || pl.vendor == "":
		pl.share = false
	case a.familyOthers():
		pl.share = d.fam.share
	default: // the checkbox is hidden: a shared key stays shared, a new one is not
		pl.share = saved.CredentialRef != "" || pl.chosen
	}
	return pl
}

// run does the plan (any goroutine).
func (pl keyPlan) run() (string, error) {
	ctx := context.Background()
	ref := ""
	if pl.share {
		ref = actnow.Ref(pl.vendor)
	}
	switch {
	case pl.typed != "" && pl.share:
		if err := pl.cs.Save(ctx, pl.vendor, pl.typed, actnow.SaveOptions(pl.vendor, pl.p)); err != nil {
			return "", err
		}
		if err := pl.keys.Set(pl.acc, pl.label, ""); err != nil { // no copy of its own
			return "", err
		}
	case pl.typed != "":
		if err := pl.keys.Set(pl.acc, pl.label, pl.typed); err != nil {
			return "", err
		}
	case pl.remove:
		if err := pl.keys.Set(pl.acc, pl.label, ""); err != nil {
			return "", err
		}
		return "", pl.leave(ctx, true)
	case pl.chosen && pl.share:
		if err := pl.keys.Set(pl.acc, pl.label, ""); err != nil {
			return "", err
		}
	default:
		if err := pl.reshare(ctx); err != nil {
			return "", err
		}
	}
	if pl.oldVendor != "" && actnow.Ref(pl.oldVendor) != ref {
		return ref, pl.leave(ctx, false)
	}
	return ref, nil
}

// reshare moves an unchanged key between VoxtMini's item and the
// family's when the checkbox changed: shared now, its own key goes to the
// family; no longer shared, the family's key becomes its own.
func (pl keyPlan) reshare(ctx context.Context) error {
	switch {
	case pl.share && pl.oldVendor != pl.vendor:
		own, err := pl.keys.Get(pl.acc)
		if err != nil || own == "" {
			return err
		}
		if err := pl.cs.Save(ctx, pl.vendor, own, actnow.SaveOptions(pl.vendor, pl.p)); err != nil {
			return err
		}
		return pl.keys.Set(pl.acc, pl.label, "")
	case !pl.share && pl.oldVendor != "" && pl.cs != nil:
		s, err := pl.cs.Secret(ctx, pl.oldVendor)
		if errors.Is(err, credentials.ErrNotConsented) || errors.Is(err, credentials.ErrMissing) {
			return nil
		}
		if err != nil {
			return err
		}
		return pl.keys.Set(pl.acc, pl.label, s)
	}
	return nil
}

// leave stops using the family key the service used, when no other
// service of VoxtMini uses it; removing deletes it when no other app
// uses it either.
func (pl keyPlan) leave(ctx context.Context, removing bool) error {
	if pl.oldVendor == "" || pl.oldInUse || pl.cs == nil {
		return nil
	}
	if removing && len(pl.cs.OtherUsers(pl.oldVendor)) == 0 {
		return pl.cs.DeleteForAll(ctx, pl.oldVendor)
	}
	return pl.cs.StopUsing(ctx, pl.oldVendor)
}
