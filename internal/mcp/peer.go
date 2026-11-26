package mcp

// Peer identification via kernel socket credentials (never trusting
// client-provided identity strings alone).

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// Proc describes a local OS process.
type Proc struct {
	PID, PPID int
	UID       int
	Exe       string // executable path ("" if unknown)
}

// Inspector reads local process metadata (implemented per OS in peer_*.go).
type Inspector interface {
	// Peer returns the process at the other end of a Unix socket.
	Peer(conn net.Conn) (Proc, error)
	// Proc returns the process for pid.
	Proc(pid int) (Proc, error)
	// Team returns the code-signing Team ID for path ("" if none).
	Team(path string) string
}

// Client is the verified program that spawned the bridge.
type Client struct {
	// Key uniquely identifies the client by app/binary path and signer.
	Key string
	// App is the enclosing .app bundle name ("ChatGPT"; "" if CLI binary).
	// Path is the .app path (or executable path), Exe the executable,
	// Team the signer Team ID.
	App, Path, Exe, Team string
	// Name is the self-reported MCP clientInfo.name (sanitized).
	Name string
	// Self is true for VoxtMini's own self-test connection.
	Self bool
}

// Display formats the client for user-visible prompts and audit records.
func (c Client) Display() string {
	if c.Self {
		return "VoxtMini"
	}
	base := c.App
	if base == "" {
		base = filepath.Base(c.Exe)
	}
	if n := c.Name; n != "" && !strings.EqualFold(n, base) && !strings.EqualFold(n, "voxtmini") {
		return base + " (" + n + ")"
	}
	return base
}

var (
	errNotBridge = errors.New("connect through VoxtMini's bridge (VoxtMini Settings shows the command)")
	errOtherUser = errors.New("connection from another user's process")
)

// identify verifies that conn's peer is one of VoxtMini's bridge binaries
// running as uid, then inspects its parent process to identify the agent.
func identify(ins Inspector, conn net.Conn, bridges []string, uid int) (Client, error) {
	peer, err := ins.Peer(conn)
	if err != nil {
		return Client{}, err
	}
	if peer.UID != uid {
		return Client{}, errOtherUser
	}
	if !slices.ContainsFunc(bridges, func(b string) bool { return sameFile(peer.Exe, b) }) {
		return Client{}, errNotBridge
	}
	if peer.PID == os.Getpid() {
		return Client{Key: "voxtmini:self", Exe: peer.Exe, Self: true}, nil
	}
	parent, err := ins.Proc(peer.PPID)
	if err != nil || parent.Exe == "" {
		return Client{}, errors.New("the program that started the bridge is gone")
	}
	c := Client{Exe: parent.Exe, Path: parent.Exe}
	if app := appOf(parent.Exe); app != "" {
		c.Path = app
		c.App = strings.TrimSuffix(filepath.Base(app), ".app")
	}
	c.Team = ins.Team(c.Path)
	c.Key = c.Path + "|" + c.Team
	return c, nil
}

// appOf returns the enclosing .app bundle path if path is inside one.
func appOf(path string) string {
	cleanPath := filepath.Clean(path)
	parts := strings.Split(cleanPath, string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.HasSuffix(parts[i], ".app") {
			joined := filepath.Join(parts[:i+1]...)
			if filepath.IsAbs(cleanPath) {
				return string(filepath.Separator) + joined
			}
			return joined
		}
	}
	return ""
}

func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// clean sanitizes an untrusted client name: single line, no control or
// format characters, capped at 48 runes.
func clean(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= 48 {
			b.WriteString("…")
			break
		}
		switch {
		case r == '\n' || r == '\t' || r == '\r':
			r = ' '
		case unicode.Is(unicode.Cf, r) || unicode.IsControl(r):
			continue
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}
