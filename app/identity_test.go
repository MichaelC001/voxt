package main

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestReopenAtLogin: the settings moved from the old bundle id say to open
// at login and macOS does not know the new id: VoxtMini registers it,
// once; a failure turns the setting off.
func TestReopenAtLogin(t *testing.T) {
	var calls atomic.Int32 // a.async: at once in tests
	fail := false
	setOpenAtLogin = func(on bool) error {
		calls.Add(1)
		if !on {
			t.Error("asked to turn it off")
		}
		if fail {
			return errors.New("not in a bundle")
		}
		return nil
	}
	t.Cleanup(func() { setOpenAtLogin = mygo.App.SetOpenAtLogin })
	a, _, _ := newTestApp(t)
	a.updateSettings(func(s *settings.Settings) { s.General.LaunchAtLogin = true })

	if a.reopenAtLogin(false) {
		t.Fatal("registered without a move from the old folders")
	}
	a.renamed = true
	if a.reopenAtLogin(true) {
		t.Fatal("registered while macOS knows it")
	}
	if !a.reopenAtLogin(false) {
		t.Fatal("not registered")
	}
	if !a.loginIs {
		t.Fatal("not registered")
	}
	if a.reopenAtLogin(false) || calls.Load() != 1 {
		t.Errorf("registered %d times", calls.Load())
	}

	fail, a.renamed, a.loginIs = true, true, false
	a.reopenAtLogin(false)
	if a.settings.Get().General.LaunchAtLogin || a.loginIs {
		t.Error("the setting stayed on after a failure")
	}
}
