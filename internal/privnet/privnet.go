// Package privnet is the one rule for plain (unencrypted) connections to
// remote services (docs/remote-models.md): http:// and ws:// carry a key
// in clear, so they are allowed only to this Mac or a private network
// (a LAN, Tailscale). An address is checked when the endpoint is saved; a
// host name, which may resolve anywhere, is checked again when the
// connection is made (Guard): a name resolving to a public address never
// gets the key.
package privnet

import (
	"errors"
	"net"
	"strings"
)

// ErrPublic: a plain connection was refused, the host being (or resolving
// to) a public address.
var ErrPublic = errors.New("plain http only to this Mac or a private network")

// ErrLocalNetwork: a private address could not be reached ("no route to
// host"), which is what macOS's Local Network privacy looks like when
// VoxtMini is not allowed there.
var ErrLocalNetwork = errors.New("no route to a private address (Local Network permission?)")

// cgnat is 100.64.0.0/10: carrier-grade NAT, Tailscale's addresses.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0).To4(), Mask: net.CIDRMask(10, 32)}

// IsLoopback reports whether host is this machine: localhost (and
// *.localhost) or a loopback address.
func IsLoopback(host string) bool {
	host = normal(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Private reports whether ip is on a private network: RFC 1918
// (10/8, 172.16/12, 192.168/16), 100.64/10 (CGNAT, Tailscale),
// link-local (169.254/16, fe80::/10) or an IPv6 ULA (fc00::/7). The
// loopback is not (Local).
func Private(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip)
}

// Local reports whether ip is the loopback or a private address: where a
// plain connection may go.
func Local(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || Private(ip))
}

// PlainOK reports whether a plain endpoint (http, ws) may be saved for
// host: the loopback, a private address, or a host name (localhost,
// gpubox, *.local, *.ts.net…), whose addresses Guard checks at dial
// time. A public address, or 0.0.0.0, is not.
func PlainOK(host string) bool {
	host = normal(host)
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return Local(ip)
	}
	return true
}

// Direct reports whether a request to host bypasses the proxy: the
// loopback, a private address, a name ending in .local (mDNS) or a plain
// name without a dot (gpubox): a proxy would not reach them.
func Direct(host string) bool {
	host = normal(host)
	if IsLoopback(host) {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return Private(ip)
	}
	return host != "" && (strings.HasSuffix(host, ".local") || !strings.Contains(host, "."))
}

// normal is host without brackets, a zone or a trailing dot, lower case.
func normal(host string) string {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if i := strings.IndexByte(host, '%'); i >= 0 { // fe80::1%en0
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}
