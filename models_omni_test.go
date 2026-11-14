package main

import (
	"context"
	"errors"
	fammodels "github.com/hehehai/actnowkit/models"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/omni"
	"github.com/hehehai/VoxtMini/internal/omni/omnitest"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// fakeOmni makes this test binary the app's Omni runtime (omnitest):
// links named as its servers (qwen3_asr_server, whisper_server) in a
// runtime's bin/. It returns the fake's log.
func fakeOmni(t *testing.T, o omnitest.Opts) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{omni.ServerQwen3ASR, omni.ServerWhisper} {
		if err := os.Symlink(self, filepath.Join(bin, n)); err != nil {
			t.Fatal(err)
		}
	}
	o.Log = filepath.Join(t.TempDir(), "omni.jsonl")
	old := omniLocate
	omniLocate = func() (string, error) { return filepath.Join(bin, omni.ServerQwen3ASR), nil }
	t.Cleanup(func() { omniLocate = old })
	k, v, _ := strings.Cut(omnitest.Env(o)[0], "=")
	t.Setenv(k, v)
	return o.Log
}

// TestSessionOmni: a Transcribe session through the app's own local
// models (the Library and localOpeners) over the fake Omni server: the
// live preview while recording, then the Final; Silero (installed with
// the speech model) gates the stream; deleting the model unloads it;
// quitting stops the servers.
func TestSessionOmni(t *testing.T) {
	log := fakeOmni(t, omnitest.Opts{Text: "来自 Omni 的文本"})
	ev := script(t, hotkeyLine(0, "transcribe/0", proto.PhaseTrigger), hotkeyLine(2500, "transcribe/0", proto.PhaseTrigger))
	a := newSessionApp(t, ev, func(s *settings.Settings) {
		s.Features.Transcribe.Enhance = false
		s.General.MainLanguage = "zh"
	}, realLocal{installed: []string{models.ASRFast, models.VADSilero}})
	partial := false
	a.pump("the end", func() bool {
		partial = partial || a.sess.State == session.Recording && strings.HasPrefix(a.sess.Partial, "partial ")
		return a.sess.State.Ended()
	})
	end := a.sess
	if end.State != session.Done || end.Raw != "来自 Omni 的文本" || end.ASR.ID != models.ASRFast {
		t.Fatalf("end %s %+v %q", end.State, end.Problem, end.Raw)
	}
	if !partial {
		t.Error("no live partial")
	}
	rs := omnitest.Records(log)
	finals := omnitest.Of(rs, "final")
	if len(omnitest.Of(rs, "launch")) != 2 || len(finals) != 1 || finals[0].Fields["language"] != "zh" || len(omnitest.Of(rs, "vad-stream")) != 1 {
		t.Errorf("omni records %+v", rs)
	}

	// Deleting the model stops its server first.
	if _, err := lib(a.app).Delete(context.Background(), models.ASRFast, fammodels.ThisApp); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the unload", func() bool { return len(omnitest.Of(omnitest.Records(log), "shutdown")) == 1 })
	a.quit()
	if n := len(omnitest.Of(omnitest.Records(log), "shutdown")); n != 2 {
		t.Errorf("%d servers shut down", n)
	}
	if _, err := a.openOmniASR(context.Background(), models.ASRFast, t.TempDir()); !errors.Is(err, local.ErrNoRuntime) {
		t.Errorf("an open after quit: %v", err)
	}
}

