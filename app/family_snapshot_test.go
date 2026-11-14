package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestSnapshotsFamily saves the settings a family changes: General with
// another ActNow app installed (the language's scope line), Storage with
// models only an uninstalled app used (their clean-up), English light and
// Chinese dark.
func TestSnapshotsFamily(t *testing.T) { runSnapshots(t, "VOXTMINI_SNAPSHOTS", snapshotsFamily) }

func snapshotsFamily(t *testing.T, out string) {
	w, h := snapshotSize()
	for _, run := range []struct {
		lang string
		dark bool
	}{{"en", false}, {"zh-Hans", true}} {
		a, _, tt := newTestApp(t)
		tt.SetSize(w, h)
		if run.lang != "en" {
			a.updateSettings(func(s *settings.Settings) { s.General.Language = run.lang })
		}
		tt.SetDark(run.dark)
		tt.SetScale(2)
		a.version = "0.1.0"
		suffix := "light"
		if run.dark {
			suffix = "dark"
		}
		if run.lang != "en" {
			suffix += "-" + run.lang
		}
		save := func(name string) {
			t.Helper()
			a.toasts = nil
			frames(tt)
			f, err := os.Create(filepath.Join(out, name+"-"+suffix+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
		orphanFamily(t, a)
		a.navigate("/settings/general")
		save("family-general")
		a.navigate("/settings/storage")
		frames(tt)
		pinStorage(a)
		scrollToText(tt, i18n.T("Model folder"))
		save("family-storage")
		familyCleanupShots(a, tt, save)
	}
}

// orphanFamily installs Dicta beside VoxtMini, and leaves the models
// AudioSmith alone claimed after it was uninstalled.
func orphanFamily(t *testing.T, a *app) {
	t.Helper()
	if _, err := addPeer(t, a, dictaID, nil).Heal(t.Context()); err != nil { // its claims file, as it starts
		t.Fatal(err)
	}
	smith := addPeer(t, a, "dev.hehehai.audiosmith", nil)
	seedModels(t, a, models.ASRAccurate, models.VADSilero)
	peerClaims(t, smith, models.ASRAccurate, models.VADSilero)
	uninstallPeer(t, a, "dev.hehehai.audiosmith")
	a.familyFound(a.fam.Discover())
	a.models.Refresh()
}
