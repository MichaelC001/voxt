package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestOverlayStyleShots renders Settings → Overlay style alone, light and
// dark, English and Chinese, each of the preview's states: quicker than
// TestSnapshots when only this section changed.
//
//	VOXTMINI_OVSHOTS=/tmp/ov go test -run TestOverlayStyleShots .
func TestOverlayStyleShots(t *testing.T) {
	runSnapshots(t, "VOXTMINI_OVSHOTS", func(t *testing.T, out string) {
		w, h := snapshotSize()
		for _, lang := range []string{"en", "zh-Hans"} {
			for _, dark := range []bool{false, true} {
				a, _, tt := newTestApp(t)
				tt.SetSize(w, h)
				if lang != "en" {
					a.updateSettings(func(s *settings.Settings) { s.General.Language = lang })
				}
				tt.SetDark(dark)
				tt.SetScale(2)
				suffix := "light"
				if dark {
					suffix = "dark"
				}
				if lang != "en" {
					suffix += "-" + lang
				}
				save := func(name string) {
					t.Helper()
					frames(tt)
					f, err := os.Create(filepath.Join(out, name+"-"+suffix+".png"))
					if err != nil {
						t.Fatal(err)
					}
					defer f.Close()
					if err := png.Encode(f, tt.Image()); err != nil {
						t.Fatal(err)
					}
				}
				a.navigate("/settings/overlay")
				for i := range ovPreviewStates {
					a.settingsForm.ovStyle.preview = i
					save("overlay-" + ovPreviewNames[i])
				}
				a.settingsForm.ovStyle.preview = 0
				// Top, with the time, no live text.
				a.updateSettings(func(s *settings.Settings) {
					s.Overlay.Position, s.Overlay.ShowTime, s.Overlay.HideLiveText = "top", true, true
				})
				save("overlay-top-time")
			}
		}
	})
}
