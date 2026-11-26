// Package ctlbridge is the lightweight client and socket plumbing used by
// cmd/voxtmini-ctl and the main app (run/mcp.json, run/mcp.sock,
// run/control.sock). It imports only the Go standard library so that
// voxtmini-ctl stays a few MB with no UI or database dependencies.
package ctlbridge

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BundleID and AppName identify VoxtMini on macOS.
const (
	BundleID = "dev.hehehai.voxtmini"
	AppName  = "VoxtMini"
)

// State is written by the app to run/mcp.json (0600 in 0700 run/ dir)
// whenever MCP or Trigger settings change, so voxtmini-ctl can check
// enabled state without opening SQLite.
type State struct {
	MCP     bool `json:"mcp"`
	Trigger bool `json:"trigger"`
}

// DefaultDirs returns VoxtMini's default data directory and cache directory
// on the current platform. VOXTMINI_DATA_HOME / VOXTMINI_CACHE_DIR override
// them for tests.
func DefaultDirs() (home, cache string, err error) {
	if h := os.Getenv("VOXTMINI_DATA_HOME"); h != "" {
		return h, os.Getenv("VOXTMINI_CACHE_DIR"), nil
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(u, "Library", "Application Support", AppName),
			filepath.Join(u, "Library", "Caches", AppName), nil
	}
	dataRoot := os.Getenv("XDG_DATA_HOME")
	if dataRoot == "" {
		dataRoot = filepath.Join(u, ".local", "share")
	}
	cacheRoot := os.Getenv("XDG_CACHE_HOME")
	if cacheRoot == "" {
		cacheRoot = filepath.Join(u, ".cache")
	}
	return filepath.Join(dataRoot, AppName), filepath.Join(cacheRoot, AppName), nil
}

// MCPSocketPath is where the app's MCP server listens: run/mcp.sock in
// home, or in cache when that path is too long for a Unix socket (macOS:
// 104 bytes).
func MCPSocketPath(home, cache string) string {
	return socketPath(home, cache, "mcp.sock")
}

// ControlSocketPath is where the app's external trigger server listens:
// run/control.sock in home, or in cache when that path is too long.
func ControlSocketPath(home, cache string) string {
	return socketPath(home, cache, "control.sock")
}

func socketPath(home, cache, name string) string {
	p := filepath.Join(home, "run", name)
	if len(p) < 100 || cache == "" {
		return p
	}
	return filepath.Join(cache, name)
}

// StatePath is home/run/mcp.json.
func StatePath(home string) string {
	return filepath.Join(home, "run", "mcp.json")
}

// WriteState atomically writes st to home/run/mcp.json (0600 in 0700 run/).
func WriteState(home string, st State) error {
	dir := filepath.Join(home, "run")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := StatePath(home) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, StatePath(home))
}

// ReadState reads home/run/mcp.json; missing or unreadable returns zero State.
func ReadState(home string) State {
	var st State
	if b, err := os.ReadFile(StatePath(home)); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

// CtlPath returns the path to the bundled voxtmini-ctl binary inside
// resourcesDir, or the default installed bundle path when resourcesDir is empty.
func CtlPath(resourcesDir string) string {
	if resourcesDir != "" {
		return filepath.Join(resourcesDir, "ctl", "voxtmini-ctl")
	}
	return "/Applications/VoxtMini.app/Contents/Resources/ctl/voxtmini-ctl"
}

// AppOf returns the outermost .app bundle containing path ("" if none).
func AppOf(path string) string {
	i := strings.Index(path, ".app/")
	if i < 0 {
		if strings.HasSuffix(path, ".app") {
			return path
		}
		return ""
	}
	return path[:i+len(".app")]
}

// launchSelf opens VoxtMini in the background without bringing it to the
// front. When invoked from inside an .app bundle it opens that bundle;
// otherwise it falls back to `open -g -b dev.hehehai.voxtmini`.
func launchSelf() error {
	if runtime.GOOS != "darwin" {
		return errors.New("VoxtMini runs on macOS only")
	}
	exe, _ := os.Executable()
	app := AppOf(exe)
	return exec.Command("/usr/bin/open", launchArgs(os.Environ(), app)...).Run()
}

// launchArgs builds /usr/bin/open arguments: -g, safe env vars, and either
// the enclosing .app bundle or -b BundleID.
func launchArgs(env []string, app string) []string {
	args := []string{"-g"}
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if k == "HOME" || k == "ACTNOW_FAMILY_DIR" || strings.HasPrefix(k, "VOXTMINI_") {
			args = append(args, "--env", kv)
		}
	}
	if app != "" && strings.HasSuffix(app, ".app") {
		return append(args, app)
	}
	return append(args, "-b", BundleID)
}
