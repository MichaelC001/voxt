package omni

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/omni/omnitest"
)

// testWhisper is a Whisper model the tests add to the curated ones (the
// catalogue's may change; the policy is whisperTuning's).
const testWhisper = "asr.test-whisper"

// withWhisper adds testWhisper to the specs for t, unless a curated
// Whisper is there already under that id.
func withWhisper(t *testing.T) {
	t.Helper()
	old := specs
	specs = append(slices.Clone(specs), Spec{ID: testWhisper, Repo: "mlx-community/whisper-test", Revision: "0",
		Kind: kindWhisper, Server: ServerWhisper, ASR: whisperTuning()})
	t.Cleanup(func() { specs = old })
}

// newServersFake is newFake with a runtime directory of links to the
// test binary named as the real servers (qwen3_asr_server, and
// whisper_server unless noWhisper), so that each runs as its server.
func newServersFake(t *testing.T, o omnitest.Opts, noWhisper bool) *fake {
	t.Helper()
	withWhisper(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{ServerQwen3ASR, ServerWhisper}
	if noWhisper {
		names = names[:1]
	}
	for _, n := range names {
		if err := os.Symlink(exe, filepath.Join(bin, n)); err != nil {
			t.Fatal(err)
		}
	}
	f := newFake(t, o, func(c *Config) { c.Binary = filepath.Join(bin, ServerQwen3ASR) })
	if err := os.MkdirAll(f.model(testWhisper), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestFileOnlyASR: a model whose server has no realtime API (Whisper)
// runs its own binary, without the live flags; it says it does not
// stream nor take hints, refuses a live session, and its Final sends the
// family's fields: language, the budget, temperature, never a prompt.
func TestFileOnlyASR(t *testing.T) {
	f := newServersFake(t, omnitest.Opts{Text: " Hello there. "}, false)
	a := f.asr(t, testWhisper)
	if c := a.Caps(); c.Stream || !c.File || c.Hints || c.Prompt {
		t.Errorf("caps %+v", c)
	}
	if _, err := a.Open(context.Background(), asr.StreamRequest{}); asr.KindOf(err) != asr.KindConfig {
		t.Errorf("a live session: %v", err)
	}
	res, err := a.Transcribe(context.Background(), asr.FileRequest{Audio: tone(10*time.Second, 0.3), Format: "pcm",
		Lang: "en-US", Hints: []string{"VoxtMini"}, Prompt: "a prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Hello there." || res.Lang != "en" || res.Model != testWhisper {
		t.Errorf("result %+v", res)
	}
	launches := f.records("launch")
	if len(launches) != 1 || launches[0].Binary != ServerWhisper {
		t.Fatalf("launches %+v", launches)
	}
	args := strings.Join(launches[0].Args, " ")
	if !strings.Contains(args, "--model-kind whisper") || strings.Contains(args, "--first-decode-ms") || strings.Contains(args, "--cache-limit-mb") {
		t.Errorf("launch %q", args)
	}
	finals := f.records("final")
	if len(finals) != 1 {
		t.Fatalf("%d finals", len(finals))
	}
	want := map[string]string{"model": "fake-whisper", "stream": "true", "language": "en", "max_new_tokens": "344",
		"temperature": "0", "include_generation_metadata": "true"}
	if !maps(finals[0].Fields, want) {
		t.Errorf("fields %v, want %v", finals[0].Fields, want)
	}

	// Qwen3-ASR's server is the one Config.Binary names.
	q := f.asr(t, ASRFast)
	if _, err := q.Transcribe(context.Background(), asr.FileRequest{Audio: tone(time.Second, 0.3), Format: "pcm"}); err != nil {
		t.Fatal(err)
	}
	if l := f.records("launch"); len(l) != 2 || l[1].Binary != ServerQwen3ASR {
		t.Errorf("launches %+v", l)
	}
}

func maps(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// TestFileOnlyChunks: Whisper's recordings over 30 s are requests of at
// most 30 s each, cut in their quietest 100 ms of the last 5 s (here a
// pause at 27 s), detected per request when there is no hint, and the
// texts joined with a space between words.
func TestFileOnlyChunks(t *testing.T) {
	f := newServersFake(t, omnitest.Opts{Text: "part {n}"}, false)
	a := f.asr(t, testWhisper)
	pcm := tone(50*time.Second, 0.3)
	clear(pcm[27*32000 : 27*32000+3200]) // 100 ms of silence at 27 s
	res, err := a.Transcribe(context.Background(), asr.FileRequest{Audio: pcm, Format: "pcm", Lang: "zh", Others: []string{"en"}})
	if err != nil {
		t.Fatal(err)
	}
	finals := f.records("final")
	if res.Text != "part 1 part 2" || len(finals) != 2 || res.Lang != "" {
		t.Fatalf("%+v in %d requests", res, len(finals))
	}
	if s := finals[0].Samples; s < 27*16000 || s > 27*16000+1600 {
		t.Errorf("the first request has %d samples", s)
	}
	for _, r := range finals {
		if _, ok := r.Fields["language"]; ok || r.Samples > 30*16000 {
			t.Errorf("request %v of %d samples", r.Fields, r.Samples)
		}
	}
}

// TestHintAlone: Whisper gets the main language only when the user
// speaks no other one; Qwen3-ASR always.
func TestHintAlone(t *testing.T) {
	withWhisper(t)
	w, _ := Lookup(testWhisper)
	q, _ := Lookup(ASRAccurate)
	for _, c := range []struct {
		lang   string
		others []string
		w, q   string
	}{
		{"zh-Hans", nil, "zh", "zh"},
		{"zh-Hans", []string{"en"}, "", "zh"},
		{"zh-Hans", []string{"zh", " "}, "zh", "zh"},
		{"haw", nil, "", ""}, // not one of VoxtMini's languages
		{"", nil, "", ""},
	} {
		if got := w.languageHint(c.lang, c.others); got != c.w {
			t.Errorf("whisper %q %v: %q, want %q", c.lang, c.others, got, c.w)
		}
		if got := q.languageHint(c.lang, c.others); got != c.q {
			t.Errorf("qwen %q %v: %q, want %q", c.lang, c.others, got, c.q)
		}
	}
}

// TestNoServer: a runtime without the model's server (built before it
// was staged) fails NewASR with ErrNoServer, before anything starts.
func TestNoServer(t *testing.T) {
	f := newServersFake(t, omnitest.Opts{}, true)
	_, err := NewASR(f.rt, testWhisper, f.model(testWhisper))
	if !errors.Is(err, ErrNoServer) || asr.KindOf(err) != asr.KindConfig {
		t.Fatalf("NewASR: %v", err)
	}
	if len(f.records("launch")) != 0 {
		t.Error("a server started")
	}
}
