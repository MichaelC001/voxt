// Package trigger is the unified external trigger engine behind
// voxtmini://trigger/* URLs, voxtmini-ctl (run/control.sock), and MCP's
// trigger_session tool.
package trigger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hehehai/VoxtMini/internal/ctlbridge"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

// Actions supported by Trigger.
type Action string

const (
	ActionToggle Action = "toggle"
	ActionStart  Action = "start"
	ActionStop   Action = "stop"
	ActionCancel Action = "cancel"
)

const (
	// DefaultWaitDeviceMS is used when Device is set without an explicit wait_device_ms.
	DefaultWaitDeviceMS = 800
	// ToggleDebounce is the per-source debounce window for repeated toggle requests.
	ToggleDebounce = 200 * time.Millisecond
)

var (
	ErrDisabled       = errors.New("external trigger is disabled")
	ErrURLDisabled    = errors.New("URL trigger is disabled")
	ErrBusy           = errors.New("a session is already processing")
	ErrDeviceNotFound = errors.New("device_not_found")
)

// Request is one external trigger invocation.
type Request struct {
	Feature        string // "transcribe" | "translate" | "rewrite"
	Action         Action // "toggle" (default) | "start" | "stop" | "cancel"
	Device         string // Core Audio UID, name, substring, or transport
	Fallback       string // "" | "default" | "fail"
	WaitDeviceMS   int    // 0..3000
	WaitDeviceSet  bool   // true when WaitDeviceMS was explicitly specified
	TargetLanguage string // optional target language for translate
	Deliver        string // "" | "cursor" | "clipboard" | "card"
	Source         string // caller label for logging and toggle debounce
	ViaURL         bool   // true when invoked from voxtmini://trigger/*
	Wait           bool   // block until session ends and return text/delivered/error
}

func (r Request) validate() error {
	switch r.Action {
	case "", ActionToggle, ActionStart, ActionStop, ActionCancel:
	default:
		return fmt.Errorf("unknown action %q", r.Action)
	}
	if r.Feature != "" {
		switch r.Feature {
		case string(store.KindTranscribe), string(store.KindTranslate), string(store.KindRewrite):
		default:
			return fmt.Errorf("unknown feature %q", r.Feature)
		}
	}
	switch r.Fallback {
	case "", session.DeviceFallbackDefault, session.DeviceFallbackFail:
	default:
		return fmt.Errorf("unknown fallback %q", r.Fallback)
	}
	switch r.Deliver {
	case "", "cursor", "clipboard", "card":
	default:
		return fmt.Errorf("unknown deliver mode %q", r.Deliver)
	}
	return nil
}

// Result is the outcome of Trigger.
type Result struct {
	OK        bool
	Error     string
	SessionID string
	Feature   string
	State     string
	Text      string
	Delivered bool
	Debounced bool
	Device    *ctlbridge.DeviceInfo
}

// Config wires Service to the session manager and platform lookups.
type Config struct {
	Settings  func() settings.Settings
	Session   *session.Manager
	Start     func(session.StartRequest) (string, error) // optional; defaults to Session.Start
	Devices   func(ctx context.Context) ([]proto.CaptureDevice, error)
	Frontmost func(ctx context.Context) (proto.App, error)
	Log       func(level, msg string)
	Now       func() time.Time
}

// Service validates and dispatches external trigger requests. Goroutine-safe.
type Service struct {
	cfg Config

	mu         sync.Mutex
	lastToggle map[string]time.Time
	notifyCh   chan struct{}
}

