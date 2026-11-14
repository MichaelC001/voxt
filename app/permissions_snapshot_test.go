package main

// Settings → Permissions with Apple Speech on the Mac: the Speech
// Recognition row only while a feature uses Apple Speech.

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/helper/helpertest"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/settings"
)

func TestSnapshotsPermissions(t *testing.T) {
	runSnapshots(t, "VOXTMINI_SNAPSHOTS", snapshotsPermissions)
}

func snapshotsPermissions(t *testing.T, out string) {
	for _, run := range []struct {
		lang string
		dark bool
	}{{"zh-Hans", true}, {"en", false}} {
		suffix := "light"
		if run.dark {
			suffix = "dark-" + run.lang
		}
		shot := func(name string, setup func(a *app, sim *helpertest.AppleSim)) {
			t.Helper()
			a, fs, tt := newTestApp(t)
			tt.SetSize(windowWidth, windowHeight)
			if run.lang != "en" {
				a.updateSettings(func(s *settings.Settings) { s.General.Language = run.lang })
			}
			tt.SetDark(run.dark)
			tt.SetScale(2)
			a.version = "0.1.0"
			sim := helpertest.NewAppleSim()
			withApple(fs, sim)
			setup(a, sim)
			a.clearNotices()
			a.refreshApple()
			a.recheckPermissions()
			a.navigate("/settings/permissions")
			a.toasts = nil
			frames(tt)
			frames(tt)
			f, err := os.Create(filepath.Join(out, fmt.Sprintf("permissions-%s-%s.png", name, suffix)))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
		// Downloaded models: Apple Speech is on the Mac but unused.
		shot("apple-unused", func(a *app, sim *helpertest.AppleSim) {
			sim.SpeechPerm = proto.PermNotDetermined
			readyWithApple(a)
		})
		// Nothing downloaded: Automatic is Apple Speech.
		shot("apple-auto", func(a *app, sim *helpertest.AppleSim) {
			sim.SpeechPerm = proto.PermNotDetermined
		})
		// Transcribe picks Apple Speech, which macOS refused.
		shot("apple-denied", func(a *app, sim *helpertest.AppleSim) {
			sim.SpeechPerm = proto.PermDenied
			readyWithApple(a)
			a.updateSettings(func(s *settings.Settings) { s.Features.Transcribe.ASR = apple.SpeechID })
		})
	}
}
