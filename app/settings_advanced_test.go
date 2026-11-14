package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestOverlaySwitchesVisible: in the window's default size, Overlay
// style's two switches show inside the dialog without scrolling, under
// the preview and the position.
func TestOverlaySwitchesVisible(t *testing.T) {
	a, _, tt := newTestApp(t)
	tt.SetSize(windowWidth, windowHeight)
	a.navigate("/settings/overlay")
	frames(tt)
	_, dh := settingsDialogSize(ui.LightTheme(), windowWidth, windowHeight)
	bottom := (float32(windowHeight) + dh) / 2
	pos, _ := tt.Find("Position")
	for _, s := range []string{"Show the recording time", "Show live text"} {
		r, ok := tt.Find(s)
		if !ok {
			t.Fatalf("no %q", s)
		}
		if r.Y < pos.Y || r.Y+r.H > bottom {
			t.Errorf("%q at %+v: not between the position (%v) and the dialog's bottom (%v)", s, r, pos.Y, bottom)
		}
	}
	if r, _ := tt.Find("Distance from the edge"); r.Y < pos.Y {
		t.Error("the sliders come before the position")
	}
}

// TestAdvancedFolds: Audio's and Appearance's less used settings wait in
// a closed Advanced fold whose header says what they are; opened, they
// work, and the fold stays open while VoxtMini runs (the dialog closed
// and opened again), not in the settings.
func TestAdvancedFolds(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/settings/audio")
	if hasExact(tt, "Skip silence") || hasExact(tt, "Mute system audio while recording") {
		t.Fatal("the audio's advanced settings show while folded")
	}
	if !hasExact(tt, "Skip silence: On · Optimize sound: On · Mute while recording: On") {
		t.Fatalf("no summary: %q", firstTexts(tt))
	}
	if !hasExact(tt, "Play sounds") {
		t.Error("Play sounds went into the fold")
	}
	clickFold(t, tt)
	if !hasExact(tt, "Skip silence") || !hasExact(tt, "Optimize sound") || !hasExact(tt, "Mute system audio while recording") {
		t.Fatalf("opened: %q", firstTexts(tt))
	}
	clickDialogSwitch(t, tt, "Optimize sound", func() bool { return a.settings.Get().Audio.Clean.Off })
	clickFold(t, tt)
	if !hasExact(tt, "Skip silence: On · Optimize sound: Off · Mute while recording: On") {
		t.Errorf("summary after a change: %q", firstTexts(tt))
	}
	clickFold(t, tt)

	a.navigate("/settings/appearance")
	if hasExact(tt, "Reduce motion") || hasExact(tt, "Icons only") {
		t.Fatal("the appearance's advanced settings show while folded")
	}
	if !hasExact(tt, "Theme") || !hasExact(tt, "Sidebar: Automatic · Reduce motion: Off") {
		t.Fatalf("appearance: %q", firstTexts(tt))
	}
	clickFold(t, tt)
	clickDialogSwitch(t, tt, "Reduce motion", func() bool { return a.settings.Get().Appearance.ReduceMotion })

	// Closed and opened again: the folds as they were.
	a.navigate("/")
	frames(tt)
	a.navigate("/settings/audio")
	if !hasExact(tt, "Skip silence") {
		t.Error("the audio's fold closed")
	}
	a.navigate("/settings/appearance")
	if !hasExact(tt, "Reduce motion") {
		t.Error("the appearance's fold closed")
	}
}

// clickFold scrolls the dialog to its Advanced fold and opens or closes
// it.
func clickFold(t *testing.T, tt *ui.Tester) {
	t.Helper()
	frames(tt)
	if r, ok := tt.Find("Advanced"); ok {
		tt.Scroll(windowWidth*0.6, windowHeight*0.5, 0, r.Y-windowHeight*0.3)
	}
	click(t, tt, "Advanced")
}
