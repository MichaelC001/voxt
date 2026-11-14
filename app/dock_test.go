package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/settings"
)

const (
	regular   = mygo.ActivationPolicyRegular
	accessory = mygo.ActivationPolicyAccessory
)

// TestDockPolicy: on, always in the Dock; off, only while the window
// shows (Voxt's resolvedActivationPolicy).
func TestDockPolicy(t *testing.T) {
	for _, c := range []struct {
		dock, shown bool
		want        mygo.ActivationPolicy
	}{{true, true, regular}, {true, false, regular}, {false, true, regular}, {false, false, accessory}} {
		if got := dockPolicy(c.dock, c.shown); got != c.want {
			t.Errorf("dock %v, window %v: %v, want %v", c.dock, c.shown, got, c.want)
		}
	}
}

// dockRig is an app whose window's visibility the test sets, recording
// the policies applied.
type dockRig struct {
	a     *app
	tt    *ui.Tester
	shown bool
	got   []mygo.ActivationPolicy
}

func newDockRig(t *testing.T, dir string) *dockRig {
	a, _, tt := newTestAppIn(t, dir)
	r := &dockRig{a: a, tt: tt, shown: true}
	a.policy = dockPolicy(a.prefs.General.ShowInDock, true) // as at start, the window open
	a.winShown = func() bool { return r.shown }
	a.setPolicy = func(p mygo.ActivationPolicy) { r.got = append(r.got, p) }
	return r
}

// window shows or hides the window, as its OnShow / OnHide do.
func (r *dockRig) window(shown bool) {
	r.shown = shown
	r.a.applyDock()
}

func (r *dockRig) want(t *testing.T, ps ...mygo.ActivationPolicy) {
	t.Helper()
	if !slices.Equal(r.got, ps) {
		t.Fatalf("policies %v, want %v", r.got, ps)
	}
}

// TestShowInDock: off by default, VoxtMini leaves the Dock when its window
// hides and comes back when it shows; the switch applies at once, is
// saved, and the next start reads it.
func TestShowInDock(t *testing.T) {
	dir := testDir(t)
	r := newDockRig(t, dir)
	a := r.a
	if a.prefs.General.ShowInDock || a.settings.Get().General.ShowInDock {
		t.Fatal("in the Dock by default")
	}
	r.window(false)
	r.want(t, accessory)
	r.window(true)
	r.want(t, accessory, regular)

	a.navigate("/settings/general")
	frames(r.tt)
	clickDialogSwitch(t, r.tt, "Show icon in Dock", func() bool { return a.prefs.General.ShowInDock })
	eventually(t, "the setting saved", func() bool { return a.settings.Get().General.ShowInDock })
	r.window(false) // on: the window hidden keeps the Dock icon
	r.want(t, accessory, regular)
	a.quit()
	a.lib.close()

	// The next start: the setting and the switch are on.
	b := newDockRig(t, dir)
	if !b.a.prefs.General.ShowInDock || !b.a.settingsForm.showInDock {
		t.Fatal("not kept")
	}
	b.window(false)
	b.want(t)
	b.a.updateSettings(func(s *settings.Settings) { s.General.ShowInDock = false })
	eventually(t, "out of the Dock, the window hidden", func() bool { return slices.Equal(b.got, []mygo.ActivationPolicy{accessory}) })
}

