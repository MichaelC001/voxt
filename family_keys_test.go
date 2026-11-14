package main

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/remote"
	"github.com/hehehai/actnowkit/credentials"
	"github.com/hehehai/actnowkit/family"
	"github.com/hehehai/actnowkit/registry"
)

// familyKeysApp is a test app on a fake family Keychain, with Auralink
// installed beside it (the share checkbox shows) and Auralink's store of
// the family's keys.
func familyKeysApp(t *testing.T) (*app, *ui.Tester, *memKeyring, *actnow.FakeKeychain, *credentials.Store) {
	t.Helper()
	kc := actnow.NewFakeKeychain()
	old := familyKeychain
	familyKeychain = kc.Keychain
	t.Cleanup(func() { familyKeychain = old })
	ring := &memKeyring{keys: map[string]string{}}
	a, tt := remoteApp(t, ring)
	if a.fam == nil {
		a.openFamily()
	}
	oldRing := newKeyring
	newKeyring = func() remote.Keyring { return ring }
	t.Cleanup(func() { newKeyring = oldRing })
	a.rkeys = remote.NewKeys(a.keyRing())
	a.local = remote.New(remote.Config{Local: readyLocal(), Settings: a.lib.settings.Get, Keys: a.rkeys})
	if a.famKeys.Load() == nil {
		t.Fatal("no family keys")
	}
	au, _ := family.AppByID("auralink")
	a.famApps = []registry.Found{{BundleID: au.BundleID, App: au, Variant: family.Release, Name: au.Name, Installed: true}}
	other := &credentials.Store{Family: a.fam.Dir(), BundleID: au.BundleID, Keychain: kc.Keychain()}
	a.navigate(remotePath)
	a.mdTab = 1
	return a, tt, ring, kc, other
}

