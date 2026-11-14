package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hehehai/VoxtMini/internal/helper/helpertest"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestSnapshotsAppleCards renders Apple's cards on the Models page (the
// speech one asking for Speech Recognition, Apple Intelligence ready),
// light, in English and Chinese.
func TestSnapshotsAppleCards(t *testing.T) {
	runSnapshots(t, "VOXTMINI_SNAPSHOTS", func(t *testing.T, out string) {
		for _, lang := range []string{"en", "zh-Hans"} {
			a, fs, tt := newTestApp(t)
			tt.SetSize(windowWidth, windowHeight)
			if lang != "en" {
				a.updateSettings(func(s *settings.Settings) { s.General.Language = lang })
			}
			tt.SetScale(2)
			a.version = "0.1.0"
			sim := helpertest.NewAppleSim()
			sim.SpeechPerm = proto.PermNotDetermined
			withApple(fs, sim)
			a.recheckPermissions()
			a.clearNotices()
			suffix := "light"
			if lang != "en" {
				suffix += "-" + lang
			}
			for i, path := range []string{modelsPath + "/speech", modelsPath + "/language"} {
				a.navigate("/home")
				a.navigate(path)
				frames(tt)
				scrollToText(tt, i18n.T("Built into macOS"))
				a.toasts = nil
				frames(tt)
				name := []string{"models-apple-speech", "models-apple-intelligence"}[i]
				f, err := os.Create(filepath.Join(out, name+"-"+suffix+".png"))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, tt.Image()); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
		}
	})
}
