package ctlbridge

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrTriggerOff is returned when external triggers are disabled in settings.
var ErrTriggerOff = errors.New("VoxtMini's external trigger is off: VoxtMini → Settings → Integrations → External trigger")

// ControlRequest is one newline-delimited JSON request over run/control.sock.
type ControlRequest struct {
	Method         string `json:"method"` // "trigger" | "devices" | "status"
	Feature        string `json:"feature,omitempty"`
	Action         string `json:"action,omitempty"`
	Device         string `json:"device,omitempty"`
	Fallback       string `json:"fallback,omitempty"`
	WaitDeviceMS   int    `json:"wait_device_ms,omitempty"`
	WaitDeviceSet  bool   `json:"wait_device_set,omitempty"`
	TargetLanguage string `json:"to,omitempty"`
	Deliver        string `json:"deliver,omitempty"`
	Source         string `json:"source,omitempty"`
	Wait           bool   `json:"wait,omitempty"`
}

// DeviceInfo describes an audio input device in control responses.
type DeviceInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Channels   int    `json:"channels,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	Default    bool   `json:"default,omitempty"`
	Transport  string `json:"transport,omitempty"`
	FellBack   bool   `json:"fell_back,omitempty"`
}

// ControlResponse is one newline-delimited JSON response over run/control.sock.
type ControlResponse struct {
	OK        bool         `json:"ok"`
	Error     string       `json:"error,omitempty"`
	SessionID string       `json:"session_id,omitempty"`
	Feature   string       `json:"feature,omitempty"`
	State     string       `json:"state,omitempty"`
	Text      string       `json:"text,omitempty"`
	Delivered bool         `json:"delivered,omitempty"`
	Debounced bool         `json:"debounced,omitempty"`
	Device    *DeviceInfo  `json:"device,omitempty"`
	Devices   []DeviceInfo `json:"devices,omitempty"`
}

// ControlOptions configures CallControl.
type ControlOptions struct {
	Home, Cache string
	Launch      func() error
	// Wait is how long to wait for the app to start (0: 15s).
	Wait time.Duration
	// Timeout is the per-request I/O deadline (0: 15s, or 15m when req.Wait is true).
	Timeout time.Duration
}

// CallControl sends req to run/control.sock and decodes the response.
// If the socket is absent and run/mcp.json says Trigger is enabled, it
// auto-launches VoxtMini in the background and waits for the socket.
func CallControl(o ControlOptions, req ControlRequest) (ControlResponse, error) {
	if o.Wait == 0 {
		o.Wait = 15 * time.Second
	}
	if o.Launch == nil {
		o.Launch = launchSelf
	}
	timeout := o.Timeout
	if timeout == 0 {
		if req.Wait {
			timeout = 15 * time.Minute
		} else {
			timeout = 15 * time.Second
		}
	}
	path := ControlSocketPath(o.Home, o.Cache)
	conn, err := net.Dial("unix", path)
	if err != nil {
		if !ReadState(o.Home).Trigger {
			return ControlResponse{}, ErrTriggerOff
		}
		if conn, err = startAndDial(o.Launch, o.Wait, path); err != nil {
			return ControlResponse{}, err
		}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	b, err := json.Marshal(req)
	if err != nil {
		return ControlResponse{}, err
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return ControlResponse{}, err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return ControlResponse{}, fmt.Errorf("reading control response: %w", err)
		}
		return ControlResponse{}, errors.New("VoxtMini closed the control connection without responding")
	}
	var resp ControlResponse
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return ControlResponse{}, fmt.Errorf("invalid control response: %w", err)
	}
	return resp, nil
}
