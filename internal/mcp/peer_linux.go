//go:build linux

package mcp

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// SystemInspector reads local Linux process metadata (used in tests).
func SystemInspector() Inspector { return linuxInspector{} }

type linuxInspector struct{}

func (linuxInspector) Peer(conn net.Conn) (Proc, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return Proc{}, errors.New("not a Unix socket")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return Proc{}, err
	}
	var cred *unix.Ucred
	var serr error
	if err := raw.Control(func(fd uintptr) {
		cred, serr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return Proc{}, err
	}
	if serr != nil {
		return Proc{}, serr
	}
	p, err := linuxInspector{}.Proc(int(cred.Pid))
	if err != nil {
		return Proc{}, err
	}
	p.UID = int(cred.Uid)
	return p, nil
}

func (linuxInspector) Proc(pid int) (Proc, error) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return Proc{}, err
	}
	s := string(stat)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return Proc{}, errors.New("bad /proc stat")
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 2 {
		return Proc{}, errors.New("bad /proc stat")
	}
	ppid, _ := strconv.Atoi(f[1])
	exe, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	uid := -1
	if st, err := os.Stat("/proc/" + strconv.Itoa(pid)); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			uid = int(sys.Uid)
		}
	}
	return Proc{PID: pid, PPID: ppid, UID: uid, Exe: exe}, nil
}

func (linuxInspector) Team(string) string { return "" }
