//go:build darwin

package mcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// SystemInspector reads local macOS process metadata.
func SystemInspector() Inspector { return &darwinInspector{teams: map[string]teamCache{}} }

type teamCache struct {
	id  string
	mod time.Time
}

type darwinInspector struct {
	mu    sync.Mutex
	teams map[string]teamCache
}

func (d *darwinInspector) Peer(conn net.Conn) (Proc, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return Proc{}, errors.New("not a Unix socket")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return Proc{}, err
	}
	var pid int
	var cred *unix.Xucred
	var serr error
	if err := raw.Control(func(fd uintptr) {
		pid, serr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if serr == nil {
			cred, serr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		}
	}); err != nil {
		return Proc{}, err
	}
	if serr != nil {
		return Proc{}, serr
	}
	p, err := d.Proc(pid)
	if err != nil {
		return Proc{}, err
	}
	p.UID = int(cred.Uid)
	return p, nil
}

func (d *darwinInspector) Proc(pid int) (Proc, error) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return Proc{}, err
	}
	if k.Proc.P_pid != int32(pid) {
		return Proc{}, errors.New("no such process")
	}
	return Proc{
		PID:  pid,
		PPID: int(k.Eproc.Ppid),
		UID:  int(k.Eproc.Ucred.Uid),
		Exe:  procExe(pid),
	}, nil
}

func procExe(pid int) string {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(buf) < 5 {
		return ""
	}
	_ = binary.LittleEndian.Uint32(buf)
	rest := buf[4:]
	if i := bytes.IndexByte(rest, 0); i > 0 {
		return string(rest[:i])
	}
	return ""
}

func (d *darwinInspector) Team(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	d.mu.Lock()
	t, ok := d.teams[path]
	d.mu.Unlock()
	if ok && t.mod.Equal(fi.ModTime()) {
		return t.id
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "/usr/bin/codesign", "-dv", "--verbose=2", path).CombinedOutput()
	id := teamOf(string(out))
	d.mu.Lock()
	d.teams[path] = teamCache{id: id, mod: fi.ModTime()}
	d.mu.Unlock()
	return id
}

func teamOf(out string) string {
	for line := range strings.Lines(out) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "TeamIdentifier="); ok {
			if v == "not set" {
				return ""
			}
			return clean(v)
		}
	}
	return ""
}
