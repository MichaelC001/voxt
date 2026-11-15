package session

import (
	"slices"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

// TestMicStoppedBeforeAudio is 11-14's first session after launch: the
// models loading, the microphone gave no audio for 5 s, the user stopped.
// It used to stop (Recognizing: the stop sound), then end cancelled as
// "too short" without a word. Now it stays recording until the capture
// answers and fails once, saying the microphone did not start; nothing
// is kept.
func TestMicStoppedBeforeAudio(t *testing.T) {
	st := &fakeStreamer{final: "hello", openWait: 300 * time.Millisecond}
	model := &prefillLLM{fakeLLM: &fakeLLM{text: "Hello."}}
	h := newHarness(t, st, model, func(c *Config, _ *settings.Settings) { c.WaitNote = 20 * time.Millisecond })
	h.audio.startWait = 10 * time.Second
	id := h.start()
	h.rec.waitFor(t, "waiting for the microphone", func(s Snapshot) bool { return s.Waiting == WaitMic })
	h.m.Stop(id)
	end := h.rec.wait(t)
	if end.State != Failed || end.Problem == nil || end.Problem.Code != ProblemMicStart {
		t.Fatalf("end %+v", end)
	}
	if slices.Contains(h.states(), Recognizing) {
		t.Errorf("states %v: recognising without audio", h.states())
	}
	if !h.audio.waitStopped(time.Second) {
		t.Error("the capture still runs")
	}
	h.hist.mu.Lock()
	if len(h.hist.entries) != 0 {
		t.Errorf("history %+v", h.hist.entries)
	}
	h.hist.mu.Unlock()
	if !h.logged("before the microphone gave any audio") {
		t.Errorf("logs %q", h.logs)
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.prefills) != 0 {
		t.Errorf("the language model warmed up before the microphone gave audio")
	}
}

// TestMicStartLimit: a microphone that gives no audio within
// MicStartLimit ends the session with an error, and its capture job.
func TestMicStartLimit(t *testing.T) {
	h := newHarness(t, &fakeStreamer{final: "hello"}, nil, func(c *Config, _ *settings.Settings) { c.MicStartLimit = 150 * time.Millisecond })
	h.audio.startWait = 10 * time.Second
	h.start()
	end := h.rec.wait(t)
	if end.State != Failed || end.Problem == nil || end.Problem.Code != ProblemMicStart {
		t.Fatalf("end %+v", end)
	}
	if !h.audio.waitStopped(time.Second) {
		t.Error("the capture still runs")
	}
}

// TestMicPrompt: while macOS asks for the microphone the session says so
// from the start, and waits for the answer longer than MicStartLimit.
func TestMicPrompt(t *testing.T) {
	h := newHarness(t, &fakeStreamer{final: "hello"}, nil, func(c *Config, _ *settings.Settings) { c.MicStartLimit = 50 * time.Millisecond })
	h.audio.startWait = 300 * time.Millisecond
	id, err := h.m.Start(StartRequest{Kind: store.KindTranscribe, Target: slack, MicPrompt: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := h.m.Current(); s.Waiting != WaitPermission {
		t.Errorf("waiting %q at the start", s.Waiting)
	}
	h.rec.waitFor(t, "the first audio", func(s Snapshot) bool { return !s.HeardAt.IsZero() && s.Waiting == WaitNone })
	h.speak(id, 5)
	if end := h.rec.wait(t); end.State != Done || end.Result != "hello" {
		t.Fatalf("end %+v", end)
	}
}

// TestColdStartWarmsAfterAudio: on a cold start the language model warms
// up once the microphone gave audio, not before (the microphone first),
// and the session works.
func TestColdStartWarmsAfterAudio(t *testing.T) {
	st := &fakeStreamer{final: "hello", openWait: 100 * time.Millisecond}
	model := &prefillLLM{fakeLLM: &fakeLLM{text: "Hello."}}
	h := newHarness(t, st, model, nil)
	h.audio.startWait = 250 * time.Millisecond
	id := h.start()
	time.Sleep(150 * time.Millisecond) // the services are there, the microphone not yet
	model.mu.Lock()
	early := len(model.prefills)
	model.mu.Unlock()
	if early != 0 {
		t.Error("warmed up before the microphone gave audio")
	}
	h.speak(id, 5)
	if end := h.rec.wait(t); end.State != Done || end.Result != "Hello." {
		t.Fatalf("end %+v", end)
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.prefills) != 1 {
		t.Errorf("%d warm-ups", len(model.prefills))
	}
}
