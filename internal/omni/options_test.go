package omni

import (
	"context"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/knobs"
	"github.com/hehehai/VoxtMini/internal/omni/omnitest"
)

// TestOptions: the user's options reach the next Final of their model
// only: Whisper's temperature, no language hint when the model detects
// it; Qwen3-ASR takes no temperature.
func TestOptions(t *testing.T) {
	f := newServersFake(t, omnitest.Opts{Text: "Hello."}, false)
	w := f.asr(t, testWhisper)
	q := f.asr(t, ASRFast)
	half := 0.4
	f.rt.SetOptions(map[string]Options{testWhisper: {DetectLanguage: true, Temperature: &half}, ASRFast: {Temperature: &half}})
	req := asr.FileRequest{Audio: tone(time.Second, 0.3), Format: "pcm", Lang: "en-US"}
	for _, a := range []*ASR{w, q} {
		if _, err := a.Transcribe(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	f.rt.SetOptions(nil)
	if _, err := w.Transcribe(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	finals := f.records("final")
	if len(finals) != 3 {
		t.Fatalf("%d finals", len(finals))
	}
	if _, ok := finals[0].Fields["language"]; ok || finals[0].Fields["temperature"] != "0.4" {
		t.Errorf("whisper with options %v", finals[0].Fields)
	}
	if _, ok := finals[1].Fields["temperature"]; ok || finals[1].Fields["language"] != "en" {
		t.Errorf("qwen %v", finals[1].Fields)
	}
	if finals[2].Fields["language"] != "en" || finals[2].Fields["temperature"] != "0" {
		t.Errorf("whisper without options %v", finals[2].Fields)
	}
}

// TestSpecKnobs: every speech model takes the main language; Whisper its
// temperature too; Silero nothing.
func TestSpecKnobs(t *testing.T) {
	for _, s := range specs {
		k := s.Knobs()
		switch {
		case s.VAD != nil:
			if k != nil {
				t.Errorf("%s: %+v", s.ID, k)
			}
		case s.Kind == kindWhisper:
			if len(k) != 2 || k[0].Knob != knobs.MainLanguage || !k[0].On() || k[1].Knob != knobs.Temperature || k[1].Max != 1 {
				t.Errorf("%s: %+v", s.ID, k)
			}
		default:
			if len(k) != 1 || k[0].Knob != knobs.MainLanguage {
				t.Errorf("%s: %+v", s.ID, k)
			}
		}
	}
}
