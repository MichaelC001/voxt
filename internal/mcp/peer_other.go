//go:build !darwin && !linux

package mcp

import (
	"errors"
	"net"
)

// SystemInspector reports peer inspection as unsupported on other OSes.
func SystemInspector() Inspector { return otherInspector{} }

type otherInspector struct{}

var errNoPeers = errors.New("peer inspection not supported on this platform")

func (otherInspector) Peer(net.Conn) (Proc, error) { return Proc{}, errNoPeers }
func (otherInspector) Proc(int) (Proc, error)      { return Proc{}, errNoPeers }
func (otherInspector) Team(string) string          { return "" }
