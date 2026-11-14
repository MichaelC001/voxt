package main

// Apple's on-device models in the app (apple_link.go, models_apple.go):
// the Models page's cards, the feature selects, Settings → Permissions,
// and a whole session through them over the fake helper process.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/helper/helpertest"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/permission"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// withApple makes fs's helper answer apple.* from sim, and Speech
// Recognition from it in permissions.*.
func withApple(fs *fakeSupervisor, sim *helpertest.AppleSim) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	route := func(method string) {
		fs.handlers[method] = func(raw json.RawMessage) (any, error) {
			var p any
			json.Unmarshal(raw, &p)
			out, err := sim.Call(context.Background(), "j", method, p, nil)
			return json.RawMessage(out), err
		}
	}
	for _, m := range []string{proto.MethodAppleStatus, proto.MethodAppleInstall} {
		route(m)
	}
	perms, request := fs.handlers[proto.MethodPermissionsStatus], fs.handlers[proto.MethodPermissionsRequest]
	fs.handlers[proto.MethodPermissionsStatus] = func(raw json.RawMessage) (any, error) {
		v, _ := perms(raw)
		r := v.(proto.PermissionsResult)
		r.SpeechRecognition = sim.Permission()
		return r, nil
	}
	fs.handlers[proto.MethodPermissionsRequest] = func(raw json.RawMessage) (any, error) {
		var p proto.PermissionsRequestParams
		json.Unmarshal(raw, &p)
		if p.Permission == proto.PermissionSpeechRecognition {
			sim.RequestPermission()
		}
		v, _ := request(raw)
		r := v.(proto.PermissionsResult)
		r.SpeechRecognition = sim.Permission()
		return r, nil
	}
}

func TestAppleModelsPage(t *testing.T) {
	a, fs, tt := newTestApp(t)
	tt.SetSize(windowWidth, 2400)
	a.navigate(modelsPath)
	if has(tt, "Built into macOS") || has(tt, "Apple Speech") {
		t.Fatal("Apple's models shown by a helper without them")
	}
	sim := helpertest.NewAppleSim()
	sim.SpeechPerm = proto.PermNotDetermined
	withApple(fs, sim)
	a.recheckPermissions()
	a.navigate("/home")
	a.navigate(modelsPath) // the page reads apple.status as it shows
	for _, s := range []string{"Built into macOS", "Apple Speech", "Ready", "English: Ready · Chinese (Simplified): Ready · Japanese: Not downloaded",
		"No download from VoxtMini: macOS provides it.", "Allow speech recognition"} {
		if !has(tt, s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	// Nothing downloaded: Apple's models are the automatic ones.
	if m, ok := a.featureModel(local.ASR, hotkey.Transcribe); !ok || m.ID != apple.SpeechID || m.Name != "Apple Speech" {
		t.Errorf("transcribe's speech model %+v", m)
	}
	if m, ok := a.featureModel(local.LLM, hotkey.Translate); !ok || m.ID != apple.IntelligenceID {
		t.Errorf("translate's language model %+v", m)
	}
	if got := a.obReady(local.ASR); len(got) != 0 {
		t.Errorf("the setup guide counts Apple's model as downloaded: %v", got)
	}
	// Japanese is the user's other language: Apple downloads it.
	a.updateSettings(func(s *settings.Settings) {
		s.General.MainLanguage, s.General.OtherLanguages = "zh", []string{"ja", "fr"}
	})
	click(t, tt, "Download languages")
	if !has(tt, "Japanese: Ready") || has(tt, "Download languages") || !has(tt, "Apple Speech's languages are ready.") {
		t.Errorf("after the download: %q", tt.Texts())
	}
	click(t, tt, "Allow speech recognition")
	if a.perms.speech != permission.Granted || has(tt, "Allow speech recognition") {
		t.Errorf("speech recognition %v", a.perms.speech)
	}

	// A downloaded model goes first; Apple's stays a choice.
	seedModels(t, a, models.ASRFast, models.VADSilero)
	a.refreshApple()
	if m, _ := a.featureModel(local.ASR, hotkey.Transcribe); m.ID != models.ASRFast {
		t.Errorf("automatic with a downloaded model: %+v", m)
	}
	opts, _ := modelOptions(a.installed(local.ASR), a.installed(local.ASR))
	if !strings.Contains(strings.Join(opts, "|"), "|Apple Speech") {
		t.Errorf("select %q", opts)
	}

	// Apple Intelligence turned off: its card says why.
	sim.Set(func(s *helpertest.AppleSim) { s.Intelligence = proto.AppleReasonNotEnabled })
	a.refreshApple()
	a.mdTab = 1
	if !has(tt, "Apple Intelligence") || !has(tt, "Turn on Apple Intelligence in System Settings to use it.") || !has(tt, "Not available") {
		t.Errorf("intelligence off: %q", tt.Texts())
	}
	if _, ok := a.featureModel(local.LLM, hotkey.Rewrite); ok {
		t.Error("a language model without Apple Intelligence")
	}
	// Hidden like any card.
	a.setHidden(apple.IntelligenceID, "Apple Intelligence", true)
	if hasExact(tt, "Apple Intelligence") {
		t.Error("hidden, still listed")
	}
}

// TestAppleSessionEndToEnd: nothing downloaded, the helper has Apple's
// models: a shortcut, speech, a shortcut; Apple Speech hears it (live
// text first) and Apple Intelligence cleans it up.
func TestAppleSessionEndToEnd(t *testing.T) {
	ev := script(t, hotkeyLine(400, "transcribe/0", proto.PhaseTrigger), hotkeyLine(1000, "transcribe/0", proto.PhaseTrigger))
	a := newSessionApp(t, "apple,"+ev, func(s *settings.Settings) { s.General.MainLanguage = "en" }, nil)
	a.pump("Apple's models", func() bool { return a.appleSt.SpeechReady() && a.appleSt.IntelligenceReady() })
	partial := false
	a.pump("the end", func() bool {
		partial = partial || a.sess.State == session.Recording && strings.HasPrefix(a.sess.Partial, "Hello")
		return a.sess.State.Ended()
	})
	end := a.sess
	if end.State != session.Done || end.Raw != "Hello from Apple Speech." || !strings.HasPrefix(end.Result, "Apple:") || !partial {
		t.Fatalf("end %+v (live text %v)", end, partial)
	}
	es := a.history()
	if len(es) != 1 || es[0].ASRProvider != apple.SpeechID || es[0].LLMProvider != apple.IntelligenceID {
		t.Errorf("history %+v", es)
	}
}
