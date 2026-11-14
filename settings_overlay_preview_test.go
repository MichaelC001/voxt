package main

import (
	"testing"

	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// The preview is first in the section, its switch shows each state with
// the overlay's own faces, and it follows the settings as they change.
func TestOverlayPreviewStates(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/settings/overlay")
	frames(tt)
	pv, ok := tt.Find("Preview of the overlay")
	pos, ok2 := tt.Find("Position")
	if !ok || !ok2 || pv.Y > pos.Y {
		t.Fatalf("the preview is not above the controls: %v %v", pv, pos)
	}
	said, draft, answer := ovPreviewTexts()

	// Recording: the waveform, and the live text while it shows.
	if _, ok := tt.Find("Input level"); !ok || !has(tt, said) {
		t.Fatalf("recording: %q", firstTexts(tt))
	}
	a.settingsForm.ovStyle.liveText = false
	a.overlayEdited()
	if has(tt, said) {
		t.Error("the live text shows while it is off")
	}
	a.settingsForm.ovStyle.showTime = true
	a.overlayEdited()
	if !has(tt, "0:07") {
		t.Error("no time")
	}
	a.settingsForm.ovStyle.liveText = true
	a.overlayEdited()

	// The place follows Position, live.
	bottom, _ := tt.Find("Input level")
	a.settingsForm.ovStyle.position = 1
	a.overlayEdited()
	frames(tt)
	top, _ := tt.Find("Input level")
	if top.Y >= bottom.Y {
		t.Errorf("top %v, bottom %v", top, bottom)
	}
	if top.Y < pv.Y || top.Y+top.H > pv.Y+pv.H {
		t.Errorf("the face %v is out of the preview %v", top, pv)
	}

	click(t, tt, i18n.X("overlay preview", "Processing"))
	if a.settingsForm.ovStyle.preview != ovPreviewWorking || !has(tt, draft) {
		t.Errorf("working: %d %q", a.settingsForm.ovStyle.preview, firstTexts(tt))
	}
	if _, ok := tt.Find(i18n.T("Cleaning up…")); !ok {
		t.Error("no loader")
	}

	click(t, tt, i18n.X("overlay preview", "Answer"))
	if !has(tt, "AI Answer") || !has(tt, answer) {
		t.Errorf("answer: %q", firstTexts(tt))
	}
	// A picture: its buttons do nothing.
	click(t, tt, "Continue")
	click(t, tt, "Insert")
	if a.answer.open || a.sess.ID != "" || !a.settingsOpen || !has(tt, "AI Answer") {
		t.Error("a click went through the preview")
	}

	click(t, tt, i18n.X("overlay preview", "Error"))
	n := previewNotice()
	if !has(tt, n.text) {
		t.Errorf("notice: %q", firstTexts(tt))
	}
	click(t, tt, n.text+" · "+noticeActionLabel(n.action))
	if a.shownPath() != "/settings/overlay" {
		t.Errorf("the notice's action ran: %s", a.shownPath())
	}
}

// The preview's faces take the settings' look, in Chinese too.
func TestOverlayPreviewChinese(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.updateSettings(func(s *settings.Settings) { s.General.Language = "zh-Hans" })
	a.navigate("/settings/overlay")
	said, _, _ := ovPreviewTexts()
	if !has(tt, "明天下午三点开会，讨论一下发布计划") || said != "明天下午三点开会，讨论一下发布计划" {
		t.Errorf("zh: %q", firstTexts(tt))
	}
	if !has(tt, "处理中") || !has(tt, "回答") || !has(tt, "出错") {
		t.Errorf("tabs: %q", firstTexts(tt))
	}
}
