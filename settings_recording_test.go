package main

// Tests of the output and recording switches: the clipboard (General),
// the live text (Overlay style), Escape and the voice end command
// (Shortcuts).

import (
	"testing"

	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
)

func TestKeepResultSwitch(t *testing.T) {
	a, _, tt := newTestApp(t)
	if a.prefs.General.RestoreClipboard {
		t.Fatal("restores by default")
	}
	a.navigate("/settings/general")
	clickDialogSwitch(t, tt, "Keep the result on the clipboard", func() bool { return a.settings.Get().General.RestoreClipboard })
}

func TestLiveTextSwitch(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.sess = session.Snapshot{ID: "s1", State: session.Recording, Partial: "so the meeting", Draft: "x", Selection: "y"}
	if s := a.pillSession(); s.Partial == "" {
		t.Fatal("no live text by default")
	}
	a.navigate("/settings/overlay")
	clickDialogSwitch(t, tt, "Show live text", func() bool { return a.settings.Get().Overlay.HideLiveText })
	if s := a.pillSession(); s.Partial != "" || s.Draft != "" || s.Selection != "" || liveLineText(s) != "" {
		t.Errorf("live text hidden: %+v", s)
	}
	// The other overlay settings keep it.
	a.settingsForm.ovStyle.opacity = 0.7
	a.overlayEdited()
	if !a.settings.Get().Overlay.HideLiveText {
		t.Error("a slider brought the live text back")
	}
}

func TestRecordingSwitches(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/settings/shortcuts")
	if has(tt, "End word") {
		t.Error("the end word shows while the command is off")
	}
	clickDialogSwitch(t, tt, "Escape cancels", func() bool { return a.settings.Get().Features.EscapeOff })
	if !a.hotkeyPlan().Configure(true).PassEscape {
		t.Error("Escape still cancels")
	}
	// The feature pages' shortcut tip no longer says so.
	a.navigate("/transcribe")
	frames(tt)
	if _, ok := tt.Find("Tap: press to start, press again to stop. Double tap: press twice quickly. Long press: record while you hold the keys. Escape cancels."); ok {
		t.Error("the shortcuts' tip still says Escape cancels")
	}
	if _, ok := tt.Find("Tap: press to start, press again to stop. Double tap: press twice quickly. Long press: record while you hold the keys."); !ok {
		t.Error("no shortcuts' tip")
	}
	a.navigate("/settings/shortcuts")
	frames(tt)
	clickDialogSwitch(t, tt, "Voice end command", func() bool { return a.settings.Get().Features.EndCommand.On })
	frames(tt)
	if !has(tt, "End word") || a.settingsForm.endChoice != "over" {
		t.Fatalf("end word %q", a.settingsForm.endChoice)
	}
	// A word of one's own.
	a.settingsForm.endChoice = "Custom…"
	a.settingsForm.endCustom, a.settingsForm.endWord = true, "roger that"
	a.updateSettings(func(s *settings.Settings) { s.Features.EndCommand.Word = "roger  that " })
	if w := session.EndWord(a.settings.Get(), "en"); w != "roger that" || !a.settingsForm.endCustom || a.settingsForm.endWord != "roger that" {
		t.Errorf("custom word %q, form %+v", w, a.settingsForm.endWord)
	}
}
