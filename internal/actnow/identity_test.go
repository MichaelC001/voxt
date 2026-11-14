package actnow

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/conformance"
	"github.com/hehehai/actnowkit/credentials"
	"github.com/hehehai/actnowkit/family"
	fammodels "github.com/hehehai/actnowkit/models"
	"github.com/hehehai/actnowkit/registry"
	famruntime "github.com/hehehai/actnowkit/runtime"
)

func catalogue(id string) bool { return id == "asr-fast" || id == "llm-balanced" }

// oldVoxtMini is what VoxtMini left under its old bundle id: a claim on
// an installed repo (downloaded) and one on a repo gone since, its
// manifest (named name), a shared key it added and one it uses, and the
// runtime file of a process long gone.
func oldVoxtMini(t *testing.T, fx *conformance.Fixture, name string) (installed fammodels.Ref) {
	t.Helper()
	ctx := context.Background()
	repo := conformance.TestRepo("mlx-community/Qwen3-ASR-0.6B-4bit", "313d8501", 64)
	fx.Install(repo)
	old := fx.Peer(OldBundleID)
	if err := old.Claim(ctx, fammodels.Claim{Repo: repo.Ref.Repo, Revision: repo.Ref.Revision, Model: "asr-fast", Origin: fammodels.OriginDownloaded}); err != nil {
		t.Fatal(err)
	}
	if name != "" {
		m := registry.Manifest{App: KitAppID, BundleID: OldBundleID, Name: name, Version: "0.1.0-beta.2",
			Path: "/Applications/VoxtMini.app", Capabilities: []string{}}
		if err := registry.Write(fx.Family, m); err != nil {
			t.Fatal(err)
		}
	}
	kc := NewFakeKeychain()
	mine := &credentials.Store{Family: fx.Family, BundleID: OldBundleID, Keychain: kc.Keychain()}
	if err := mine.Save(ctx, "deepseek", "sk-old-0001", credentials.SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	other := &credentials.Store{Family: fx.Family, BundleID: "dev.hehehai.auralink", Keychain: kc.Keychain()}
	if err := other.Save(ctx, "openai", "sk-aura-0001", credentials.SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := mine.Use(ctx, "openai"); err != nil {
		t.Fatal(err)
	}
	if err := famruntime.Write(fx.Family, famruntime.File{BundleID: OldBundleID, PID: 1 << 30}); err != nil {
		t.Fatal(err)
	}
	return repo.Ref
}

// TestAdoptOld: VoxtMini's old claims, keys and manifest pass to the new
// id; the model stays; a second start does nothing.
func TestAdoptOld(t *testing.T) {
	fx := conformance.New(t)
	ctx := context.Background()
	ref := oldVoxtMini(t, fx, "VoxtMini")
	f := open(t, fx, settings.Defaults())
	got, err := f.AdoptOld(ctx, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	if got.Claims != 1 || got.Keys != 2 || !got.Manifest {
		t.Errorf("adopted %+v", got)
	}
	c, ok := fx.Claims(BundleID).Find(ref)
	if !ok || c.Origin != fammodels.OriginDownloaded || c.Model != "asr-fast" {
		t.Errorf("the new claim: %+v %v", c, ok)
	}
	if fx.Claims(BundleID).App != AppID {
		t.Errorf("claims of app %q", fx.Claims(BundleID).App)
	}
	if !fx.Installed(ref) {
		t.Error("the model is gone")
	}
	for _, p := range []string{fx.Family.ClaimsFile(OldBundleID), fx.Family.AppFile(OldBundleID), fx.Family.RuntimeFile(OldBundleID)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s is still there", p)
		}
	}
	file, _, _ := credentials.Read(fx.Family)
	if e := file.Vendors["deepseek"]; e.AddedBy != BundleID || !slices.Equal(e.Users, []string{BundleID}) {
		t.Errorf("deepseek: %+v", e)
	}
	if e := file.Vendors["openai"]; e.AddedBy != "dev.hehehai.auralink" || !slices.Equal(e.Users, []string{"dev.hehehai.auralink", BundleID}) {
		t.Errorf("openai: %+v", e)
	}
	if again, err := f.AdoptOld(ctx, catalogue); err != nil || again.Any() {
		t.Errorf("again: %+v %v", again, err)
	}
}

// TestAdoptOldNotOurs: the old id's files are another app's (Voxt 2: a
// manifest of another name, or claims on models VoxtMini does not have):
// nothing changes.
func TestAdoptOldNotOurs(t *testing.T) {
	fx := conformance.New(t)
	ctx := context.Background()
	oldVoxtMini(t, fx, "Voxt")
	f := open(t, fx, settings.Defaults())
	if got, err := f.AdoptOld(ctx, catalogue); err != nil || got.Any() {
		t.Fatalf("adopted %+v %v", got, err)
	}
	if _, err := os.Stat(fx.Family.ClaimsFile(BundleID)); !os.IsNotExist(err) {
		t.Error("claims under the new id")
	}
	if len(fx.Claims(OldBundleID).Claims) != 1 {
		t.Error("the old claims changed")
	}

	// Without a manifest, claims on a model VoxtMini's catalogue lacks.
	fx = conformance.New(t)
	oldVoxtMini(t, fx, "")
	f = open(t, fx, settings.Defaults())
	if got, _ := f.AdoptOld(ctx, func(string) bool { return false }); got.Any() {
		t.Errorf("adopted %+v", got)
	}
	// With only VoxtMini's models: its claims (the keys need a manifest).
	if got, err := f.AdoptOld(ctx, catalogue); err != nil || got.Claims != 1 || got.Keys != 0 {
		t.Errorf("adopted %+v %v", got, err)
	}
}

func TestOwnBundleIDs(t *testing.T) {
	for id, want := range map[string]family.Variant{
		"dev.hehehai.voxtmini": family.Release, "DEV.hehehai.VoxtMini.dev": family.Dev,
		"dev.hehehai.voxtmini.updtest": family.Test,
		"dev.hehehai.voxtmini.helper":  "", "dev.hehehai.voxtmini.llm": "",
		"dev.hehehai.voxtnew": "", "dev.hehehai.voxtminix": "",
	} {
		if v, _ := ownVariant(id); v != want {
			t.Errorf("%s: %q, want %q", id, v, want)
		}
	}
	if n := AppName("dev.hehehai.voxtmini.dev"); n != "VoxtMini (dev)" {
		t.Errorf("AppName = %q", n)
	}
	if !IsSelf(registry.Found{BundleID: "dev.hehehai.voxtmini.betatest"}) || IsSelf(registry.Found{BundleID: OldBundleID}) {
		t.Error("IsSelf")
	}
}
