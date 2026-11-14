package main

// VoxtMini's own identity (docs/behavior.md 9.21): until 0.1.0-beta.3 it
// ran as dev.hehehai.voxtnew with folders named VoxtNew, as Voxt 2 does.
// Its folders are now …/VoxtMini: the first start moves VoxtMini's
// entries over (internal/datadir, AdoptOldName) before anything opens
// them, and turns opening at login on again for the new bundle id; the
// family's files follow in family.go (actnow.AdoptOld). The Keychain
// items (service "VoxtMini") do not depend on the bundle id.

import (
	"context"
	"log"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/datadir"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// renameResult is what the first start under the new name moved.
type renameResult struct {
	got datadir.Adopted
	err error
}

// adoptOldName moves VoxtMini's entries from the folders of its old
// name to now's (a no-op once moved, or on a Mac without them). It runs
// before the logs open: tellRenamed logs it.
func adoptOldName(now datadir.Dirs) renameResult {
	got, err := datadir.AdoptOldName(context.Background(), now.Renamed(datadir.OldName), now)
	return renameResult{got, err}
}

// tellRenamed logs what adoptOldName did and remembers that the data
// came from the old bundle id (syncOpenAtLogin).
func (a *app) tellRenamed(r renameResult) {
	if r.err != nil {
		log.Printf("data: moving from the %s folders: %v", datadir.OldName, r.err)
	}
	if len(r.got.Moved) == 0 {
		return
	}
	a.renamed = true
	log.Printf("data: moved %d entries from the %s folders (%d paths in the library updated): %v", len(r.got.Moved), datadir.OldName, r.got.Paths, r.got.Moved)
	if len(r.got.Left) > 0 {
		log.Printf("data: left in the %s folders, not VoxtMini's: %v", datadir.OldName, r.got.Left)
	}
}

// setOpenAtLogin is MyGo's (tests replace it).
var setOpenAtLogin = mygo.App.SetOpenAtLogin

// reopenAtLogin registers the new bundle id to open at login, when the
// settings that came from the old one say so and macOS does not know the
// new one yet (main thread; the call runs on a goroutine, a.async).
func (a *app) reopenAtLogin(on bool) bool {
	if !a.renamed || on || !a.prefs.General.LaunchAtLogin {
		return false
	}
	a.renamed = false
	a.async(func() {
		err := setOpenAtLogin(true)
		a.post(func() {
			if err != nil {
				log.Printf("login: opening the renamed app at login: %v", err)
				a.updateSettings(func(s *settings.Settings) { s.General.LaunchAtLogin = false })
				return
			}
			a.loginIs = true
		})
	})
	return true
}
