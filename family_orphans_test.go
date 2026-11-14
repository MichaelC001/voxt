package main

import (
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/actnow"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
	"github.com/hehehai/actnowkit/registry"
)

// uninstallPeer removes a peer's app (addPeer) as the user would: its
// manifest and claims stay in the family directory.
func uninstallPeer(t *testing.T, a *app, bundleID string) {
	t.Helper()
	m, err := registry.Read(a.fam.Dir(), bundleID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(m.Path); err != nil {
		t.Fatal(err)
	}
}

// familyCleanupShots saves the clean-up dialog (TestSnapshotsFamily).
func familyCleanupShots(a *app, tt *ui.Tester, save func(string)) {
	if a.stReport == nil || len(a.stReport.orphans) == 0 {
		return
	}
	a.openOrphans()
	frames(tt)
	save("family-storage-cleanup")
	a.orphanOpen = false
}

// TestStorageOrphans: the models only an uninstalled app claimed show
// under the model folder with their size; Clean up lists them, all
// picked; unpicking one keeps it; what VoxtMini claims never shows.
func TestStorageOrphans(t *testing.T) {
	a, _, tt := newTestApp(t)
	tt.SetSize(windowWidth, 1600)
	smith := addPeer(t, a, "dev.hehehai.audiosmith", nil)
	seedModels(t, a, models.ASRAccurate, models.VADSilero, models.ASRFast)
	peerClaims(t, smith, models.ASRAccurate, models.VADSilero, models.ASRFast)
	a.models.Use(t.Context(), models.ASRFast) // VoxtMini uses this one
	a.navigate("/settings/storage")
	frames(tt)
	if has(tt, "Left by uninstalled apps") {
		t.Fatal("AudioSmith is installed: nothing is left")
	}

	uninstallPeer(t, a, "dev.hehehai.audiosmith")
	a.familyFound(a.fam.Discover())
	a.stReport = nil
	if !has(tt, "Left by uninstalled apps") || !has(tt, "Clean up") {
		t.Fatalf("no orphans row: %q", tt.Texts())
	}
	if n := len(a.stReport.orphans); n != 2 {
		t.Fatalf("orphans %+v", a.stReport.orphans)
	}
	click(t, tt, "Clean up")
	asr := a.stReport.orphans[0]
	if !a.orphanOpen || !has(tt, "Clean up models") || !has(tt, asr.Name) || !has(tt, "AudioSmith") {
		t.Fatalf("dialog: %v %q", a.orphanOpen, tt.Texts())
	}
	vad := a.stReport.orphans[1]
	a.orphanPick[vad.Ref] = false // keep the smaller one
	_, bytes := a.pickedOrphans()
	click(t, tt, i18n.T("Delete (%s)", uitext.HumanBytes(bytes)))
	frames(tt) // measured again
	if len(a.stReport.orphans) != 1 || a.stReport.orphans[0].Ref != vad.Ref {
		t.Fatalf("left %+v", a.stReport.orphans)
	}
	if a.models.Installed(models.ASRAccurate) || !a.models.Installed(models.VADSilero) || !a.models.Installed(models.ASRFast) {
		t.Errorf("installed: accurate %v vad %v fast %v", a.models.Installed(models.ASRAccurate), a.models.Installed(models.VADSilero), a.models.Installed(models.ASRFast))
	}
}

// TestLanguageFamilyLine: with another ActNow app installed, the app
// language says it applies to all of them; "Use a different setting for
// this app" keeps VoxtMini's, and the family's value no longer changes it;
// following again takes the family's at once.
func TestLanguageFamilyLine(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.famStarted = true
	a.navigate("/settings/general")
	if has(tt, "Applies to all ActNow apps") {
		t.Fatal("no other ActNow app: no scope line")
	}
	addPeer(t, a, dictaID, nil)
	a.familyFound(a.fam.Discover())
	if !has(tt, "Applies to all ActNow apps") {
		t.Fatalf("no scope line: %q", tt.Texts())
	}
	// A change while following goes to the family.
	a.updateSettings(func(s *settings.Settings) { s.General.Language = "zh-Hans" })
	if got := familyValue(t, a, actnow.KeyLanguage); got != "zh-Hans" {
		t.Fatalf("family's language %q", got)
	}

	click(t, tt, i18n.T("Use a different setting for this app"))
	if a.prefs.Family.Follows(actnow.KeyLanguage) || a.prefs.General.Language != "zh-Hans" || !has(tt, i18n.T("Only VoxtMini uses this setting.")) {
		t.Fatalf("own: %+v %q", a.prefs.Family, a.prefs.General.Language)
	}
	a.updateSettings(func(s *settings.Settings) { s.General.Language = "en" })
	frames(tt)
	if got := familyValue(t, a, actnow.KeyLanguage); got != "zh-Hans" {
		t.Errorf("family's language %q after VoxtMini's own changed", got)
	}

	click(t, tt, "Use the same setting as the other ActNow apps")
	if !a.prefs.Family.Follows(actnow.KeyLanguage) || a.prefs.General.Language != "zh-Hans" {
		t.Errorf("follow again: %+v %q", a.prefs.Family, a.prefs.General.Language)
	}
}

// TestLanguageFamilyConflict: the family's other language shows once as a
// notice: "Use theirs" takes it, × keeps VoxtMini's.
func TestLanguageFamilyConflict(t *testing.T) {
	for _, theirs := range []bool{true, false} {
		a, _, tt := newTestApp(t)
		readyModels(a) // the sidebar's notice has a Dismiss too
		a.updateSettings(func(s *settings.Settings) {
			s.Family.Follow = map[string]bool{actnow.KeyLanguage: false}
			s.Family.Conflicts = []string{actnow.KeyLanguage}
		})
		a.navigate("/settings/general")
		if !has(tt, "Other ActNow apps use a different language.") {
			t.Fatalf("no notice: %q", tt.Texts())
		}
		if theirs {
			click(t, tt, "Use theirs")
			if !a.prefs.Family.Follows(actnow.KeyLanguage) {
				t.Error("Use theirs: not following")
			}
		} else {
			click(t, tt, "Dismiss")
			if a.prefs.Family.Follows(actnow.KeyLanguage) {
				t.Error("×: following")
			}
		}
		if len(a.prefs.Family.Conflicts) != 0 || has(tt, "Other ActNow apps use a different language.") {
			t.Errorf("theirs %v: notice stays: %v", theirs, a.prefs.Family.Conflicts)
		}
	}
}
