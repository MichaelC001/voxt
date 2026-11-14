package main

import (
	"testing"

	"github.com/hehehai/VoxtMini/internal/helper/proto"
)

func TestTapLine(t *testing.T) {
	for _, c := range []struct {
		st   proto.HotkeyStatus
		want string
	}{
		{proto.HotkeyStatus{Installed: true, Events: 12, MaxCallbackMicros: 21.4},
			"hotkey taps: keyboard (active) events=12 slowest=21.4µs"},
		{proto.HotkeyStatus{Installed: true, Events: 31, MaxCallbackMicros: 412.7, MaxLagMS: 212.6, MissedEvents: 1, DisabledByTimeout: 2,
			Pointer: &proto.HotkeyTapStatus{Installed: true, Events: 9, MaxLagMS: 1840.2}},
			"hotkey taps: keyboard (active) events=31 slowest=412.7µs lag≤212.6ms disabled=2 missed=1; mouse (listen-only) events=9 lag≤1840.2ms"},
		{proto.HotkeyStatus{Installed: true, Pointer: &proto.HotkeyTapStatus{Error: "CGEvent.tapCreate (listen-only, mouse) failed"}},
			"hotkey taps: keyboard (active) events=0 slowest=0.0µs; mouse (listen-only) not installed: CGEvent.tapCreate (listen-only, mouse) failed"},
		{proto.HotkeyStatus{Installed: true, Pointer: &proto.HotkeyTapStatus{Installed: true}, Buttons: &proto.HotkeyTapStatus{Installed: true, Events: 4, MaxLagMS: 0.6}},
			"hotkey taps: keyboard (active) events=0 slowest=0.0µs; mouse (listen-only) events=0; mouse buttons (active) events=4 lag≤0.6ms"},
	} {
		if got := tapLine(c.st); got != c.want {
			t.Errorf("tapLine:\n got %s\nwant %s", got, c.want)
		}
	}
}