// TestMenuBarOrDock: one of the two icons stays on. With the Dock icon
// off (the default), the menu bar switch is locked on, with a note; with
// the Dock icon on, the menu bar icon may go, and the Dock switch is then
// locked. Saved settings with both off come back with the menu bar on.
func TestMenuBarOrDock(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/settings/general")
	frames(tt)
	tt.Scroll(windowWidth/2, windowHeight/2, 0, 400)
	frames(tt)
	menuNote := "Required while the Dock icon is off."
	dockNote := "Required while the menu bar icon is off."
	if !has(tt, menuNote) || has(tt, dockNote) {
		t.Fatalf("notes, the Dock icon off: %q", firstTexts(tt))
	}
	// The locked switch does nothing. Clicks on the switch only: the
	// dialog's scrollbar is at its right edge.
	r, _ := tt.Find("Show in the menu bar")
	dw, _ := settingsDialogSize(ui.LightTheme(), windowWidth, windowHeight)
	right := (windowWidth + dw) / 2
	for x := right - 28; x > right-64; x -= 4 {
		tt.ClickAt(x, r.Y+r.H/2)
	}
	frames(tt)
	if !a.prefs.General.MenuBar {
		t.Fatal("the menu bar icon went with the Dock icon off")
	}

	a.updateSettings(func(s *settings.Settings) { s.General.ShowInDock = true })
	frames(tt)
	clickDialogSwitch(t, tt, "Show in the menu bar", func() bool { return !a.prefs.General.MenuBar })
	eventually(t, "the menu bar icon off", func() bool { return !a.settings.Get().General.MenuBar })
	frames(tt)
	if has(tt, menuNote) || !has(tt, dockNote) {
		t.Fatalf("notes, the menu bar icon off: %q", firstTexts(tt))
	}

	// Both off, as an older or edited document may say: the menu bar
	// comes back on.
	s, err := settings.Migrate([]byte(`{"version":4,"general":{"menuBar":false,"showInDock":false}}`))
	if err != nil || !s.General.MenuBar {
		t.Fatalf("both off: %+v %v", s.General, err)
	}
}

// clickDialogSwitch clicks the switch of a settings row: at the row
// title's height (the page scrolled down), from inside the dialog's
// scrollbar (a click on its track would scroll) inwards until on() turns
// true.
func clickDialogSwitch(t *testing.T, tt *ui.Tester, title string, on func() bool) {
	t.Helper()
	frames(tt)                                       // laid out: the scroll finds its scroller
	tt.Scroll(windowWidth/2, windowHeight/2, 0, 400) // the row to the dialog's middle
	frames(tt)
	r, ok := tt.Find(title)
	if !ok {
		t.Fatalf("no %q: %q", title, firstTexts(tt))
	}
	dw, _ := settingsDialogSize(ui.LightTheme(), windowWidth, windowHeight)
	right := (windowWidth + dw) / 2
	for x := right - 16; x > right-120; x -= 4 {
		tt.ClickAt(x, r.Y+r.H/2)
		frames(tt)
		if on() {
			return
		}
	}
	t.Fatalf("no switch beside %q at %+v", title, r)
}

// TestKeepRunningWithDock: with the menu bar icon off and the Dock icon
// on, "Keep running when the window closes" can be turned on, and closing
// the window then only hides it: the Dock icon stays (regular) and a
// click on it shows the window again (OnActivate). Opened at login, the
// window waits hidden there too.
func TestKeepRunningWithDock(t *testing.T) {
	r := newDockRig(t, testDir(t))
	a, tt := r.a, r.tt
	a.updateSettings(func(s *settings.Settings) { s.General.ShowInDock, s.General.CloseToTray = true, false })
	a.updateSettings(func(s *settings.Settings) { s.General.MenuBar = false })
	eventually(t, "saved", func() bool { g := a.settings.Get().General; return g.ShowInDock && !g.MenuBar })
	a.closeTray() // as applyTray does in the app
	a.loadSettingsForm()
	if a.keepsRunning() {
		t.Fatal("keeps running with the setting off")
	}
	a.navigate("/settings/general")
	frames(tt)
	clickDialogSwitch(t, tt, "Keep running when the window closes", func() bool { return a.prefs.General.CloseToTray })
	if !a.keepsRunning() {
		t.Fatal("closing the window would quit with the Dock icon on")
	}
	r.got = nil
	r.window(false) // the close, prevented, hides the window
	r.want(t)       // still regular: the Dock icon stays
	a.quitting = true
	if a.keepsRunning() {
		t.Error("keeps running while quitting")
	}
	a.quitting = false
	// Neither icon (only an older document; normalize repairs it): quits.
	a.prefs.General.ShowInDock = false
	if a.keepsRunning() {
		t.Error("keeps running with no icon to come back from")
	}
	if !startHidden(true, settings.General{ShowInDock: true, CloseToTray: true}) {
		t.Error("opened at login with the Dock icon: the window should wait hidden")
	}
}
