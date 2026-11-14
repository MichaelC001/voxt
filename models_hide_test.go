package main

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestModelsFilter: the switch and the filter chips: local models first,
// then the remote services; Local and Remote leave out the others.
func TestModelsFilter(t *testing.T) {
	a, _, tt := newTestApp(t)
	tt.SetSize(windowWidth, 6000) // every card on screen
	a.navigate(modelsPath)
	if !has(tt, "Whisper small") || !has(tt, "OpenAI Transcribe") || has(tt, "DeepSeek") {
		t.Fatalf("speech, all: %q", firstTexts(tt))
	}
	click(t, tt, "Remote 7") // Gemini Live since #29
	if a.mdFilter != filterRemote || has(tt, "Whisper small") || !has(tt, "OpenAI Transcribe") || has(tt, "Advanced") {
		t.Errorf("remote: %q", firstTexts(tt))
	}
	click(t, tt, "Local 6")
	if has(tt, "OpenAI Transcribe") || !has(tt, "Whisper small") || !has(tt, "Advanced") {
		t.Errorf("local: %q", firstTexts(tt))
	}
	click(t, tt, "Language models")
	if !has(tt, "LFM2 1.2B · 4-bit") || has(tt, "Whisper small") || has(tt, "DeepSeek") {
		t.Errorf("language, local: %q", firstTexts(tt))
	}
	click(t, tt, "All 38")
	if !has(tt, "DeepSeek") {
		t.Errorf("language, all: %q", firstTexts(tt))
	}
	// The order: recommended, more models, remote services.
	texts := tt.Texts()
	rec, more, remote := slices.Index(texts, "Recommended"), slices.Index(texts, "More models"), slices.Index(texts, "Remote services")
	if !(rec >= 0 && rec < more && more < remote) {
		t.Errorf("sections at %d %d %d", rec, more, remote)
	}
}

// TestModelsRoutes: /remote and the Models page's sub-paths open it on a
// tab or filter, from the app and from a deep link.
func TestModelsRoutes(t *testing.T) {
	a, _, _ := newTestApp(t)
	for _, c := range []struct {
		path        string
		tab, filter int
		link        bool
	}{
		{"/models/language", 1, filterAll, false},
		{"/remote", 1, filterRemote, false},
		{"/models/speech", 0, filterRemote, false},
		{"/models/local", 0, filterLocal, false},
		{"voxtnew://remote", 0, filterRemote, true},
		{"voxtnew://models/language", 1, filterRemote, true},
	} {
		if c.link {
			a.openURL(c.path)
		} else {
			a.navigate(c.path)
		}
		if a.router.Path() != modelsPath || a.mdTab != c.tab || a.mdFilter != c.filter {
			t.Errorf("%s: %s, tab %d, filter %d", c.path, a.router.Path(), a.mdTab, a.mdFilter)
		}
	}
	a.toasts = nil
	a.openURL("voxtnew://models/nope")
	if len(a.toasts) != 1 {
		t.Errorf("an unknown sub-path: %q", a.toasts)
	}
}

