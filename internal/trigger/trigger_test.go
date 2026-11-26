package trigger

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/ctlbridge"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
)

func voiceFrame() []byte {
	n := asr.BytesPerSecond / 10
	b := make([]byte, n)
	for i := 0; i+1 < n; i += 2 {
		v := int16(0.14 * 32767 * math.Sin(2*math.Pi*220*float64(i/2)/asr.SampleRate))
		binary.LittleEndian.PutUint16(b[i:], uint16(v))
	}
	return b
}

type stubAudio struct {
	mu      sync.Mutex
	stops   map[string]chan struct{}
	stopped map[string]bool
	last    proto.CaptureParams
}

func newStubAudio() *stubAudio {
	return &stubAudio{stops: map[string]chan struct{}{}, stopped: map[string]bool{}}
}

func (a *stubAudio) Record(ctx context.Context, id string, p proto.CaptureParams, fn func(session.AudioEvent)) (proto.CaptureResult, error) {
	ch := make(chan struct{})
	a.mu.Lock()
	a.stops[id] = ch
	a.last = p
	already := a.stopped[id]
	a.mu.Unlock()
	if already {
		close(ch)
	}

	devID := "builtin-uid"
	devName := "Built-in Mic"
	if len(p.Devices) > 0 {
		devID = p.Devices[0]
		devName = "Mic " + devID
	}
	fn(session.AudioEvent{Started: &proto.CaptureStartedParams{
		Job: id, Device: devID, DeviceName: devName,
	}})
	f := voiceFrame()
	for i := 0; i < 6; i++ {
		fn(session.AudioEvent{PCM: &proto.PCMParams{Job: id, Seq: i, Offset: i * len(f) / 2, Data: f}})
	}
	select {
	case <-ch:
		return proto.CaptureResult{Device: devID, Reason: proto.CaptureStopped, Frames: 6, Samples: 6 * len(f) / 2, Seconds: 0.6}, nil
	case <-ctx.Done():
		return proto.CaptureResult{}, ctx.Err()
	}
}