// TestSessionWhisper: a Transcribe session on Whisper (a file-only
// server): no live text while recording, "Recognizing…" after Stop, the
// recording sent once to whisper_server from the folder of links in the
// data folder (the MLX checkpoint and the tokenizer), Silero beside it;
// the other language the user speaks means no language hint. Deleting
// the model removes its links, never $HUB's tokenizer files of a model
// that shares them.
func TestSessionWhisper(t *testing.T) {
	log := fakeOmni(t, omnitest.Opts{Text: "Hello from Whisper."})
	ev := script(t, hotkeyLine(0, "transcribe/0", proto.PhaseTrigger), hotkeyLine(2500, "transcribe/0", proto.PhaseTrigger))
	a := newSessionApp(t, ev, func(s *settings.Settings) {
		s.Features.Transcribe.Enhance = false
		s.Features.Transcribe.ASR = models.ASRWhisperTurbo
		s.General.MainLanguage, s.General.OtherLanguages = "zh", []string{"en"}
	}, realLocal{installed: []string{models.ASRWhisperTurbo, models.ASRWhisperLarge, models.VADSilero}})
	partial, recognizing := false, false
	a.pump("the end", func() bool {
		partial = partial || a.sess.State == session.Recording && a.sess.Partial != ""
		recognizing = recognizing || a.sess.State == session.Recognizing && uitext.WorkingText(a.sess) == "Recognizing…"
		return a.sess.State.Ended()
	})
	end := a.sess
	if end.State != session.Done || end.Raw != "Hello from Whisper." || end.ASR.ID != models.ASRWhisperTurbo {
		t.Fatalf("end %s %+v %q", end.State, end.Problem, end.Raw)
	}
	if partial || !recognizing {
		t.Errorf("live text %v, recognizing %v", partial, recognizing)
	}
	rs := omnitest.Records(log)
	finals, launches := omnitest.Of(rs, "final"), omnitest.Of(rs, "launch")
	if len(finals) != 1 || finals[0].Fields["temperature"] != "0" || finals[0].Fields["prompt"] != "" || len(omnitest.Of(rs, "live")) != 0 {
		t.Fatalf("finals %+v", finals)
	}
	if _, hint := finals[0].Fields["language"]; hint {
		t.Error("a language hint while the user speaks another language")
	}
	links := filepath.Join(a.dataDir(), "models", models.ASRWhisperTurbo)
	var whisper omnitest.Record
	for _, l := range launches {
		if l.Binary == omni.ServerWhisper {
			whisper = l
		}
	}
	if !slices.Contains(whisper.Args, links) || !slices.Contains(whisper.Args, "whisper") {
		t.Errorf("launches %+v", launches)
	}
	for _, f := range []string{"config.json", "weights.safetensors", "vocab.json", "merges.txt", "tokenizer_config.json", "generation_config.json"} {
		if _, err := os.Stat(filepath.Join(links, f)); err != nil {
			t.Errorf("the folder of links: %v", err)
		}
	}

	// Deleting it removes its folder; the tokenizer stays for large-v3.
	tok := filepath.Join(a.models.Hub(), "models--openai--whisper-large-v3")
	if _, err := lib(a.app).Delete(context.Background(), models.ASRWhisperTurbo, fammodels.ThisApp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(links); !os.IsNotExist(err) {
		t.Errorf("the links stay: %v", err)
	}
	if _, err := os.Stat(tok); err != nil || !a.models.Installed(models.ASRWhisperLarge) {
		t.Errorf("the shared tokenizer went: %v", err)
	}
	a.quit()
}

// TestOmniOpener: the opener is cheap after the first call, carries
// Silero only when it is installed, and says "no runtime" without one.
func TestOmniOpener(t *testing.T) {
	fakeOmni(t, omnitest.Opts{})
	a := newSessionApp(t, "", func(*settings.Settings) {}, realLocal{installed: []string{models.ASRFast}})
	dir := a.models.Dir(models.ASRFast)
	p1, err := a.openOmniASR(context.Background(), models.ASRFast, dir)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := a.openOmniASR(context.Background(), models.ASRFast, dir)
	if p1 != p2 {
		t.Error("a second open made another provider")
	}
	if va, ok := p1.(local.VoiceActivity); !ok || va.VAD() != nil {
		t.Error("a VAD that is not installed")
	}
	if _, err := a.openOmniASR(context.Background(), "llm.fast", dir); err == nil {
		t.Error("a language model opened as speech")
	}

	old := omniLocate
	omniLocate = func() (string, error) { return "", errors.New("no runtime here") }
	defer func() { omniLocate = old }()
	b := &app{}
	if _, err := b.openOmniASR(context.Background(), models.ASRFast, dir); !errors.Is(err, local.ErrNoRuntime) {
		t.Errorf("without a runtime: %v", err)
	}
}

// eventually waits for cond, at most 10 s.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out: %s", what)
}

// lib is a's local models' Library.
func lib(a *app) *local.Library {
	l, _ := a.localLibrary()
	return l
}
