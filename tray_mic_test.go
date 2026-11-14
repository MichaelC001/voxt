package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/helper/helpertest"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// micMenu is the tray's Microphone submenu.
func micMenu(t *testing.T, a *app) []*mygo.MenuItem {
	t.Helper()
	for _, it := range a.trayItems() {
		if it.Label == "Microphone" {
			return it.Submenu
		}
	}
	t.Fatal("no Microphone submenu")
	return nil
}

func checkedLabels(items []*mygo.MenuItem) (labels []string, checked string) {
	for _, it := range items {
		labels = append(labels, it.Label)
		if it.Checked {
			checked = it.Label
		}
	}
	return labels, checked
}

func TestTrayMicrophones(t *testing.T) {
	a, _, _ := newTestApp(t)
	// The helper became ready: the inputs are listed without Settings.
	labels, checked := checkedLabels(micMenu(t, a))
	if !slices.Equal(labels, []string{"MacBook Air Microphone", "AirPods Pro", "", "Microphone settings…"}) || checked != "MacBook Air Microphone" {
		t.Fatalf("menu %q, checked %q", labels, checked)
	}
	// Choosing one puts it first in the list.
	a.updateSettings(func(s *settings.Settings) { s.Audio.Devices = []string{"USBMic", "BuiltInMicrophoneDevice"} })
	sub := micMenu(t, a)
	sub[1].Click(sub[1], nil)
	if d := a.settings.Get().Audio; !slices.Equal(d.Devices, []string{"AirPodsPro:input", "USBMic", "BuiltInMicrophoneDevice"}) || d.DeviceNames["AirPodsPro:input"] != "AirPods Pro" {
		t.Fatalf("chosen: %+v", d)
	}
	if _, checked := checkedLabels(micMenu(t, a)); checked != "AirPods Pro" {
		t.Errorf("checked %q", checked)
	}
	sub = micMenu(t, a)
	sub[0].Click(sub[0], nil)
	if d := a.settings.Get().Audio.Devices; !slices.Equal(d, []string{"BuiltInMicrophoneDevice", "AirPodsPro:input", "USBMic"}) {
		t.Fatalf("chosen again: %q", d)
	}
	// AirPods unplugged: capture.devices.changed.
	m, _ := proto.NewNotification(proto.MethodDevicesChanged, proto.DevicesResult{Devices: helpertest.Devices()[:1]})
	a.helperNotified(m)
	if labels, _ := checkedLabels(micMenu(t, a)); !slices.Equal(labels, []string{"MacBook Air Microphone", "", "Microphone settings…"}) {
		t.Errorf("unplugged: %q", labels)
	}
	// None at all.
	m, _ = proto.NewNotification(proto.MethodDevicesChanged, proto.DevicesResult{})
	a.helperNotified(m)
	sub = micMenu(t, a)
	if sub[0].Label != "No microphone found" || !sub[0].Disabled {
		t.Errorf("none: %+v", sub[0])
	}
	// Microphone settings… opens Settings → Audio.
	sub[len(sub)-1].Click(sub[len(sub)-1], nil)
	if !a.settingsOpen || a.setRouter.Path() != "/audio" {
		t.Errorf("settings: %v %q", a.settingsOpen, a.setRouter.Path())
	}
}
