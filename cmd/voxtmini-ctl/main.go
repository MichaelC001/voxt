// Command voxtmini-ctl is the lightweight helper CLI and MCP stdio bridge
// bundled inside VoxtMini.app/Contents/Resources/ctl/voxtmini-ctl. It
// imports only the Go standard library and internal/ctlbridge (no UI,
// MyGo, or SQLite) so each process stays a few MB.
//
// Usage:
//
//	voxtmini-ctl mcp [--self-test]
//	voxtmini-ctl trigger <transcribe|translate|rewrite|stop|cancel> [flags]
//	voxtmini-ctl devices [--json]
//	voxtmini-ctl status [--json]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hehehai/VoxtMini/internal/ctlbridge"
)

func main() {
	home, cache, err := ctlbridge.DefaultDirs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "voxtmini-ctl: %v\n", err)
		os.Exit(1)
	}
	os.Exit(run(os.Args[1:], home, cache, os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, home, cache string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(out)
		return 0
	}
	switch args[0] {
	case "mcp":
		return runMCP(args[1:], home, cache, in, out, errOut)
	case "trigger":
		return runTrigger(args[1:], home, cache, out, errOut)
	case "devices":
		return runDevices(args[1:], home, cache, out, errOut)
	case "status":
		return runStatus(args[1:], home, cache, out, errOut)
	default:
		fmt.Fprintf(errOut, "voxtmini-ctl: unknown command %q\n", args[0])
		printUsage(errOut)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  voxtmini-ctl mcp [--self-test]
  voxtmini-ctl trigger <transcribe|translate|rewrite|stop|cancel> [flags]
  voxtmini-ctl devices [--json]
  voxtmini-ctl status [--json]

Trigger flags:
  --action <toggle|start|stop|cancel>
  --device <uid|name|transport>
  --fallback <default|fail>
  --wait-device <ms>
  --to <lang>
  --deliver <cursor|clipboard|card>
  --source <name>
  --wait
  --json`)
}

func runMCP(args []string, home, cache string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(errOut)
	selfTest := fs.Bool("self-test", false, "run an MCP initialize + tools/list self-test")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := ctlbridge.BridgeOptions{Home: home, Cache: cache, In: in, Out: out}
	if *selfTest {
		summary, err := ctlbridge.SelfTest(opts)
		if err != nil {
			fmt.Fprintf(errOut, "voxtmini-ctl: %v\n", err)
			return 1
		}
		fmt.Fprintln(out, summary)
		return 0
	}
	return ctlbridge.RunMCPBridge(opts)
}

func runTrigger(args []string, home, cache string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printUsage(out)
		return 0
	}
	target := strings.ToLower(strings.TrimSpace(args[0]))
	fs := flag.NewFlagSet("trigger", flag.ContinueOnError)
	fs.SetOutput(errOut)
	action := fs.String("action", "", "toggle, start, stop, or cancel")
	device := fs.String("device", "", "microphone UID, name, or transport")
	fallback := fs.String("fallback", "", "default or fail")
	waitDev := fs.Int("wait-device", -1, "ms to wait for device to appear (0-3000)")
	toLang := fs.String("to", "", "target language for translate")
	deliver := fs.String("deliver", "", "cursor, clipboard, or card")
	source := fs.String("source", "voxtmini-ctl", "caller identifier")
	wait := fs.Bool("wait", false, "wait until the session ends")
	asJSON := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	req := ctlbridge.ControlRequest{
		Method:         "trigger",
		Device:         *device,
		Fallback:       *fallback,
		TargetLanguage: *toLang,
		Deliver:        *deliver,
		Source:         *source,
		Wait:           *wait,
	}
	if *waitDev >= 0 {
		req.WaitDeviceMS = *waitDev
		req.WaitDeviceSet = true
	}
	switch target {
	case "transcribe", "translate", "rewrite":
		req.Feature = target
		req.Action = *action
		if req.Action == "" {
			req.Action = "toggle"
		}
	case "stop":
		req.Action = "stop"
	case "cancel":
		req.Action = "cancel"
	default:
		fmt.Fprintf(errOut, "voxtmini-ctl: unknown trigger target %q (expected transcribe, translate, rewrite, stop, or cancel)\n", target)
		return 2
	}

	resp, err := ctlbridge.CallControl(ctlbridge.ControlOptions{Home: home, Cache: cache}, req)
	if err != nil {
		return emitError(out, errOut, *asJSON, err)
	}
	if *asJSON {
		writeJSON(out, resp)
		if !resp.OK {
			return 1
		}
		return 0
	}
	if !resp.OK {
		fmt.Fprintf(errOut, "voxtmini-ctl: %s\n", resp.Error)
		return 1
	}
	if resp.Text != "" {
		fmt.Fprintln(out, resp.Text)
		return 0
	}
	fmt.Fprintf(out, "%s (%s)\n", resp.State, resp.SessionID)
	return 0
}

func runDevices(args []string, home, cache string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("devices", flag.ContinueOnError)
	fs.SetOutput(errOut)
	asJSON := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	resp, err := ctlbridge.CallControl(ctlbridge.ControlOptions{Home: home, Cache: cache}, ctlbridge.ControlRequest{Method: "devices"})
	if err != nil {
		return emitError(out, errOut, *asJSON, err)
	}
	if *asJSON {
		writeJSON(out, resp)
		if !resp.OK {
			return 1
		}
		return 0
	}
	if !resp.OK {
		fmt.Fprintf(errOut, "voxtmini-ctl: %s\n", resp.Error)
		return 1
	}
	for _, d := range resp.Devices {
		def := ""
		if d.Default {
			def = " [default]"
		}
		tr := ""
		if d.Transport != "" {
			tr = " (" + d.Transport + ")"
		}
		fmt.Fprintf(out, "%s\t%s%s%s\n", d.ID, d.Name, tr, def)
	}
	return 0
}

func runStatus(args []string, home, cache string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(errOut)
	asJSON := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	resp, err := ctlbridge.CallControl(ctlbridge.ControlOptions{Home: home, Cache: cache}, ctlbridge.ControlRequest{Method: "status"})
	if err != nil {
		return emitError(out, errOut, *asJSON, err)
	}
	if *asJSON {
		writeJSON(out, resp)
		if !resp.OK {
			return 1
		}
		return 0
	}
	if !resp.OK {
		fmt.Fprintf(errOut, "voxtmini-ctl: %s\n", resp.Error)
		return 1
	}
	if resp.Device != nil && resp.Device.Name != "" {
		fmt.Fprintf(out, "%s (%s)\n", resp.State, resp.Device.Name)
		return 0
	}
	fmt.Fprintln(out, resp.State)
	return 0
}

func emitError(out, errOut io.Writer, asJSON bool, err error) int {
	if asJSON {
		writeJSON(out, ctlbridge.ControlResponse{OK: false, Error: err.Error()})
		return 1
	}
	fmt.Fprintf(errOut, "voxtmini-ctl: %v\n", err)
	return 1
}

func writeJSON(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	_, _ = w.Write(append(b, '\n'))
}
