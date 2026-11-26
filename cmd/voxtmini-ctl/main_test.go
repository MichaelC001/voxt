package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/ctlbridge"
)

func TestCLIHelpAndErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--help"}, t.TempDir(), "", nil, &out, &errOut); code != 0 {
		t.Fatalf("help exit=%d", code)
	}
	if !strings.Contains(out.String(), "voxtmini-ctl trigger") {
		t.Fatalf("help output: %q", out.String())
	}

	out.Reset()
	errOut.Reset()
	if code := run([]string{"bogus"}, t.TempDir(), "", nil, &out, &errOut); code != 2 {
		t.Fatalf("unknown cmd exit=%d, want 2", code)
	}

	out.Reset()
	errOut.Reset()
	if code := run([]string{"status", "--json"}, t.TempDir(), "", nil, &out, &errOut); code != 1 {
		t.Fatalf("disabled status exit=%d, want 1", code)
	}
	var resp ctlbridge.ControlResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil || resp.OK || resp.Error == "" {
		t.Fatalf("disabled status json: %q err=%v", out.String(), err)
	}
}

func TestCLITriggerDevicesStatus(t *testing.T) {
	home := t.TempDir()
	if err := ctlbridge.WriteState(home, ctlbridge.State{Trigger: true}); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", ctlbridge.ControlSocketPath(home, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				sc := bufio.NewScanner(c)
				if !sc.Scan() {
					return
				}
				var req ctlbridge.ControlRequest
				_ = json.Unmarshal(sc.Bytes(), &req)
				var resp ctlbridge.ControlResponse
				switch req.Method {
				case "devices":
					resp = ctlbridge.ControlResponse{OK: true, Devices: []ctlbridge.DeviceInfo{
						{ID: "usb-1", Name: "MyMic", Transport: "usb", Default: true},
					}}
				case "status":
					resp = ctlbridge.ControlResponse{OK: true, State: "idle"}
				case "trigger":
					resp = ctlbridge.ControlResponse{OK: true, SessionID: "s1", Feature: req.Feature, State: "recording"}
				}
				b, _ := json.Marshal(resp)
				_, _ = c.Write(append(b, '\n'))
			}(c)
		}
	}()

	var out, errOut bytes.Buffer
	if code := run([]string{"devices", "--json"}, home, "", nil, &out, &errOut); code != 0 {
		t.Fatalf("devices exit=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"usb-1"`) {
		t.Fatalf("devices json: %s", out.String())
	}

	out.Reset()
	if code := run([]string{"status"}, home, "", nil, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != "idle" {
		t.Fatalf("status exit=%d out=%q", code, out.String())
	}

	out.Reset()
	if code := run([]string{"trigger", "transcribe", "--action", "start", "--device", "usb-1", "--wait-device", "500", "--json"}, home, "", nil, &out, &errOut); code != 0 {
		t.Fatalf("trigger exit=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"recording"`) {
		t.Fatalf("trigger json: %s", out.String())
	}
}
