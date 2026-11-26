package mcp

// Self-test for Settings: connects to VoxtMini's own MCP socket as a
// bridge, runs handshake, tools/list, and voxtmini_overview.

import (
	"context"
	"fmt"
	"net"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// SelfTestResult summarizes a successful connection self-test.
type SelfTestResult struct {
	Protocol string
	Tools    int
	Took     time.Duration
}

// SelfTest connects to path, sends the bridge handshake, negotiates MCP,
// lists tools, and calls voxtmini_overview.
func SelfTest(ctx context.Context, path, version string) (SelfTestResult, error) {
	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return SelfTestResult{}, fmt.Errorf("VoxtMini does not answer at %s: %w", path, err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(BridgeHelloLine + "\n")); err != nil {
		return SelfTestResult{}, err
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "voxtmini-self-test", Version: version}, nil)
	sess, err := client.Connect(ctx, &sdk.IOTransport{Reader: conn, Writer: conn}, nil)
	if err != nil {
		return SelfTestResult{}, err
	}
	defer sess.Close()
	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		return SelfTestResult{}, err
	}
	res, err := sess.CallTool(ctx, &sdk.CallToolParams{Name: "voxtmini_overview"})
	if err != nil {
		return SelfTestResult{}, err
	}
	if res.IsError {
		msg := ""
		if len(res.Content) > 0 {
			if t, ok := res.Content[0].(*sdk.TextContent); ok {
				msg = t.Text
			}
		}
		return SelfTestResult{}, fmt.Errorf("voxtmini_overview: %s", msg)
	}
	return SelfTestResult{
		Protocol: sess.InitializeResult().ProtocolVersion,
		Tools:    len(tools.Tools),
		Took:     time.Since(start),
	}, nil
}
