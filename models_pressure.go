package main

// Memory-aware unloading (audit #37, WP-A): with "Keep models loaded" on
// Automatic, a model left idle for a minute is unloaded when the Mac is
// short of memory (macOS's pressure warning or critical, or swap at least
// 80 % used), instead of waiting out its 5 or 10 minutes: on a Mac that
// swaps, an idle model is swapped out anyway (so keeping it does not make
// the next dictation quicker) and its gigabytes push the user's other
// apps into swap. A fixed choice keeps its time; a session that runs is
// never interrupted.
//
// The pressure is read with sysctl (macx.ReadSystemMemory, microseconds)
// every pressurePoll while a model is loaded, never otherwise. When each
// model last went idle comes from the runtimes' resident events
// (modelEvents, family_runtime.go). Goroutine-safe: the events and the
// checks run on goroutines of their own.

import (
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hehehai/VoxtMini/internal/macx"
	"github.com/hehehai/VoxtMini/internal/resident"
	"github.com/hehehai/VoxtMini/internal/settings"
)

const (
	// pressurePoll is how often the memory is read while a model is loaded.
	pressurePoll = 30 * time.Second
	// pressureIdle is how long a model must be idle to go under pressure.
	pressureIdle = time.Minute
	// pressureSwapPercent is the swap use that counts as short of memory.
	pressureSwapPercent = 80
)

// sysMemory reads the Mac's memory (a test seam; an error off a Mac).
var sysMemory = macx.ReadSystemMemory

// idleMark is a loaded model's use: busy, or idle since when.
type idleMark struct {
	busy  bool
	since time.Time
}

// pressureBook is the loaded models' uses and the check's timer.
type pressureBook struct {
	mu      sync.Mutex
	models  map[string]idleMark
	ticking bool // a check is due (a model is loaded)
	// after schedules the next check (tests: by hand); nil: time.AfterFunc.
	after func(d time.Duration, fn func())
}

// noteModel records a runtime's event; the first model loaded starts the
// checks.
func (a *app) noteModel(ev resident.Event, now time.Time) {
	b := &a.pressure
	b.mu.Lock()
	if b.models == nil {
		b.models = map[string]idleMark{}
	}
	switch ev.Kind {
	case resident.Loaded, resident.Busy:
		b.models[ev.ID] = idleMark{busy: true}
	case resident.Idle:
		b.models[ev.ID] = idleMark{since: now}
	case resident.Unloaded:
		delete(b.models, ev.ID)
	}
	start := len(b.models) > 0 && !b.ticking
	if start {
		b.ticking = true
	}
	b.mu.Unlock()
	if start {
		a.schedulePressure()
	}
}

func (a *app) schedulePressure() {
	if after := a.pressure.after; after != nil {
		after(pressurePoll, a.pressureTick)
		return
	}
	time.AfterFunc(pressurePoll, a.pressureTick)
}

// pressureTick is a check; the next one is scheduled while a model is
// loaded.
func (a *app) pressureTick() {
	b := &a.pressure
	b.mu.Lock()
	if len(b.models) == 0 {
		b.ticking = false
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()
	a.checkPressure(time.Now())
	a.schedulePressure()
}

// shortOfMemory reports whether m says the Mac is short of memory, and
// how.
func shortOfMemory(m macx.SystemMemory) bool {
	if m.Pressure == "warning" || m.Pressure == "critical" {
		return true
	}
	return m.SwapTotalBytes > 0 && m.SwapUsedBytes*100 >= pressureSwapPercent*m.SwapTotalBytes
}

// pressureUnloads are the models to unload now: with keep Automatic, no
// session running and the Mac short of memory, those idle for
// pressureIdle, in id order.
func pressureUnloads(marks map[string]idleMark, now time.Time, keep settings.Models, live bool, m macx.SystemMemory) []string {
	if _, auto := keep.KeepFor(); !auto || live || !shortOfMemory(m) {
		return nil
	}
	var out []string
	for id, mk := range marks {
		if !mk.busy && now.Sub(mk.since) >= pressureIdle {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// checkPressure reads the memory and unloads the idle models it should,
// with a line in the log.
func (a *app) checkPressure(now time.Time) {
	if _, auto := a.keepSetting().KeepFor(); !auto {
		return // no system call either
	}
	m, err := sysMemory()
	if err != nil {
		return
	}
	b := &a.pressure
	b.mu.Lock()
	marks := maps.Clone(b.models)
	b.mu.Unlock()
	ids := pressureUnloads(marks, now, a.keepSetting(), a.sessionLive(), m)
	if len(ids) == 0 {
		return
	}
	var what []string
	for _, id := range ids {
		what = append(what, fmt.Sprintf("%s (idle %s)", id, now.Sub(marks[id].since).Round(time.Second)))
		a.unloadIdleModel(id) // family_runtime.go: never one in use
	}
	log.Printf("models: %s: unloading the idle %s", m, strings.Join(what, ", "))
}

// sessionLive reports whether a session runs (goroutine-safe): its
// models stay, idle or not (a language model waits while the user
// speaks).
func (a *app) sessionLive() bool {
	return a.mgr != nil && a.mgr.Current().State.Live()
}
