package main

// The Dock icon (Settings → General → Show icon in Dock, off by default),
// as Voxt's AppBehaviorController: on, VoxtMini is a regular app, always
// in the Dock; off, it is regular only while the main window shows (in the
// Dock and in ⌘Tab, with its menus), and an accessory, in the menu bar
// only, once the window is closed into the menu bar or hidden. At least
// one of the menu bar icon and the Dock icon stays on (settings.normalize).
// Clicking the Dock icon shows the main window (connectWindowLife,
// tray.go).

import "github.com/egoist/mygo"

// dockPolicy is the activation policy for the setting and the main
// window (Voxt's resolvedActivationPolicy).
func dockPolicy(showInDock, windowShown bool) mygo.ActivationPolicy {
	if showInDock || windowShown {
		return mygo.ActivationPolicyRegular
	}
	return mygo.ActivationPolicyAccessory
}

// startDock sets the policy the app starts with; main calls it before
// App.Run, before there is a window: an accessory, unless the setting
// wants the Dock. openWindow then applies the window's (applyDock), so
// a VoxtMini opened at login into the menu bar never shows in the Dock.
func (a *app) startDock() {
	a.policy = dockPolicy(a.prefs.General.ShowInDock, false)
	mygo.App.SetActivationPolicy(a.policy)
}

// watchDock follows the main window: shown, VoxtMini comes into the Dock;
// hidden (closed into the menu bar), it leaves it, as the setting says.
func (a *app) watchDock() {
	if a.win == nil {
		return
	}
	a.win.OnShow(func() { a.post(a.applyDock) })
	a.win.OnHide(func() { a.post(a.applyDock) })
	a.applyDock()
	a.win.OnShow(func() { a.post(a.windowBack) })
	a.win.OnHide(func() { a.post(a.windowAway) })
}

// windowAway runs when the main window hides (closed into the menu bar):
// what only a shown page needs stops or goes. Its memTick stops itself.
func (a *app) windowAway() {
	a.winAway = true
	a.logsOnScreen.Store(false) // shell.go: new lines redraw nothing
	a.dropIcons()               // appicons.go
}

// windowBack runs when the main window shows again: the Local models
// page measures again (memTick stopped while hidden); the post itself
// redraws, with the lines that came meanwhile.
func (a *app) windowBack() {
	a.winAway = false
	if a.currentPage() == modelsPath[1:] && !a.memPolling {
		a.modelsShown() // models_memory.go
	}
}

// windowShown is whether the main window shows (tests: the fake's).
func (a *app) windowShown() bool {
	if a.winShown != nil {
		return a.winShown()
	}
	return a.win != nil && a.win.IsVisible()
}

// applyDock sets the policy for the setting and the window, when it
// changed. Becoming regular, MyGo activates the app.
func (a *app) applyDock() {
	p := dockPolicy(a.prefs.General.ShowInDock, a.windowShown())
	if p == a.policy {
		return
	}
	a.policy = p
	if a.setPolicy != nil { // tests
		a.setPolicy(p)
		return
	}
	if a.hasMenu {
		mygo.App.SetActivationPolicy(p)
	}
}
