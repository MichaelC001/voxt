package session

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

type fakeDevices struct {
	fn func(ctx context.Context) ([]proto.CaptureDevice, error)
}

func (f fakeDevices) Devices(ctx context.Context) ([]proto.CaptureDevice, error) {
	return f.fn(ctx)
}

type captureSpy struct {
	*fakeAudio
	mu       sync.Mutex
	got      proto.CaptureParams
	fellBack bool
	reason   string
}

func (c *captureSpy) Record(ctx context.Context, id string, p proto.CaptureParams, fn func(AudioEvent)) (proto.CaptureResult, error) {
	c.mu.Lock()
	c.got = p
	fb, reason := c.fellBack, c.reason
	c.mu.Unlock()
	if fb {
		fn(AudioEvent{Started: &proto.CaptureStartedParams{
			Job:            id,
			Device:         "builtin-uid",
			DeviceName:     "MacBook Pro Microphone",
			FellBack:       true,
			FallbackReason: reason,
		}})
		<-ctx.Done()
		return proto.CaptureResult{}, ctx.Err()
	}
	return c.fakeAudio.Record(ctx, id, p, fn)
}

func sampleDevices() []proto.CaptureDevice {
	return []proto.CaptureDevice{
		{ID: "builtin-uid", Name: "MacBook Pro Microphone", Channels: 1, SampleRate: 48000, Default: true, Transport: "builtin"},
		{ID: "usb-mic-uid", Name: "MyMic Pro USB", Channels: 1, SampleRate: 48000, Transport: "usb"},
		{ID: "bt-mic-uid", Name: "Lavalier Clip", Channels: 1, SampleRate: 16000, Transport: "bluetooth"},
	}
}

func TestMatchDevicePriority(t *testing.T) {
	devs := sampleDevices()
	cases := []struct {
		query string
		want  string
	}{
		{"usb-mic-uid", "usb-mic-uid"},
		{"MyMic Pro USB", "usb-mic-uid"},
		{"mymic pro usb", "usb-mic-uid"},
		{"lavalier", "bt-mic-uid"},
		{"bluetooth", "bt-mic-uid"},
		{"usb", "usb-mic-uid"},
		{"builtin", "builtin-uid"},
	}
	for _, tc := range cases {
		m, ok := MatchDevice(devs, tc.query)
		if !ok || m.ID != tc.want {
			t.Errorf("MatchDevice(%q) = (%q, %v), want %q", tc.query, m.ID, ok, tc.want)
		}
	}
	if _, ok := MatchDevice(devs, "nonexistent"); ok {
		t.Error("MatchDevice(nonexistent) unexpectedly matched")
	}
}