func (a *stubAudio) Stop(_ context.Context, id string) error {
	a.mu.Lock()
	a.stopped[id] = true
	ch := a.stops[id]
	a.mu.Unlock()
	if ch != nil {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
	return nil
}

type stubASR struct{ text string }

func (s stubASR) Caps() asr.Caps { return asr.Caps{} }
func (s stubASR) Transcribe(_ context.Context, _ asr.FileRequest) (asr.Result, error) {
	return asr.Result{Text: s.text}, nil
}

type stubText struct{}

func (stubText) Focus(context.Context) (proto.FocusResult, error) {
	return proto.FocusResult{PID: 42, App: &proto.App{PID: 42, BundleID: "com.apple.Notes"}, Editable: true}, nil
}
func (stubText) Paste(context.Context, proto.PasteParams) (proto.PasteResult, error) {
	return proto.PasteResult{Posted: true}, nil
}

func newTestService(t *testing.T, s *settings.Settings) (*Service, *stubAudio, *session.Manager) {
	t.Helper()
	audio := newStubAudio()
	var svc *Service
	mgr := session.New(session.Config{
		Audio: audio,
		Text:  stubText{},
		Resolve: func(context.Context, session.Kind, settings.Settings) (session.Services, error) {
			return session.Services{ASR: stubASR{text: "hello from trigger"}}, nil
		},
		Settings: func() settings.Settings { return *s },
		OnChange: func(sn session.Snapshot) {
			if svc != nil {
				svc.Notify(sn)
			}
		},
	})
	svc = New(Config{
		Settings: func() settings.Settings { return *s },
		Session:  mgr,
		Devices: func(context.Context) ([]proto.CaptureDevice, error) {
			return []proto.CaptureDevice{
				{ID: "builtin-uid", Name: "Built-in Mic", Default: true, Transport: "builtin"},
				{ID: "usb-uid", Name: "USB Mic", Transport: "usb"},
			}, nil
		},
		Frontmost: func(context.Context) (proto.App, error) {
			return proto.App{PID: 42, BundleID: "com.apple.Notes"}, nil
		},
	})
	return svc, audio, mgr
}

func TestParseURL(t *testing.T) {
	req, err := ParseURL("voxtmini://trigger/transcribe?action=start&device=usb-uid&fallback=fail&wait_device_ms=5000&deliver=clipboard&source=mic-app")
	if err != nil {
		t.Fatal(err)
	}
	if req.Feature != "transcribe" || req.Action != ActionStart || req.Device != "usb-uid" ||
		req.Fallback != "fail" || req.WaitDeviceMS != 3000 || !req.WaitDeviceSet ||
		req.Deliver != "clipboard" || req.Source != "mic-app" || !req.ViaURL {
		t.Fatalf("parsed: %+v", req)
	}

	req2, err := ParseURL("voxtmini://trigger/translate?to=en")
	if err != nil || req2.Feature != "translate" || req2.Action != ActionToggle || req2.TargetLanguage != "en" {
		t.Fatalf("translate: %+v err=%v", req2, err)
	}

	stopReq, err := ParseURL("voxtmini://trigger/stop")
	if err != nil || stopReq.Action != ActionStop {
		t.Fatalf("stop: %+v err=%v", stopReq, err)
	}

	cancelReq, err := ParseURL("voxtmini://trigger/cancel")
	if err != nil || cancelReq.Action != ActionCancel {
		t.Fatalf("cancel: %+v err=%v", cancelReq, err)
	}

	for _, bad := range []string{
		"https://example.com/trigger/transcribe",
		"voxtmini://trigger/unknown",
		"voxtmini://trigger/transcribe?action=bogus",
		"voxtmini://trigger/transcribe?fallback=bogus",
		"voxtmini://trigger/transcribe?deliver=bogus",
		"voxtmini://trigger/transcribe?wait_device_ms=abc",
	} {
		if _, err := ParseURL(bad); err == nil {
			t.Errorf("ParseURL(%q) succeeded, want error", bad)
		}
	}
}

func TestTriggerDisabledAndURLDisabled(t *testing.T) {
	s := settings.Defaults()
	s.Features.Transcribe.Enhance = false
	svc, _, _ := newTestService(t, &s)

	if _, err := svc.Trigger(context.Background(), Request{Feature: "transcribe", Action: ActionStart}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled: err=%v, want ErrDisabled", err)
	}

	s.Trigger.Enabled = true
	off := false
	s.Trigger.AllowURL = &off
	if _, err := svc.Trigger(context.Background(), Request{Feature: "transcribe", Action: ActionStart, ViaURL: true}); !errors.Is(err, ErrURLDisabled) {
		t.Fatalf("url disabled: err=%v, want ErrURLDisabled", err)
	}
}

func TestTriggerDebounceAndWait(t *testing.T) {
	s := settings.Defaults()
	s.Features.Transcribe.Enhance = false
	s.Trigger.Enabled = true
	svc, _, _ := newTestService(t, &s)

	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	svc.cfg.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

	// First toggle starts recording.
	res, err := svc.Trigger(context.Background(), Request{Feature: "transcribe", Action: ActionToggle, Source: "btn1"})
	if err != nil || !res.OK || res.State != "recording" || res.Debounced {
		t.Fatalf("start toggle: res=%+v err=%v", res, err)
	}

	// Second toggle within 200ms from same source is debounced.
	mu.Lock()
	now = now.Add(100 * time.Millisecond)
	mu.Unlock()
	res2, err := svc.Trigger(context.Background(), Request{Feature: "transcribe", Action: ActionToggle, Source: "btn1"})
	if err != nil || !res2.OK || !res2.Debounced {
		t.Fatalf("debounced toggle: res=%+v err=%v", res2, err)
	}

	// Advance past 200ms and toggle with Wait=true -> stops and waits for final result.
	mu.Lock()
	now = now.Add(250 * time.Millisecond)
	mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res3, err := svc.Trigger(ctx, Request{Feature: "transcribe", Action: ActionToggle, Source: "btn1", Wait: true})
	if err != nil || !res3.OK || res3.State != "done" || res3.Text != "hello from trigger" || !res3.Delivered {
		t.Fatalf("stop+wait toggle: res=%+v err=%v", res3, err)
	}
}

func TestSocketServerEndToEnd(t *testing.T) {
	s := settings.Defaults()
	s.Features.Transcribe.Enhance = false
	s.Trigger.Enabled = true
	svc, _, _ := newTestService(t, &s)

	home := t.TempDir()
	srv, err := Listen(home, "", svc, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	info, err := os.Stat(srv.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("socket mode = %o, want 600", info.Mode().Perm())
	}

	// devices
	dresp, err := ctlbridge.CallControl(ctlbridge.ControlOptions{Home: home}, ctlbridge.ControlRequest{Method: "devices"})
	if err != nil || !dresp.OK || len(dresp.Devices) != 2 {
		t.Fatalf("devices: resp=%+v err=%v", dresp, err)
	}

	// status
	sresp, err := ctlbridge.CallControl(ctlbridge.ControlOptions{Home: home}, ctlbridge.ControlRequest{Method: "status"})
	if err != nil || !sresp.OK || sresp.State != "idle" {
		t.Fatalf("status: resp=%+v err=%v", sresp, err)
	}

	// trigger start -> stop --wait
	tresp, err := ctlbridge.CallControl(ctlbridge.ControlOptions{Home: home}, ctlbridge.ControlRequest{
		Method: "trigger", Feature: "transcribe", Action: "start", Source: "cli",
	})
	if err != nil || !tresp.OK || tresp.State != "recording" {
		t.Fatalf("trigger start: resp=%+v err=%v", tresp, err)
	}
	wresp, err := ctlbridge.CallControl(ctlbridge.ControlOptions{Home: home}, ctlbridge.ControlRequest{
		Method: "trigger", Action: "stop", Source: "cli", Wait: true,
	})
	if err != nil || !wresp.OK || wresp.State != "done" || wresp.Text != "hello from trigger" {
		t.Fatalf("trigger stop --wait: resp=%+v err=%v", wresp, err)
	}

	// unknown device with fallback=fail fails synchronously with device_not_found
	fresp, err := ctlbridge.CallControl(ctlbridge.ControlOptions{Home: home}, ctlbridge.ControlRequest{
		Method: "trigger", Feature: "transcribe", Action: "start",
		Device: "nonexistent-mic", Fallback: "fail", WaitDeviceMS: 10, WaitDeviceSet: true, Source: "cli",
	})
	if err != nil || fresp.OK || !strings.Contains(fresp.Error, "device_not_found") {
		t.Fatalf("unknown device fallback=fail: resp=%+v err=%v", fresp, err)
	}
}
