package ctlbridge

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const initLine = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"codex-mcp-client","version":"1"}}}` + "\n"

func pipeBridge(t *testing.T, o BridgeOptions) (io.WriteCloser, *bufio.Reader, chan int) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	o.In, o.Out = inR, outW
	code := make(chan int, 1)
	go func() { code <- RunMCPBridge(o); _ = outW.Close() }()
	t.Cleanup(func() { _ = inW.Close() })
	return inW, bufio.NewReader(outR), code
}

func TestWriteReadStatePermissions(t *testing.T) {
	home := t.TempDir()
	if err := WriteState(home, State{MCP: true, Trigger: false}); err != nil {
		t.Fatal(err)
	}
	st := ReadState(home)
	if !st.MCP || st.Trigger {
		t.Fatalf("got %+v", st)
	}
	dirInfo, err := os.Stat(filepath.Join(home, "run"))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("run dir mode = %o, want 700", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(StatePath(home))
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("mcp.json mode = %o, want 600", fileInfo.Mode().Perm())
	}
}

func TestSocketPathLength(t *testing.T) {
	long := "/Users/" + strings.Repeat("x", 90) + "/Library/Application Support/VoxtMini"
	if p := MCPSocketPath(long, "/c"); p != "/c/mcp.sock" {
		t.Errorf("mcp long = %s", p)
	}
	if p := ControlSocketPath(long, "/c"); p != "/c/control.sock" {
		t.Errorf("control long = %s", p)
	}
	if p := MCPSocketPath("/tmp/v", "/c"); p != "/tmp/v/run/mcp.sock" {
		t.Errorf("mcp short = %s", p)
	}
}

func TestLaunchArgs(t *testing.T) {
	env := []string{"HOME=/tmp/h", "OPENAI_API_KEY=sk-x", "VOXTMINI_DATA_HOME=/tmp/d", "ACTNOW_FAMILY_DIR=/tmp/f", "PATH=/bin"}
	gotApp := strings.Join(launchArgs(env, "/Applications/VoxtMini.app"), " ")
	wantApp := "-g --env HOME=/tmp/h --env VOXTMINI_DATA_HOME=/tmp/d --env ACTNOW_FAMILY_DIR=/tmp/f /Applications/VoxtMini.app"
	if gotApp != wantApp {
		t.Fatalf("with app: %q, want %q", gotApp, wantApp)
	}
	gotBundle := strings.Join(launchArgs(env, ""), " ")
	wantBundle := "-g --env HOME=/tmp/h --env VOXTMINI_DATA_HOME=/tmp/d --env ACTNOW_FAMILY_DIR=/tmp/f -b dev.hehehai.voxtmini"
	if gotBundle != wantBundle {
		t.Fatalf("without app: %q, want %q", gotBundle, wantBundle)
	}
}

func TestMCPBridgeOff(t *testing.T) {
	home := t.TempDir()
	launched := false
	w, r, code := pipeBridge(t, BridgeOptions{Home: home, Launch: func() error { launched = true; return nil }})
	_, _ = w.Write([]byte(initLine))
	line, _ := r.ReadString('\n')
	if !strings.Contains(line, "MCP server is off") || <-code != 3 || launched {
		t.Fatalf("line=%q launched=%v", line, launched)
	}
}

func TestMCPBridgeAutoLaunchAndSelfTest(t *testing.T) {
	home := t.TempDir()
	if err := WriteState(home, State{MCP: true}); err != nil {
		t.Fatal(err)
	}
	sock := MCPSocketPath(home, "")
	var ln net.Listener
	launched := false
	launch := func() error {
		launched = true
		var err error
		ln, err = net.Listen("unix", sock)
		if err != nil {
			return err
		}
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer c.Close()
					sc := bufio.NewScanner(c)
					if !sc.Scan() || sc.Text() != Hello {
						return
					}
					for sc.Scan() {
						var req struct {
							ID     json.RawMessage `json:"id"`
							Method string          `json:"method"`
						}
						if json.Unmarshal(sc.Bytes(), &req) != nil || len(req.ID) == 0 {
							continue
						}
						switch req.Method {
						case "initialize":
							_, _ = c.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25","capabilities":{}}}` + "\n"))
						case "tools/list":
							_, _ = c.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"a"},{"name":"b"}]}}` + "\n"))
						}
					}
				}(c)
			}
		}()
		return nil
	}
	t.Cleanup(func() {
		if ln != nil {
			_ = ln.Close()
		}
	})

	summary, err := SelfTest(BridgeOptions{Home: home, Launch: launch, Wait: 2 * time.Second})
	if err != nil {
		t.Fatalf("SelfTest: %v", err)
	}
	if !launched || !strings.Contains(summary, "2 tools") {
		t.Fatalf("launched=%v summary=%q", launched, summary)
	}
}

func TestCallControlOffAndConnected(t *testing.T) {
	home := t.TempDir()
	launched := false
	_, err := CallControl(ControlOptions{Home: home, Launch: func() error { launched = true; return nil }}, ControlRequest{Method: "status"})
	if err != ErrTriggerOff || launched {
		t.Fatalf("off: err=%v launched=%v", err, launched)
	}

	if err := WriteState(home, State{Trigger: true}); err != nil {
		t.Fatal(err)
	}
	sock := ControlSocketPath(home, "")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		sc := bufio.NewScanner(c)
		if sc.Scan() {
			var req ControlRequest
			_ = json.Unmarshal(sc.Bytes(), &req)
			b, _ := json.Marshal(ControlResponse{OK: true, State: "idle", Feature: req.Feature})
			_, _ = c.Write(append(b, '\n'))
		}
	}()

	resp, err := CallControl(ControlOptions{Home: home}, ControlRequest{Method: "status"})
	if err != nil || !resp.OK || resp.State != "idle" {
		t.Fatalf("connected: resp=%+v err=%v", resp, err)
	}
}
