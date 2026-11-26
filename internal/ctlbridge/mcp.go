package ctlbridge

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// Hello is the first line the MCP bridge writes to mcp.sock so the app
// recognises a bridged stdio client.
const Hello = `{"bridge":1}`

const maxLine = 1 << 20

// ErrMCPOff is returned when MCP is disabled in settings.
var ErrMCPOff = errors.New("VoxtMini's MCP server is off: VoxtMini → Settings → Integrations → MCP")

// BridgeOptions configures RunMCPBridge and SelfTest.
type BridgeOptions struct {
	Home, Cache string
	// Launch starts VoxtMini in the background (nil: launchSelf).
	Launch func() error
	// Wait is how long to wait for the socket after launching (0: 45s).
	Wait time.Duration
	In   io.Reader
	Out  io.Writer
}

// RunMCPBridge serves one MCP stdio client over run/mcp.sock and returns
// the process exit code (0 ok, 3 disabled, 4 connection/launch error).
func RunMCPBridge(o BridgeOptions) int {
	if o.Wait == 0 {
		o.Wait = 45 * time.Second
	}
	if o.Launch == nil {
		o.Launch = launchSelf
	}
	path := MCPSocketPath(o.Home, o.Cache)
	conn, err := net.Dial("unix", path)
	if err != nil {
		if !ReadState(o.Home).MCP {
			answerAll(o.In, o.Out, ErrMCPOff)
			return 3
		}
		if conn, err = startAndDial(o.Launch, o.Wait, path); err != nil {
			answerAll(o.In, o.Out, err)
			return 4
		}
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(Hello + "\n")); err != nil {
		answerAll(o.In, o.Out, err)
		return 4
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(o.Out, conn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, o.In)
		if uc, ok := conn.(*net.UnixConn); ok {
			_ = uc.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	return 0
}

// SelfTest connects to mcp.sock, performs an MCP initialize + tools/list
// handshake, and returns a short summary (elapsed time and tool count).
func SelfTest(o BridgeOptions) (string, error) {
	if o.Wait == 0 {
		o.Wait = 15 * time.Second
	}
	if o.Launch == nil {
		o.Launch = launchSelf
	}
	start := time.Now()
	path := MCPSocketPath(o.Home, o.Cache)
	conn, err := net.Dial("unix", path)
	if err != nil {
		if !ReadState(o.Home).MCP {
			return "", ErrMCPOff
		}
		if conn, err = startAndDial(o.Launch, o.Wait, path); err != nil {
			return "", err
		}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write([]byte(Hello + "\n")); err != nil {
		return "", err
	}
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"voxtmini-ctl","version":"1"}}}` + "\n"
	if _, err := conn.Write([]byte(initReq)); err != nil {
		return "", err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", fmt.Errorf("mcp initialize failed: %w", err)
		}
		return "", errors.New("mcp server closed connection before initialize response")
	}
	var initResp struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(sc.Bytes(), &initResp); err != nil {
		return "", fmt.Errorf("invalid initialize response: %w", err)
	}
	if initResp.Error != nil {
		return "", fmt.Errorf("mcp error (%d): %s", initResp.Error.Code, initResp.Error.Message)
	}
	_, _ = conn.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
	if _, err := conn.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n")); err != nil {
		return "", err
	}
	if !sc.Scan() {
		return "", errors.New("mcp server closed connection before tools/list response")
	}
	var toolsResp struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(sc.Bytes(), &toolsResp); err != nil {
		return "", fmt.Errorf("invalid tools/list response: %w", err)
	}
	if toolsResp.Error != nil {
		return "", fmt.Errorf("tools/list error (%d): %s", toolsResp.Error.Code, toolsResp.Error.Message)
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	return fmt.Sprintf("ok: %d tools (%s)", len(toolsResp.Result.Tools), elapsed), nil
}

func startAndDial(launch func() error, wait time.Duration, path string) (net.Conn, error) {
	if err := launch(); err != nil {
		return nil, fmt.Errorf("VoxtMini is not running and could not be started: %v", err)
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("unix", path); err == nil {
			return c, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, errors.New("VoxtMini did not start in time: open VoxtMini and try again")
}

// answerAll answers the client's first JSON-RPC request with err so the
// client displays a clear reason.
func answerAll(in io.Reader, out io.Writer, err error) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || len(req.ID) == 0 {
			continue // notification
		}
		b, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error":   map[string]any{"code": -32603, "message": err.Error()},
		})
		_, _ = out.Write(append(b, '\n'))
		return
	}
}