// TestFamilyKeyUse: Auralink's DeepSeek key is offered without being
// read; "Use this key" and Save make the service name it, with no copy
// of its own, and it is ready with that key. Removed, VoxtMini stops
// using it and Auralink keeps it.
func TestFamilyKeyUse(t *testing.T) {
	a, tt, ring, kc, other := familyKeysApp(t)
	ctx := context.Background()
	if err := other.Save(ctx, "deepseek", "sk-auralink-a1b2", credentials.SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	a.openProviderDialog(providers.LLM, "deepseek")
	frames(tt)
	if !has(tt, "Use the DeepSeek key from Auralink (sk-…a1b2)") {
		t.Fatalf("no offer: %q", firstTexts(tt))
	}
	cs := a.famKeys.Load()
	if cs.Using("deepseek") {
		t.Fatal("used before the click")
	}
	click(t, tt, "Use this key")
	frames(tt)
	if !cs.Using("deepseek") || !has(tt, "Uses the key from Auralink (sk-…a1b2).") {
		t.Fatalf("not used: %q", firstTexts(tt))
	}
	click(t, tt, "Save")
	frames(tt)
	p, _ := providers.Setup(a.lib.settings.Get(), providers.LLM, "deepseek")
	if a.rmDlg.open || p.CredentialRef != "actnow:vendor.deepseek" || ring.keys["llm.deepseek"] != "" {
		t.Fatalf("saved %+v, own %v, err %q", p, ring.keys, a.rmDlg.err)
	}
	if k, _ := a.rkeys.Get("llm.deepseek"); k != "sk-auralink-a1b2" || !a.hasKey("llm.deepseek") {
		t.Fatalf("key %q", k)
	}

	// Removed here: Auralink keeps its key, VoxtMini leaves its users.
	a.openProviderDialog(providers.LLM, "deepseek")
	frames(tt)
	click(t, tt, "Remove the saved key")
	click(t, tt, "Save")
	frames(tt)
	p, _ = providers.Setup(a.lib.settings.Get(), providers.LLM, "deepseek")
	if p.CredentialRef != "" || cs.Using("deepseek") || kc.Item("vendor.deepseek") != "sk-auralink-a1b2" || a.hasKey("llm.deepseek") {
		t.Fatalf("removed: %+v using %v item %q", p, cs.Using("deepseek"), kc.Item("vendor.deepseek"))
	}
}

// TestFamilyKeyShare: a key typed is shared by default (one family item,
// no own copy); unchecked, it stays VoxtMini's own; a shared key only
// VoxtMini uses is deleted when removed.
func TestFamilyKeyShare(t *testing.T) {
	a, tt, ring, kc, other := familyKeysApp(t)
	a.openProviderDialog(providers.LLM, "deepseek")
	frames(tt)
	click(t, tt, "Key for DeepSeek")
	tt.Type("sk-voxtmini-9999")
	frames(tt)
	if !has(tt, "Let other ActNow apps on this Mac use this key") || !a.rmDlg.fam.share {
		t.Fatalf("no checkbox: %q", firstTexts(tt))
	}
	click(t, tt, "Save")
	frames(tt)
	p, _ := providers.Setup(a.lib.settings.Get(), providers.LLM, "deepseek")
	if p.CredentialRef == "" || kc.Item("vendor.deepseek") != "sk-voxtmini-9999" || ring.keys["llm.deepseek"] != "" {
		t.Fatalf("shared: %+v item %q own %v", p, kc.Item("vendor.deepseek"), ring.keys)
	}
	if e, ok := other.Offer("deepseek"); !ok || e.Hint != "sk-…9999" {
		t.Fatalf("offer %+v", e)
	}

	// Unshared: the family's key becomes VoxtMini's own.
	a.openProviderDialog(providers.LLM, "deepseek")
	frames(tt)
	click(t, tt, "Let other ActNow apps on this Mac use this key")
	click(t, tt, "Save")
	frames(tt)
	p, _ = providers.Setup(a.lib.settings.Get(), providers.LLM, "deepseek")
	if p.CredentialRef != "" || ring.keys["llm.deepseek"] != "sk-voxtmini-9999" || a.famKeys.Load().Using("deepseek") {
		t.Fatalf("unshared: %+v own %v", p, ring.keys)
	}

	// A new key not shared stays VoxtMini's.
	a.openProviderDialog(providers.LLM, "anthropic")
	frames(tt)
	click(t, tt, "Key for Anthropic")
	tt.Type("sk-ant-own-0000")
	frames(tt)
	click(t, tt, "Let other ActNow apps on this Mac use this key")
	click(t, tt, "Save")
	if ring.keys["llm.anthropic"] != "sk-ant-own-0000" || kc.Item("vendor.anthropic") != "" {
		t.Fatalf("own: %v item %q", ring.keys, kc.Item("vendor.anthropic"))
	}

	// Shared again, then removed: only VoxtMini used it, so it is gone.
	a.openProviderDialog(providers.LLM, "deepseek")
	frames(tt)
	click(t, tt, "Let other ActNow apps on this Mac use this key")
	click(t, tt, "Save")
	frames(tt)
	if kc.Item("vendor.deepseek") != "sk-voxtmini-9999" || ring.keys["llm.deepseek"] != "" {
		t.Fatalf("reshared: item %q own %v", kc.Item("vendor.deepseek"), ring.keys)
	}
	a.openProviderDialog(providers.LLM, "deepseek")
	frames(tt)
	click(t, tt, "Remove the saved key")
	click(t, tt, "Save")
	if kc.Item("vendor.deepseek") != "" {
		t.Fatal("kept a key no app uses")
	}
	if _, ok := other.Offer("deepseek"); ok {
		t.Fatal("metadata kept")
	}
}

// familyOfferShot is the dialog offering Auralink's DeepSeek key (the
// snapshots' remote-dialog-shared), on a fake family Keychain.
func familyOfferShot(a *app, save func(string)) {
	if a.fam == nil {
		return
	}
	kc := actnow.NewFakeKeychain()
	cs := a.fam.Credentials(kc.Keychain())
	au, _ := family.AppByID("auralink")
	other := &credentials.Store{Family: a.fam.Dir(), BundleID: au.BundleID, Keychain: kc.Keychain()}
	if other.Save(context.Background(), "anthropic", "sk-ant-auralink-a1b2", credentials.SaveOptions{}) != nil {
		return
	}
	old := a.famKeys.Load()
	a.famKeys.Store(cs)
	a.openProviderDialog(providers.LLM, "anthropic")
	save("remote-dialog-shared")
	a.rmDlg = providerDialog{}
	_ = other.DeleteForAll(context.Background(), "anthropic")
	a.famKeys.Store(old)
}
