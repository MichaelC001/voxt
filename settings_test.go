package main

// Tests of the settings sections: Overlay style, Shortcuts, General's
// other languages, the dialog's size and its column (Audio: settings_audio_test.go;
// the sidebar's automatic rail: shell_nav_test.go; the menu bar:
// tray_test.go).

import (
	"math"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

func TestOverlayStyleSettings(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/settings/overlay")
	if !has(tt, "Preview") {
		t.Fatalf("section: %q", firstTexts(tt))
	}
	if _, ok := tt.Find("Preview of the overlay"); !ok {
		t.Error("no preview")
	}
	click(t, tt, "Top centre")
	if o := a.settings.Get().Overlay; o.Position != "top" || o.Opacity != settings.DefaultOpacity || o.Margin != 96 || o.ShowTime {
		t.Fatalf("top: %+v", o)
	}
	// A slider's change goes to the overlay and the settings.
	a.settingsForm.ovStyle.opacity = 0.73
	a.overlayEdited()
	if o := a.settings.Get().Overlay; o.Opacity != 0.73 || o.Position != "top" {
		t.Fatalf("opacity: %+v", o)
	}
	// The pill has no buttons; the time shows when it is chosen.
	if _, ok := tt.Find("Stop"); ok {
		t.Error("the preview has a Stop button")
	}
	if has(tt, "0:07") {
		t.Error("the time shows by default")
	}
	if _, ok := tt.Find("Show the recording time"); !ok {
		t.Error("no time switch")
	}
	a.settingsForm.ovStyle.showTime = true
	a.overlayEdited()
	if o := a.settings.Get().Overlay; !o.ShowTime || !has(tt, "0:07") {
		t.Errorf("show time: %+v", o)
	}
	// A change saved elsewhere reaches the form.
	a.updateSettings(func(s *settings.Settings) { s.Overlay.Radius = 8 })
	if a.settingsForm.ovStyle.radius != 8 {
		t.Errorf("form radius %v", a.settingsForm.ovStyle.radius)
	}
}

func TestShortcutsSettings(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/settings/shortcuts")
	for _, s := range []string{"Transcribe", "Translate", "Rewrite", "Paste the last result", "Right ⌥ ⇧", "⌃⌥V"} {
		if !has(tt, s) {
			t.Errorf("no %q: %q", s, firstTexts(tt))
		}
	}
	// A conflict shows on both rows.
	a.updateSettings(func(s *settings.Settings) {
		s.Features.Translate.Hotkeys = []settings.Hotkey{{Keys: "RightOption", Trigger: settings.TriggerTap}}
	})
	if !has(tt, "Transcribe has the same keys and trigger: only one of them works.") || !has(tt, "Translate has the same keys and trigger: only one of them works.") {
		t.Errorf("conflict: %q", tt.Texts())
	}
	a.updateSettings(func(s *settings.Settings) { s.Features.Rewrite.Hotkeys = nil })
	if !has(tt, "None") {
		t.Error("a feature without shortcut")
	}
	click(t, tt, "Paste the last result ⌃⌥V")
	if a.settingsOpen || a.shownPath() != "/transcribe" {
		t.Fatalf("row click: %v %s", a.settingsOpen, a.shownPath())
	}
	a.navigate("/settings/shortcuts")
	click(t, tt, "Translate Right ⌥")
	if a.shownPath() != "/translate" {
		t.Fatalf("Translate row: %s", a.shownPath())
	}
}

// TestShortcutPresets: the preset select shows the one the shortcuts are,
// sets the three features' at once, and offers Custom only when they are
// none.
func TestShortcutPresets(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/settings/shortcuts")
	frames(tt)
	if a.presetSel != "Right ⌥ (default)" {
		t.Fatalf("preset %q", a.presetSel)
	}
	choose := func(id string) {
		t.Helper()
		if opts, _ := presetOptions(hotkey.PresetOf(a.prefs.Features)); slices.Contains(opts, "Custom") {
			t.Errorf("Custom offered for a preset: %q", opts)
		}
		a.choosePreset(id)
		frames(tt)
	}
	choose(hotkey.PresetMouse)
	f := a.settings.Get().Features
	if f.Transcribe.Hotkeys[0].Keys != "MouseMiddle" || f.Rewrite.Hotkeys[0].Trigger != settings.TriggerDouble || f.PasteLast.Keys != settings.DefaultPasteLast {
		t.Fatalf("mouse preset %+v", f)
	}
	if !has(tt, "Mouse middle") {
		t.Errorf("no mouse row: %q", tt.Texts())
	}
	choose(hotkey.PresetFn)
	if f := a.settings.Get().Features; f.Translate.Hotkeys[0].Keys != "Fn+Shift" || hotkey.PresetOf(f) != hotkey.PresetFn {
		t.Fatalf("fn preset %+v", f)
	}
	a.updateSettings(func(s *settings.Settings) { s.Features.Rewrite.Hotkeys[0].Trigger = settings.TriggerLong })
	frames(tt)
	if opts, _ := presetOptions(hotkey.PresetOf(a.prefs.Features)); a.presetSel != "Custom" || !slices.Contains(opts, "Custom") {
		t.Errorf("changed by hand: %q %q", a.presetSel, opts)
	}
}

func TestOtherLanguages(t *testing.T) {
	a, _, tt := newTestApp(t)
	a.navigate("/settings/general")
	click(t, tt, "Add a language")
	if err := tt.ChooseMenuItem(uitext.LangLabel("ja")); err != nil {
		t.Fatal(err)
	}
	frames(tt)
	if err := tt.Click("Add a language"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(tt.Menu(), uitext.LangLabel("en")) || slices.Contains(tt.Menu(), uitext.LangLabel("ja")) {
		t.Errorf("the menu offers the main language or a chosen one: %q", tt.Menu())
	}
	if err := tt.ChooseMenuItem(uitext.LangLabel("zh")); err != nil {
		t.Fatal(err)
	}
	frames(tt)
	if got := a.settings.Get().General.OtherLanguages; !slices.Equal(got, []string{"ja", "zh"}) {
		t.Fatalf("added: %q", got)
	}
	click(t, tt, "Remove "+uitext.LangLabel("ja"))
	if got := a.settings.Get().General.OtherLanguages; !slices.Equal(got, []string{"zh"}) {
		t.Fatalf("removed: %q", got)
	}
	// Choosing it as the main language takes it off the others.
	a.setMainLanguage("zh") // the select's choice
	if got := a.settings.Get().General.MainLanguage; got != "zh" {
		t.Fatalf("main language %q", got)
	}
	if got := a.settings.Get().General.OtherLanguages; len(got) != 0 {
		t.Errorf("main language still among the others: %q", got)
	}
	if got := withoutLanguage([]string{"zh-Hans", "en", "x-unknown"}, "zh"); !slices.Equal(got, []string{"en", "x-unknown"}) {
		t.Errorf("withoutLanguage: %q", got)
	}
	if got := withoutLanguage([]string{"x-unknown", "en"}, "y-unknown"); !slices.Equal(got, []string{"x-unknown", "en"}) {
		t.Errorf("withoutLanguage of an unknown code: %q", got)
	}
}

// TestSettingsDialogSize: 680×442 in the default window, a margin
// around it in the smallest (as before), half the growth in between, settingsMaxW × settingsMaxH in a large one.
func TestSettingsDialogSize(t *testing.T) {
	th := ui.LightTheme()
	for _, c := range []struct{ w, h, dw, dh float32 }{
		{windowWidth, windowHeight, 680, 442},
		{windowMinWidth, windowMinHeight, 612, 387.2},
		{1000, 700, 720, 532},
		{1600, 1000, settingsMaxW, settingsMaxH},
	} {
		dw, dh := settingsDialogSize(th, c.w, c.h)
		if math.Abs(float64(dw-c.dw)) > 0.5 || math.Abs(float64(dh-c.dh)) > 0.5 {
			t.Errorf("%v×%v: %v×%v, want %v×%v", c.w, c.h, dw, dh, c.dw, c.dh)
		}
	}
}

// TestSettingsColumnFits: every section's name, in bold as the chosen one
// shows, fits the settings dialog's column with its icon, in English and
// Chinese, with a little room to spare (the tests' font is wider than the
// Mac's).
func TestSettingsColumnFits(t *testing.T) {
	// The column's padding, the button's, the icon and the gap after it.
	const chrome = 2*sp2 + 2*sp2 + 16 + sp2
	for _, lang := range []string{"en", "zh-Hans"} {
		t.Run(lang, func(t *testing.T) {
			i18n.SetLocale(lang)
			t.Cleanup(func() { i18n.SetLocale("en") })
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Text(c, "probe")
				for _, s := range settingsSections() {
					w, _ := c.MeasureText(0, ui.Span{Text: s.title, Size: fsBody, Weight: 600})
					if room := settingsColumnW - chrome - w; room < 2 {
						t.Errorf("%q is %v wide: %v to spare in the column", s.title, w, room)
					}
				}
			}, 400, 200)
			tt.SetLocale(lang)
			tt.Frame()
		})
	}
}
