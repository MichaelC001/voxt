package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/llmrt"
	"github.com/hehehai/VoxtMini/internal/llmrt/llmrttest"
	"github.com/hehehai/VoxtMini/internal/macx"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/omni/omnitest"
	"github.com/hehehai/VoxtMini/internal/resident"
	"github.com/hehehai/VoxtMini/internal/settings"
)

var (
	calm     = macx.SystemMemory{Pressure: "normal", SwapUsedBytes: 7 << 30, SwapTotalBytes: 10 << 30}
	warned   = macx.SystemMemory{Pressure: "warning"}
	swapping = macx.SystemMemory{Pressure: "normal", SwapUsedBytes: 95, SwapTotalBytes: 100}
)

// TestPressureUnloads: only Automatic, no session, short of memory
// (macOS's pressure warning or critical; a full swap alone is not), and
// only the models idle for a minute.
func TestPressureUnloads(t *testing.T) {
	now := time.Unix(1000, 0)
	marks := map[string]idleMark{
		"asr.fast":   {since: now.Add(-pressureIdle)},
		"llm.fast":   {since: now.Add(-pressureIdle + time.Second)},
		"vad.silero": {since: now.Add(-time.Hour)},
		"asr.busy":   {busy: true},
	}
	auto := settings.Models{KeepLoaded: settings.KeepAuto}
	idle := []string{"asr.fast", "vad.silero"}
	for _, c := range []struct {
		name string
		keep settings.Models
		live bool
		mem  macx.SystemMemory
		want []string
	}{
		{"calm", auto, false, calm, nil},
		{"warning", auto, false, warned, idle},
		{"critical", auto, false, macx.SystemMemory{Pressure: "critical"}, idle},
		{"swap 95%, pressure normal", auto, false, swapping, nil},
		{"swap 95%, no level", auto, false, macx.SystemMemory{SwapUsedBytes: 95, SwapTotalBytes: 100}, nil},
		{"no swap", auto, false, macx.SystemMemory{Pressure: "normal"}, nil},
		{"a session runs", auto, true, warned, nil},
		{"15 minutes", settings.Models{KeepLoaded: settings.Keep15Min}, false, warned, nil},
		{"until quit", settings.Models{KeepLoaded: settings.KeepUntilQuit}, false, warned, nil},
	} {
		if got := pressureUnloads(marks, now, c.keep, c.live, c.mem); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// pressureRig makes a's checks run by hand and its memory read mem.
type pressureRig struct {
	mu     sync.Mutex
	mem    macx.SystemMemory
	reads  int
	timers int
}

func installPressure(t *testing.T, a *app, mem macx.SystemMemory) *pressureRig {
	r := &pressureRig{mem: mem}
	saved := sysMemory
	sysMemory = func() (macx.SystemMemory, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.reads++
		return r.mem, nil
	}
	t.Cleanup(func() { sysMemory = saved })
	a.pressure.after = func(time.Duration, func()) { r.mu.Lock(); r.timers++; r.mu.Unlock() }
	return r
}

// TestPressureUnloadsIdleModels: the models idle for a minute go when
// macOS reports memory pressure, stay when it does not (a full swap
// alone included) or after less than a minute; the checks start with the first model loaded and stop
// when none is.
func TestPressureUnloadsIdleModels(t *testing.T) {
	fakeOmni(t, omnitest.Opts{})
	a, _, _ := newTestApp(t)
	t.Setenv(llmrt.DevEnv, "")
	rig := installPressure(t, a, calm)
	seedModels(t, a, models.ASRFast, models.VADSilero, models.LLMFast)
	self, _ := os.Executable()
	a.llmrt = llmrt.New(llmrt.Config{Bin: self, Env: append(llmrttest.Env(""), omnitest.EnvFake+"="), IdleTimeout: -1,
		OnModel: a.modelEvents(workerLLM), BeforeLoad: a.beforeLoad()})
	if rig.timers != 0 {
		t.Fatal("checks with nothing loaded")
	}
	if _, _, err := a.local.ASR(context.Background(), models.ASRFast); err != nil {
		t.Fatal(err)
	}
	chat, _, err := a.local.LLM(context.Background(), models.LLMFast)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chat.Chat(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.User, Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	idle := func(id string) bool {
		a.pressure.mu.Lock()
		defer a.pressure.mu.Unlock()
		m, ok := a.pressure.models[id]
		return ok && !m.busy
	}
	for _, id := range []string{models.ASRFast, models.VADSilero, models.LLMFast} {
		eventually(t, id+" idle", func() bool { return idle(id) })
	}
	if rig.timers != 1 {
		t.Errorf("%d checks scheduled, want 1", rig.timers)
	}
	later := time.Now().Add(pressureIdle + time.Second)
	a.checkPressure(later) // calm: all stay
	if st := a.llmrt.Status(); st.State != llmrt.Ready {
		t.Fatalf("calm: %+v", st)
	}
	rig.mu.Lock()
	rig.mem = swapping
	rig.mu.Unlock()
	a.checkPressure(later) // swap 95 %, pressure normal: all stay
	if st := a.llmrt.Status(); st.State != llmrt.Ready {
		t.Fatalf("swap only: %+v", st)
	}
	a.checkPressure(time.Now()) // pressed, but just used
	rig.mu.Lock()
	rig.mem = warned
	rig.mu.Unlock()
	a.checkPressure(time.Now())
	if st := a.llmrt.Status(); st.State != llmrt.Ready {
		t.Fatalf("idle for less than a minute: %+v", st)
	}
	// A fixed choice keeps them, and reads nothing.
	a.updateSettings(func(s *settings.Settings) { s.Models.KeepLoaded = settings.Keep15Min })
	reads := rig.reads
	a.checkPressure(later)
	if rig.reads != reads || a.llmrt.Status().State != llmrt.Ready {
		t.Errorf("15 minutes: %d reads, %+v", rig.reads-reads, a.llmrt.Status())
	}
	a.updateSettings(func(s *settings.Settings) { s.Models.KeepLoaded = settings.KeepAuto })
	a.checkPressure(later)
	eventually(t, "the unloads", func() bool {
		return a.llmrt.Status().State == llmrt.Stopped && len(a.omniRT.Processes()) == 0
	})
	eventually(t, "the events", func() bool {
		a.pressure.mu.Lock()
		defer a.pressure.mu.Unlock()
		return len(a.pressure.models) == 0
	})
	a.pressureTick() // nothing loaded: the checks stop
	if a.pressure.ticking || rig.timers != 1 {
		t.Errorf("ticking %v, %d timers", a.pressure.ticking, rig.timers)
	}
}

// TestPressureChecksStart: the first model loaded starts the checks
// (one timer however many events); off a Mac (no memory to read) a check
// unloads nothing and they go on while a model is loaded.
func TestPressureChecksStart(t *testing.T) {
	a, _, _ := newTestApp(t)
	rig := installPressure(t, a, warned)
	sysMemory = func() (macx.SystemMemory, error) { return macx.SystemMemory{}, errors.New("not a Mac") }
	a.noteModel(resident.Event{Kind: resident.Loaded, ID: "llm.fast"}, time.Now())
	a.noteModel(resident.Event{Kind: resident.Idle, ID: "llm.fast"}, time.Now().Add(-time.Hour))
	a.noteModel(resident.Event{Kind: resident.Loaded, ID: "asr.fast"}, time.Now())
	if rig.timers != 1 {
		t.Fatalf("%d timers", rig.timers)
	}
	a.pressureTick()
	if rig.timers != 2 || len(a.pressure.models) != 2 {
		t.Errorf("%d timers, models %v", rig.timers, a.pressure.models)
	}
	a.noteModel(resident.Event{Kind: resident.Unloaded, ID: "llm.fast"}, time.Now())
	a.noteModel(resident.Event{Kind: resident.Unloaded, ID: "asr.fast"}, time.Now())
	a.pressureTick()
	if a.pressure.ticking || rig.timers != 2 {
		t.Errorf("nothing loaded: ticking %v, %d timers", a.pressure.ticking, rig.timers)
	}
	a.noteModel(resident.Event{Kind: resident.Loaded, ID: "llm.fast"}, time.Now())
	if rig.timers != 3 {
		t.Errorf("loaded again: %d timers", rig.timers)
	}
}