func TestDeviceOverrideModes(t *testing.T) {
	userDevs := []string{"builtin-uid", "other-uid"}
	origCopy := slices.Clone(userDevs)
	s := settings.Defaults()
	s.Audio.Devices = userDevs
	s.Features.Transcribe.Enhance = false

	newMgr := func(t *testing.T, spy *captureSpy, devs Devices) (*Manager, chan Snapshot) {
		t.Helper()
		ended := make(chan Snapshot, 1)
		m := New(Config{
			Audio:   spy,
			Devices: devs,
			Text:    &fakeText{},
			Resolve: func(context.Context, Kind, settings.Settings) (Services, error) {
				return Services{ASR: &fakeFile{text: "hello world"}}, nil
			},
			Settings:   func() settings.Settings { return s },
			DevicePoll: 10 * time.Millisecond,
			OnChange: func(sn Snapshot) {
				if sn.State.Ended() {
					select {
					case ended <- sn:
					default:
					}
				}
			},
		})
		return m, ended
	}

	t.Run("uid match fallback=default keeps user devices after override", func(t *testing.T) {
		spy := &captureSpy{fakeAudio: newFakeAudio()}
		spy.maxFrame = 6
		m, ended := newMgr(t, spy, fakeDevices{fn: func(context.Context) ([]proto.CaptureDevice, error) {
			return sampleDevices(), nil
		}})
		if _, err := m.Start(StartRequest{
			Kind:           store.KindTranscribe,
			DeviceOverride: &DeviceOverride{Query: "usb-mic-uid", Fallback: DeviceFallbackDefault},
		}); err != nil {
			t.Fatal(err)
		}
		sn := <-ended
		if sn.State != Done {
			t.Fatalf("state=%s problem=%+v", sn.State, sn.Problem)
		}
		want := []string{"usb-mic-uid", "builtin-uid", "other-uid"}
		if !slices.Equal(spy.got.Devices, want) {
			t.Fatalf("CaptureParams.Devices = %v, want %v", spy.got.Devices, want)
		}
		if !slices.Equal(s.Audio.Devices, origCopy) {
			t.Fatalf("user settings mutated: %v", s.Audio.Devices)
		}
	})

	t.Run("name match fallback=fail passes only matched device", func(t *testing.T) {
		spy := &captureSpy{fakeAudio: newFakeAudio()}
		spy.maxFrame = 6
		m, ended := newMgr(t, spy, fakeDevices{fn: func(context.Context) ([]proto.CaptureDevice, error) {
			return sampleDevices(), nil
		}})
		if _, err := m.Start(StartRequest{
			Kind:           store.KindTranscribe,
			DeviceOverride: &DeviceOverride{Query: "Lavalier", Fallback: DeviceFallbackFail},
		}); err != nil {
			t.Fatal(err)
		}
		sn := <-ended
		if sn.State != Done {
			t.Fatalf("state=%s problem=%+v", sn.State, sn.Problem)
		}
		if !slices.Equal(spy.got.Devices, []string{"bt-mic-uid"}) {
			t.Fatalf("CaptureParams.Devices = %v, want [bt-mic-uid]", spy.got.Devices)
		}
	})

	t.Run("wait and appear", func(t *testing.T) {
		spy := &captureSpy{fakeAudio: newFakeAudio()}
		spy.maxFrame = 6
		var calls atomic.Int32
		m, ended := newMgr(t, spy, fakeDevices{fn: func(context.Context) ([]proto.CaptureDevice, error) {
			if calls.Add(1) < 3 {
				return sampleDevices()[:1], nil // only builtin initially
			}
			return sampleDevices(), nil
		}})
		if _, err := m.Start(StartRequest{
			Kind:           store.KindTranscribe,
			DeviceOverride: &DeviceOverride{Query: "bt-mic-uid", Fallback: DeviceFallbackFail, WaitMS: 500},
		}); err != nil {
			t.Fatal(err)
		}
		sn := <-ended
		if sn.State != Done {
			t.Fatalf("state=%s problem=%+v", sn.State, sn.Problem)
		}
		if !slices.Equal(spy.got.Devices, []string{"bt-mic-uid"}) {
			t.Fatalf("CaptureParams.Devices = %v, want [bt-mic-uid]", spy.got.Devices)
		}
		if calls.Load() < 3 {
			t.Fatalf("expected polling calls >= 3, got %d", calls.Load())
		}
	})

	t.Run("not found fallback=default uses user devices", func(t *testing.T) {
		spy := &captureSpy{fakeAudio: newFakeAudio()}
		spy.maxFrame = 6
		m, ended := newMgr(t, spy, fakeDevices{fn: func(context.Context) ([]proto.CaptureDevice, error) {
			return sampleDevices(), nil
		}})
		if _, err := m.Start(StartRequest{
			Kind:           store.KindTranscribe,
			DeviceOverride: &DeviceOverride{Query: "missing-mic", Fallback: DeviceFallbackDefault},
		}); err != nil {
			t.Fatal(err)
		}
		sn := <-ended
		if sn.State != Done {
			t.Fatalf("state=%s problem=%+v", sn.State, sn.Problem)
		}
		if !slices.Equal(spy.got.Devices, origCopy) {
			t.Fatalf("CaptureParams.Devices = %v, want %v", spy.got.Devices, origCopy)
		}
	})

	t.Run("not found fallback=fail aborts with ProblemDevice", func(t *testing.T) {
		spy := &captureSpy{fakeAudio: newFakeAudio()}
		m, ended := newMgr(t, spy, fakeDevices{fn: func(context.Context) ([]proto.CaptureDevice, error) {
			return sampleDevices(), nil
		}})
		if _, err := m.Start(StartRequest{
			Kind:           store.KindTranscribe,
			DeviceOverride: &DeviceOverride{Query: "missing-mic", Fallback: DeviceFallbackFail, WaitMS: 30},
		}); err != nil {
			t.Fatal(err)
		}
		sn := <-ended
		if sn.State != Failed || sn.Problem == nil || sn.Problem.Code != ProblemDevice {
			t.Fatalf("got state=%s problem=%+v, want Failed/ProblemDevice", sn.State, sn.Problem)
		}
	})

	t.Run("CaptureStarted.FellBack in fail mode aborts with ProblemDevice", func(t *testing.T) {
		spy := &captureSpy{fakeAudio: newFakeAudio(), fellBack: true, reason: "device busy"}
		m, ended := newMgr(t, spy, fakeDevices{fn: func(context.Context) ([]proto.CaptureDevice, error) {
			return sampleDevices(), nil
		}})
		if _, err := m.Start(StartRequest{
			Kind:           store.KindTranscribe,
			DeviceOverride: &DeviceOverride{Query: "usb-mic-uid", Fallback: DeviceFallbackFail},
		}); err != nil {
			t.Fatal(err)
		}
		sn := <-ended
		if sn.State != Failed || sn.Problem == nil || sn.Problem.Code != ProblemDevice {
			t.Fatalf("got state=%s problem=%+v, want Failed/ProblemDevice", sn.State, sn.Problem)
		}
	})

	t.Run("clipboard delivery copies without inserting or opening card", func(t *testing.T) {
		spy := &captureSpy{fakeAudio: newFakeAudio()}
		spy.maxFrame = 6
		var copied string
		ended := make(chan Snapshot, 1)
		m := New(Config{
			Audio: spy,
			Text:  &fakeText{},
			Copy:  func(text string) error { copied = text; return nil },
			Resolve: func(context.Context, Kind, settings.Settings) (Services, error) {
				return Services{ASR: &fakeFile{text: "copied text"}}, nil
			},
			Settings: func() settings.Settings { return s },
			OnChange: func(sn Snapshot) {
				if sn.State.Ended() {
					ended <- sn
				}
			},
		})
		if _, err := m.Start(StartRequest{Kind: store.KindTranscribe, Clipboard: true}); err != nil {
			t.Fatal(err)
		}
		sn := <-ended
		if sn.State != Done || sn.Inserted || sn.Card || copied != "copied text" {
			t.Fatalf("state=%s inserted=%v card=%v copied=%q", sn.State, sn.Inserted, sn.Card, copied)
		}
		_ = asr.SampleRate
	})
}
