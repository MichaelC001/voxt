// Package agentsetup configures AI agent clients on macOS to connect to
// VoxtMini's local MCP server (via voxtmini-ctl mcp over stdio) and
// installs VoxtMini's embedded SKILL.md into standard skill directories.
//
// Only VoxtMini's own entry ("voxtmini") is touched; other servers and
// settings keep their order. A timestamped backup is written beside any
// existing configuration before modifying it. Files in an unsafe or
// ambiguous format are left untouched with ErrEditByHand.
package agentsetup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Supported MCP client identifiers.
const (
	Codex         = "codex"          // ChatGPT Codex mode and Codex CLI
	ClaudeCode    = "claude-code"    // Claude Code (~/.claude.json)
	ClaudeDesktop = "claude-desktop" // Claude Desktop
	Cursor        = "cursor"
	WorkBuddy     = "workbuddy"
)

// ServerName is VoxtMini's MCP server entry key.
const ServerName = "voxtmini"

const maxConfig = 4 << 20

// ErrEditByHand is returned when an existing config file cannot be
// modified safely (duplicate keys, inline tables, multiline TOML strings).
var ErrEditByHand = errors.New("configuration contains voxtmini in a form that cannot be edited safely; copy and edit by hand")

// Command describes how an MCP client launches VoxtMini's stdio bridge.
type Command struct {
	Path string
	Args []string
}

// DefaultCommand returns the standard voxtmini-ctl mcp launch command.
func DefaultCommand(ctlPath string) Command {
	return Command{Path: ctlPath, Args: []string{"mcp"}}
}

// ClientStatus reports detection and configuration state for one client.
type ClientStatus struct {
	ID         string
	Name       string
	ConfigPath string
	Installed  bool // client app or dotdir found on this machine
	Configured bool // config file already contains a voxtmini entry
}

// PreviewResult describes the effect of Write without touching disk.
type PreviewResult struct {
	Path    string
	Before  []byte
	After   []byte
	Diff    string
	Changes bool
}

// Result summarizes what Write did.
type Result struct {
	Path      string
	Backup    string // empty when creating a new file or unchanged
	Unchanged bool
}

// ConfigFile returns the MCP configuration file path for client under home.
func ConfigFile(home, client string) string {
	switch client {
	case Codex:
		return filepath.Join(home, ".codex", "config.toml")
	case ClaudeCode:
		return filepath.Join(home, ".claude.json")
	case ClaudeDesktop:
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case Cursor:
		return filepath.Join(home, ".cursor", "mcp.json")
	case WorkBuddy:
		return filepath.Join(home, ".workbuddy", "mcp.json")
	}
	return ""
}

var clientDefs = []struct {
	id, name string
	dirs     []string
	apps     []string
}{
	{Codex, "Codex / ChatGPT", []string{".codex"}, []string{"ChatGPT.app", "Codex.app"}},
	{ClaudeCode, "Claude Code", []string{".claude"}, nil},
	{ClaudeDesktop, "Claude Desktop", []string{filepath.Join("Library", "Application Support", "Claude")}, []string{"Claude.app"}},
	{Cursor, "Cursor", []string{".cursor"}, []string{"Cursor.app"}},
	{WorkBuddy, "WorkBuddy", []string{".workbuddy"}, []string{"WorkBuddy.app", "WorkBuddy AI.app"}},
}

// DetectClients inspects home and AppsDir for supported MCP clients.
func DetectClients(home string) []ClientStatus {
	out := make([]ClientStatus, 0, len(clientDefs))
	for _, c := range clientDefs {
		cfg := ConfigFile(home, c.id)
		st := ClientStatus{ID: c.id, Name: c.name, ConfigPath: cfg}
		if _, err := os.Stat(cfg); err == nil {
			st.Installed = true
			if b, err := readSmall(cfg); err == nil {
				st.Configured = hasServer(c.id, b)
			}
		}
		for _, d := range c.dirs {
			if _, err := os.Stat(filepath.Join(home, d)); err == nil {
				st.Installed = true
			}
		}
		for _, a := range c.apps {
			if _, err := os.Stat(filepath.Join(AppsDir, a)); err == nil {
				st.Installed = true
			}
		}
		out = append(out, st)
	}
	return out
}

func hasServer(client string, data []byte) bool {
	if client == Codex {
		return strings.Contains(string(data), tomlHeader)
	}
	top, err := members(data)
	if err != nil {
		return false
	}
	raw := get(top, "mcpServers")
	if raw == nil {
		return false
	}
	servers, err := members(raw)
	if err != nil {
		return false
	}
	return get(servers, ServerName) != nil
}

// Preview computes what Write would produce for client in home, including
// a readable line diff.
func Preview(home, client string, cmd Command) (PreviewResult, error) {
	path := ConfigFile(home, client)
	if path == "" {
		return PreviewResult{}, fmt.Errorf("unsupported client %q", client)
	}
	real := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		real = r
	}
	old, err := readSmall(real)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return PreviewResult{Path: path}, err
	}
	data, err := with(client, old, cmd)
	if err != nil {
		return PreviewResult{Path: path, Before: old}, err
	}
	if data == nil {
		return PreviewResult{Path: path, Before: old, After: old, Changes: false}, nil
	}
	return PreviewResult{
		Path:    path,
		Before:  old,
		After:   data,
		Diff:    lineDiff(old, data),
		Changes: true,
	}, nil
}

// Write backs up any existing config and safely upserts VoxtMini's entry.
func Write(home, client string, cmd Command) (Result, error) {
	path := ConfigFile(home, client)
	if path == "" {
		return Result{}, fmt.Errorf("unsupported client %q", client)
	}
	r := Result{Path: path}
	target := path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		target = real
	}
	old, err := readSmall(target)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return r, err
	}
	data, err := with(client, old, cmd)
	if err != nil {
		return r, err
	}
	if data == nil {
		r.Unchanged = true
		return r, nil
	}
	perm := os.FileMode(0o644)
	if exists {
		if fi, err := os.Stat(target); err == nil {
			perm = fi.Mode().Perm()
		}
		r.Backup = target + ".voxtmini-backup-" + time.Now().Format("20060102-150405")
		if err := writeFile(r.Backup, old, perm); err != nil {
			return r, fmt.Errorf("backup %s: %w", target, err)
		}
	} else if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return r, err
	}
	return r, writeFile(target, data, perm)
}

func with(client string, old []byte, cmd Command) ([]byte, error) {
	if client == Codex {
		return withTOML(old, cmd)
	}
	return withJSON(old, cmd)
}

func readSmall(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > maxConfig {
		return nil, fmt.Errorf("%s exceeds %d MB", path, maxConfig>>20)
	}
	return os.ReadFile(path)
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// lineDiff produces a compact unified-style preview of removed/added lines.
func lineDiff(before, after []byte) string {
	a := splitLines(string(before))
	b := splitLines(string(after))
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	var out strings.Builder
	for _, l := range a[prefix : len(a)-suffix] {
		out.WriteString("- " + l + "\n")
	}
	for _, l := range b[prefix : len(b)-suffix] {
		out.WriteString("+ " + l + "\n")
	}
	return out.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	return strings.Split(s, "\n")
}
