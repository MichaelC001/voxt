package main

// Settings → Permissions' Speech Recognition row (permissions.go): only
// while a feature uses Apple Speech, and laid out as the other rows.

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/helper/helpertest"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

const speechTip = "Only Apple Speech needs it."

// permsWithApple is an app whose helper has Apple Speech, refused or
// not asked yet (perm).
func permsWithApple(t *testing.T, perm string) (*app, *helpertest.AppleSim, *ui.Tester) {
	t.Helper()
	a, fs, tt := newTestApp(t)
	sim := helpertest.NewAppleSim()
	sim.SpeechPerm = perm
	withApple(fs, sim)
	return a, sim, tt
}

// readyWithApple gives a downloaded speech and language model, with
// Apple's models after them (as openModels does).
func readyWithApple(a *app) {
	a.apples = apple.New(apple.Config{Local: readyLocal(), Helper: helperCaller{a.host}, Name: uitext.ModelName})
	a.local = a.apples
	a.checkModels()
}

func showPerms(a *app) {
	a.refreshApple()
	a.recheckPermissions()
	a.navigate("/dictionary") // a page that does not name the permissions
	a.navigate("/settings/permissions")
}

func TestSpeechRowHiddenWhenUnused(t *testing.T) {
	a, _, tt := permsWithApple(t, proto.PermDenied)
	readyWithApple(a) // downloaded models: Automatic is one of them
	showPerms(a)
	if !has(tt, "Microphone") {
		t.Fatalf("no Permissions section: %q", tt.Texts())
	}
	if has(tt, "Speech Recognition") || has(tt, speechTip) {
		t.Error("a Speech Recognition row while no feature uses Apple Speech")
	}
}

func TestSpeechRowWhenAutomaticIsApple(t *testing.T) {
	a, _, tt := permsWithApple(t, proto.PermNotDetermined)
	showPerms(a) // nothing downloaded: Automatic is Apple Speech
	if !has(tt, "Speech Recognition") || !has(tt, speechTip) {
		t.Fatalf("no Speech Recognition row: %q", tt.Texts())
	}
	// Downloading a model makes it the automatic one: the row goes.
	readyWithApple(a)
	a.refreshApple()
	frames(tt)
	if has(tt, "Speech Recognition") {
		t.Error("the row stays once Automatic is a downloaded model")
	}
}

func TestSpeechRowWhenAFeaturePicksApple(t *testing.T) {
	for _, f := range []string{"transcribe", "translate", "rewrite"} {
		t.Run(f, func(t *testing.T) {
			a, sim, tt := permsWithApple(t, proto.PermDenied)
			readyWithApple(a)
			a.updateSettings(func(s *settings.Settings) {
				switch f {
				case "translate":
					s.Features.Translate.ASR = apple.SpeechID
				case "rewrite":
					s.Features.Rewrite.ASR = apple.SpeechID
				default:
					s.Features.Transcribe.ASR = apple.SpeechID
				}
			})
			showPerms(a)
			if !has(tt, "Speech Recognition") {
				t.Fatalf("no Speech Recognition row: %q", tt.Texts())
			}
			// Refused while in use: the row stays, with System Settings.
			if n := countText(tt, "Open System Settings"); n != 6 {
				t.Errorf("%d Open System Settings, want 6 (text and label) (Microphone, Accessibility, Speech Recognition)", n)
			}
			// No Apple Speech on this Mac any more: nothing uses it.
			sim.Set(func(s *helpertest.AppleSim) { s.Engine = "" })
			a.refreshApple()
			frames(tt)
			if has(tt, "Speech Recognition") {
				t.Error("a Speech Recognition row on a Mac without Apple Speech")
			}
		})
	}
}

// TestSpeechRowLikeTheOthers: not asked yet, the row has the
// Microphone's controls (Ask, Open System Settings), and its name starts
// where theirs do, at their height, in the default and the smallest
// window, in English and Chinese.
func TestSpeechRowLikeTheOthers(t *testing.T) {
	for _, lang := range []string{"en", "zh-Hans"} {
		for _, size := range [][2]int{{windowWidth, windowHeight}, {windowMinWidth, windowMinHeight}, {1400, 900}} {
			a, _, tt := permsWithApple(t, proto.PermNotDetermined)
			tt.SetSize(size[0], size[1])
			if lang != "en" {
				a.updateSettings(func(s *settings.Settings) { s.General.Language = lang })
			}
			showPerms(a)
			frames(tt)
			mic, ok1 := tt.Find(i18n.T("Microphone"))
			sp, ok2 := tt.Find(i18n.T("Speech Recognition"))
			if !ok1 || !ok2 {
				t.Fatalf("%s %v: rows %v %v in %q", lang, size, ok1, ok2, tt.Texts())
			}
			if sp.X != mic.X || sp.H != mic.H || sp.W <= 0 {
				t.Errorf("%s %v: Speech Recognition's name at %+v, the Microphone's at %+v", lang, size, sp, mic)
			}
			if n := countText(tt, i18n.T("Open System Settings")); n != 6 {
				t.Errorf("%s %v: %d Open System Settings, want 6 (text and label)", lang, size, n)
			}
		}
	}
}

// countText is how many of the last frame's texts are s.
func countText(tt *ui.Tester, s string) int {
	n := 0
	for _, x := range tt.Texts() {
		if x == s {
			n++
		}
	}
	return n
}
