package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hehehai/VoxtMini/internal/helper/proto"
)

// Device override fallback modes and wait bounds.
const (
	DeviceFallbackDefault = "default"
	DeviceFallbackFail    = "fail"
	MaxDeviceWaitMS       = 3000

	defaultDevicePollInterval = 150 * time.Millisecond
)

// Devices lists the available audio input devices (capture.devices).
type Devices interface {
	Devices(ctx context.Context) ([]proto.CaptureDevice, error)
}

// DeviceOverride selects an input device for one session without changing
// settings.Audio.Devices.
type DeviceOverride struct {
	// Query matches a device by exact UID, exact Name, case-insensitive
	// substring on Name, or Transport ("usb", "bluetooth", "builtin", …).
	Query string
	// Fallback is "default" (default when empty: fall back to the user's
	// configured devices) or "fail" (fail if the device is not available
	// or if capture falls back to another device).
	Fallback string
	// WaitMS polls briefly (capped at MaxDeviceWaitMS) for the device to
	// appear before starting capture.
	WaitMS int
}

// FailMode reports whether the override should fail instead of falling back.
func (d *DeviceOverride) FailMode() bool {
	return d != nil && strings.EqualFold(strings.TrimSpace(d.Fallback), DeviceFallbackFail)
}

func (c Config) devicePollInterval() time.Duration {
	if c.DevicePoll > 0 {
		return c.DevicePoll
	}
	return defaultDevicePollInterval
}

// resolveDeviceOverride resolves ov against lookup, returning the ordered
// device list for CaptureParams.Devices without mutating userDevices.
func resolveDeviceOverride(ctx context.Context, lookup Devices, ov *DeviceOverride, userDevices []string, poll time.Duration) ([]string, proto.CaptureDevice, error) {
	if ov == nil {
		return userDevices, proto.CaptureDevice{}, nil
	}
	q := strings.TrimSpace(ov.Query)
	if q == "" {
		return userDevices, proto.CaptureDevice{}, nil
	}
	failMode := ov.FailMode()
	if lookup == nil {
		if failMode {
			return nil, proto.CaptureDevice{}, &proto.Error{
				Code:    proto.CodeDeviceUnavailable,
				Message: fmt.Sprintf("microphone %q not available: device list unavailable", q),
			}
		}
		return prependDevice(q, userDevices), proto.CaptureDevice{}, nil
	}
	waitMS := min(max(ov.WaitMS, 0), MaxDeviceWaitMS)
	var deadline time.Time
	if waitMS > 0 {
		deadline = time.Now().Add(time.Duration(waitMS) * time.Millisecond)
	}
	if poll <= 0 {
		poll = defaultDevicePollInterval
	}
	for {
		devs, err := lookup.Devices(ctx)
		if err == nil {
			if m, ok := MatchDevice(devs, q); ok {
				if failMode {
					return []string{m.ID}, m, nil
				}
				return prependDevice(m.ID, userDevices), m, nil
			}
		}
		if waitMS <= 0 || time.Now().After(deadline) {
			if failMode {
				msg := fmt.Sprintf("microphone %q not found", q)
				if err != nil {
					msg = fmt.Sprintf("microphone %q not found: %v", q, err)
				}
				return nil, proto.CaptureDevice{}, &proto.Error{Code: proto.CodeDeviceUnavailable, Message: msg}
			}
			return append([]string(nil), userDevices...), proto.CaptureDevice{}, nil
		}
		rem := time.Until(deadline)
		sleep := min(poll, rem)
		if sleep <= 0 {
			continue
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, proto.CaptureDevice{}, ctx.Err()
		case <-timer.C:
		}
	}
}

// MatchDevice matches q against devs in priority order:
// exact UID -> exact Name -> case-insensitive exact Name -> case-insensitive
// Name substring -> Transport ("usb", "bluetooth", "builtin", ...).
func MatchDevice(devs []proto.CaptureDevice, q string) (proto.CaptureDevice, bool) {
	for _, d := range devs {
		if d.ID == q {
			return d, true
		}
	}
	for _, d := range devs {
		if d.Name == q {
			return d, true
		}
	}
	for _, d := range devs {
		if strings.EqualFold(d.Name, q) {
			return d, true
		}
	}
	ql := strings.ToLower(q)
	for _, d := range devs {
		if strings.Contains(strings.ToLower(d.Name), ql) {
			return d, true
		}
	}
	for _, d := range devs {
		if d.Transport != "" && strings.EqualFold(d.Transport, q) {
			return d, true
		}
	}
	return proto.CaptureDevice{}, false
}

// prependDevice returns a new slice with id first followed by rest (deduplicated).
func prependDevice(id string, rest []string) []string {
	out := make([]string, 0, 1+len(rest))
	out = append(out, id)
	for _, r := range rest {
		if r != id {
			out = append(out, r)
		}
	}
	return out
}