// TestHideModels: Hide leaves a model out of the page and of the
// features' selects, but not a feature's current choice, nor Automatic;
// Show hidden brings it back, Show un-hides it; nothing is deleted.
func TestHideModels(t *testing.T) {
	a, _, tt := newTestApp(t)
	tt.SetSize(windowWidth, 6000)
	seedModels(t, a, models.ASRAccurate, models.ASRFast, models.VADSilero)
	a.navigate(modelsPath)
	chooseMenu(t, tt, "More actions for Fast speech recognition", "Hide")
	if !a.prefs.Models.IsHidden(models.ASRFast) || !slices.Equal(a.settings.Get().Models.Hidden, []string{models.ASRFast}) {
		t.Fatalf("hidden %v", a.settings.Get().Models.Hidden)
	}
	if hasExact(tt, "Fast speech recognition") || !has(tt, "Show hidden 1") || !a.models.Installed(models.ASRFast) {
		t.Errorf("after Hide: %q", firstTexts(tt))
	}
	if !has(tt, "Fast speech recognition is hidden. Show hidden on the Models page brings it back.") {
		t.Errorf("no toast: %q", tt.Texts())
	}
	// The features' select offers it no more; Automatic is unchanged.
	if m, ok := a.featureModel(local.ASR, hotkey.Transcribe); !ok || m.ID != models.ASRAccurate {
		t.Errorf("automatic %+v", m)
	}
	a.navigate("/transcribe")
	click(t, tt, "Speech model for Transcribe")
	if hasExact(tt, "Fast speech recognition") {
		t.Error("the select offers a hidden model")
	}
	tt.Key(0, ui.KeyEscape)
	// A feature that chose it keeps it, and its select shows it.
	a.updateSettings(func(s *settings.Settings) { s.Features.Transcribe.ASR = models.ASRFast })
	a.navigate("/home")
	a.navigate("/transcribe")
	if !has(tt, "Fast speech recognition") {
		t.Error("the current choice is not shown")
	}
	if m, _ := a.featureModel(local.ASR, hotkey.Transcribe); m.ID != models.ASRFast {
		t.Errorf("the choice %+v", m)
	}
	// Hiding the model a feature uses says so.
	a.navigate(modelsPath)
	chooseMenu(t, tt, "More actions for Accurate speech recognition", "Hide")
	if !has(tt, "Accurate speech recognition is hidden. It stays in use for Translate and Rewrite until you choose another model there.") {
		t.Errorf("no toast: %q", tt.Texts())
	}
	a.updateSettings(func(s *settings.Settings) { s.Features.Transcribe.ASR = "" })

	// Show hidden lists them, marked; Show brings one back.
	tt.Advance(10 * time.Second) // the toasts, over the chips' end of the row, go
	click(t, tt, "Show hidden 2")
	if !hasExact(tt, "Fast speech recognition") || !hasExact(tt, "Hidden") {
		t.Fatalf("show hidden: %q", firstTexts(tt))
	}
	chooseMenu(t, tt, "More actions for Fast speech recognition", "Show")
	if a.prefs.Models.IsHidden(models.ASRFast) || !slices.Equal(a.settings.Get().Models.Hidden, []string{models.ASRAccurate}) {
		t.Errorf("shown: %v", a.settings.Get().Models.Hidden)
	}
}

// TestHideRemote: a remote service hides from its card's menu, and leaves
// the features' selects.
func TestHideRemote(t *testing.T) {
	ring := &memKeyring{keys: map[string]string{"llm.deepseek": "sk-1234567890abcd"}}
	a, tt := remoteApp(t, ring)
	a.updateSettings(func(s *settings.Settings) {
		s.Providers.LLM = []settings.Provider{{ID: "deepseek", Enabled: true, Model: "deepseek-chat"}}
	})
	_ = a.rkeys.Load([]string{"llm.deepseek"})
	key := settings.HiddenRemote("llm", "deepseek")
	if users := a.modelUsers(key); len(users) != 0 {
		t.Errorf("users %v", users)
	}
	a.setHidden(key, "DeepSeek", true)
	a.navigate("/models/language")
	if hasExact(tt, "DeepSeek") {
		t.Errorf("a hidden service is listed: %q", firstTexts(tt))
	}
	a.navigate("/translate")
	click(t, tt, "Language model for Translate")
	if hasExact(tt, "DeepSeek · deepseek-chat (remote)") {
		t.Error("the select offers a hidden service")
	}
}

// chooseMenu clicks the menu button labelled button, then its item.
func chooseMenu(t *testing.T, tt *ui.Tester, button, item string) {
	t.Helper()
	click(t, tt, button)
	if err := tt.ChooseMenuItem(item); err != nil {
		t.Fatalf("%s → %s: %v (menu %q)", button, item, err, tt.Menu())
	}
	frames(tt)
}