// New returns a trigger Service.
func New(cfg Config) *Service {
	if cfg.Settings == nil {
		cfg.Settings = settings.Defaults
	}
	if cfg.Log == nil {
		cfg.Log = func(string, string) {}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{
		cfg:        cfg,
		lastToggle: map[string]time.Time{},
		notifyCh:   make(chan struct{}),
	}
}

// Notify wakes any Trigger(..., Wait: true) callers after a session snapshot changes.
func (s *Service) Notify(_ session.Snapshot) {
	s.mu.Lock()
	close(s.notifyCh)
	s.notifyCh = make(chan struct{})
	s.mu.Unlock()
}

func (s *Service) waitSignal() <-chan struct{} {
	s.mu.Lock()
	ch := s.notifyCh
	s.mu.Unlock()
	return ch
}

// Trigger executes req against the session manager.
func (s *Service) Trigger(ctx context.Context, req Request) (Result, error) {
	prefs := s.cfg.Settings()
	if !prefs.Trigger.Enabled {
		return Result{Error: ErrDisabled.Error()}, ErrDisabled
	}
	if req.ViaURL && !prefs.Trigger.AllowURLOn() {
		return Result{Error: ErrURLDisabled.Error()}, ErrURLDisabled
	}
	if req.Action == "" {
		req.Action = ActionToggle
	}
	if err := req.validate(); err != nil {
		return Result{Error: err.Error()}, err
	}
	if s.cfg.Session == nil {
		err := errors.New("session manager unavailable")
		return Result{Error: err.Error()}, err
	}

	if req.Action == ActionToggle {
		if s.debounced(req.Source) {
			cur := s.cfg.Session.Current()
			return Result{
				OK:        true,
				SessionID: cur.ID,
				Feature:   string(cur.Kind),
				State:     string(cur.State),
				Debounced: true,
				Device:    deviceFromSnap(cur),
			}, nil
		}
		cur := s.cfg.Session.Current()
		switch {
		case cur.State == session.Recording:
			req.Action = ActionStop
		case cur.State.Live():
			return Result{Error: ErrBusy.Error(), SessionID: cur.ID, State: string(cur.State)}, ErrBusy
		default:
			req.Action = ActionStart
		}
	}

	s.cfg.Log("info", fmt.Sprintf("trigger %s %s (source=%q device=%q via_url=%v)", req.Action, req.Feature, req.Source, req.Device, req.ViaURL))

	switch req.Action {
	case ActionStart:
		return s.doStart(ctx, req)
	case ActionStop:
		return s.doStop(ctx, req)
	case ActionCancel:
		return s.doCancel()
	default:
		err := fmt.Errorf("unknown action %q", req.Action)
		return Result{Error: err.Error()}, err
	}
}

func (s *Service) debounced(source string) bool {
	key := trimSource(source)
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if prev, ok := s.lastToggle[key]; ok && now.Sub(prev) < ToggleDebounce {
		return true
	}
	s.lastToggle[key] = now
	return false
}

func (s *Service) doStart(ctx context.Context, req Request) (Result, error) {
	if req.Feature == "" {
		err := errors.New("feature is required (transcribe, translate, or rewrite)")
		return Result{Error: err.Error()}, err
	}
	startReq := session.StartRequest{
		Kind:           store.Kind(req.Feature),
		TargetLanguage: req.TargetLanguage,
		Card:           req.Deliver == "card",
		Clipboard:      req.Deliver == "clipboard",
	}
	if s.cfg.Frontmost != nil {
		fctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		if app, err := s.cfg.Frontmost(fctx); err == nil {
			startReq.Target = app
		}
		cancel()
	}
	if req.Device != "" {
		waitMS := DefaultWaitDeviceMS
		if req.WaitDeviceSet {
			waitMS = clampWaitMS(req.WaitDeviceMS)
		}
		if strings.EqualFold(req.Fallback, session.DeviceFallbackFail) && s.cfg.Devices != nil {
			if err := s.requireDevice(ctx, req.Device, waitMS); err != nil {
				return Result{Error: err.Error()}, err
			}
		}
		startReq.DeviceOverride = &session.DeviceOverride{
			Query:    req.Device,
			Fallback: req.Fallback,
			WaitMS:   waitMS,
		}
	}
	startFn := s.cfg.Session.Start
	if s.cfg.Start != nil {
		startFn = s.cfg.Start
	}
	id, err := startFn(startReq)
	if err != nil {
		return Result{Error: err.Error()}, err
	}
	if req.Wait {
		return s.waitForEnd(ctx, id)
	}
	cur := s.cfg.Session.Current()
	return Result{
		OK:        true,
		SessionID: id,
		Feature:   req.Feature,
		State:     string(cur.State),
		Device:    deviceFromSnap(cur),
	}, nil
}

// requireDevice checks that query matches an available input device before
// starting a session in fallback=fail mode, polling briefly up to waitMS.
func (s *Service) requireDevice(ctx context.Context, query string, waitMS int) error {
	deadline := time.Now().Add(time.Duration(waitMS) * time.Millisecond)
	for {
		devs, err := s.cfg.Devices(ctx)
		if err == nil {
			if _, ok := session.MatchDevice(devs, query); ok {
				return nil
			}
		}
		if waitMS <= 0 || time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("%w: microphone %q not found (%v)", ErrDeviceNotFound, query, err)
			}
			return fmt.Errorf("%w: microphone %q not found", ErrDeviceNotFound, query)
		}
		sleep := min(100*time.Millisecond, time.Until(deadline))
		if sleep <= 0 {
			continue
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Service) doStop(ctx context.Context, req Request) (Result, error) {
	cur := s.cfg.Session.Current()
	if cur.State != session.Recording {
		err := errors.New("no recording in progress")
		return Result{Error: err.Error(), State: string(cur.State)}, err
	}
	s.cfg.Session.Stop(cur.ID)
	if req.Wait {
		return s.waitForEnd(ctx, cur.ID)
	}
	after := s.cfg.Session.Current()
	return Result{
		OK:        true,
		SessionID: cur.ID,
		Feature:   string(cur.Kind),
		State:     string(after.State),
		Device:    deviceFromSnap(after),
	}, nil
}

func (s *Service) doCancel() (Result, error) {
	cur := s.cfg.Session.Current()
	if cur.State.Live() {
		s.cfg.Session.Cancel(cur.ID)
		return Result{OK: true, SessionID: cur.ID, Feature: string(cur.Kind), State: string(session.Cancelled)}, nil
	}
	return Result{OK: true, State: string(cur.State)}, nil
}

func (s *Service) waitForEnd(ctx context.Context, id string) (Result, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		cur := s.cfg.Session.Current()
		if cur.ID == id && cur.State.Ended() {
			return resultFromEnded(cur), nil
		}
		if cur.ID != "" && cur.ID != id {
			// Another session replaced it.
			return Result{OK: false, Error: "session replaced", SessionID: id, State: string(cur.State)}, nil
		}
		sig := s.waitSignal()
		select {
		case <-ctx.Done():
			return Result{Error: ctx.Err().Error(), SessionID: id, State: string(cur.State)}, ctx.Err()
		case <-sig:
		case <-ticker.C:
		}
	}
}

