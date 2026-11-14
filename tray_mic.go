package main

// The menu bar's Microphone submenu: the inputs connected now, the one a
// recording would use checked (deviceInUse); choosing one puts it first
// in Settings → Audio's list, so it records from then on. The list stays
// the one setting: no "current device" beside it (behavior.md 9.1).
// The inputs are those capture.devices listed when the helper became
// ready, and capture.devices.changed keeps them current.

import (
	"maps"
	"slices"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// trayMicItem is the Microphone submenu.
func (a *app) trayMicItem(act func(func()) func(*mygo.MenuItem, *mygo.Window)) *mygo.MenuItem {
	devs := a.audio.devices
	inUse := deviceInUse(a.prefs.Audio.Devices, devs)
	var sub []*mygo.MenuItem
	for _, d := range devs {
		sub = append(sub, &mygo.MenuItem{Label: d.Name, Type: mygo.MenuItemRadio, Checked: d.ID == inUse, Click: act(func() { a.chooseDevice(d) })})
	}
	if len(sub) == 0 {
		sub = append(sub, &mygo.MenuItem{Label: i18n.T("No microphone found"), Disabled: true})
	}
	sub = append(sub, mygo.Separator(), &mygo.MenuItem{Label: i18n.T("Microphone settings…"), Click: act(func() {
		a.showMainWindow()
		a.navigate("/settings/audio")
	})})
	return &mygo.MenuItem{Label: i18n.T("Microphone"), Submenu: sub}
}

// chooseDevice puts input d first in the list (adding it when it is not
// there), so that recordings use it while it is connected.
func (a *app) chooseDevice(d proto.CaptureDevice) {
	a.updateSettings(func(s *settings.Settings) {
		list := slices.DeleteFunc(slices.Clone(s.Audio.Devices), func(x string) bool { return x == d.ID })
		s.Audio.Devices = append([]string{d.ID}, list...)
		m := maps.Clone(s.Audio.DeviceNames)
		if m == nil {
			m = map[string]string{}
		}
		m[d.ID] = d.Name
		s.Audio.DeviceNames = m
	})
}

// devicesListed takes the inputs capture.devices or
// capture.devices.changed gave.
func (a *app) devicesListed(devs []proto.CaptureDevice) {
	a.audio.devices, a.audio.read, a.audio.err = devs, true, ""
	a.rememberDeviceNames(devs)
	a.buildTrayMenu()
}