func resultFromEnded(sn session.Snapshot) Result {
	res := Result{
		OK:        sn.State == session.Done,
		SessionID: sn.ID,
		Feature:   string(sn.Kind),
		State:     string(sn.State),
		Text:      sn.Result,
		Delivered: sn.Inserted || (sn.State == session.Done && sn.Result != "" && !sn.Card),
		Device:    deviceFromSnap(sn),
	}
	if sn.Problem != nil && sn.State != session.Done {
		if sn.Problem.Err != nil {
			res.Error = sn.Problem.Err.Error()
		} else {
			res.Error = sn.Problem.Code
		}
	}
	return res
}

// Status returns the current session status.
func (s *Service) Status() Result {
	if s.cfg.Session == nil {
		return Result{OK: true, State: string(session.Idle)}
	}
	cur := s.cfg.Session.Current()
	return Result{
		OK:        true,
		SessionID: cur.ID,
		Feature:   string(cur.Kind),
		State:     string(cur.State),
		Text:      cur.Result,
		Device:    deviceFromSnap(cur),
	}
}

// Devices lists the available audio input devices.
func (s *Service) Devices(ctx context.Context) ([]ctlbridge.DeviceInfo, error) {
	if !s.cfg.Settings().Trigger.Enabled {
		return nil, ErrDisabled
	}
	if s.cfg.Devices == nil {
		return nil, errors.New("device list unavailable")
	}
	devs, err := s.cfg.Devices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ctlbridge.DeviceInfo, 0, len(devs))
	for _, d := range devs {
		out = append(out, ctlbridge.DeviceInfo{
			ID:         d.ID,
			Name:       d.Name,
			Channels:   d.Channels,
			SampleRate: d.SampleRate,
			Default:    d.Default,
			Transport:  d.Transport,
		})
	}
	return out, nil
}

func deviceFromSnap(sn session.Snapshot) *ctlbridge.DeviceInfo {
	if sn.DeviceID == "" && sn.Device == "" {
		return nil
	}
	return &ctlbridge.DeviceInfo{
		ID:       sn.DeviceID,
		Name:     sn.Device,
		FellBack: sn.FellBack,
	}
}

// ParseAction parses a CLI action string.
func ParseAction(s string) Action {
	return Action(strings.ToLower(strings.TrimSpace(s)))
}
