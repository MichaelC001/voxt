package privnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
)

// DialFunc is an http.Transport's DialContext.
type DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error)

// Resolver looks a host name up (net.DefaultResolver; a fake in tests).
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type plainKey struct{}

// withPlain marks ctx as a plain request's: its connection is checked.
func withPlain(ctx context.Context) context.Context {
	return context.WithValue(ctx, plainKey{}, true)
}

func isPlain(ctx context.Context) bool {
	v, _ := ctx.Value(plainKey{}).(bool)
	return v
}

// Guard wraps t, the transport of clients that carry keys: its plain
// requests (http; a WebSocket's ws handshake is one too) go direct, never
// through a proxy, and connect only to the loopback or private
// addresses, a host name being resolved (with r; nil: net's) and refused
// unless every address it has is one. A private address that cannot be
// reached gives ErrLocalNetwork. https is unchanged otherwise.
func Guard(t *http.Transport, r Resolver) http.RoundTripper {
	if r == nil {
		r = net.DefaultResolver
	}
	dial := t.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	proxy := t.Proxy
	t.Proxy = func(req *http.Request) (*url.URL, error) {
		if req.URL.Scheme == "http" || Direct(req.URL.Hostname()) || proxy == nil {
			return nil, nil
		}
		return proxy(req)
	}
	t.DialContext = guardDial(dial, r)
	return guard{t}
}

type guard struct{ t *http.Transport }

func (g guard) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "http" {
		req = req.WithContext(withPlain(req.Context()))
	}
	return g.t.RoundTrip(req)
}

// CloseIdleConnections lets http.Client.CloseIdleConnections reach t.
func (g guard) CloseIdleConnections() { g.t.CloseIdleConnections() }

// guardDial is dial with the checks of Guard.
func guardDial(dial DialFunc, r Resolver) DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !isPlain(ctx) {
			return localNetwork(dial(ctx, network, addr))
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := resolve(ctx, r, host)
		if err != nil {
			return nil, err
		}
		// Every address must be local: one public answer is enough for
		// the name not to be trusted (a DNS answer is not the user's).
		for _, ip := range ips {
			if !Local(ip) {
				return nil, fmt.Errorf("%w: %s is %s", ErrPublic, host, ip)
			}
		}
		// Dial the checked addresses themselves, not the name again.
		var first error
		for _, ip := range ips {
			c, err := localNetwork(dial(ctx, network, net.JoinHostPort(ip.String(), port)))
			if err == nil {
				return c, nil
			}
			if first == nil {
				first = err
			}
			if ctx.Err() != nil {
				break
			}
		}
		return nil, first
	}
}

// resolve is host's addresses: itself when it is one.
func resolve(ctx context.Context, r Resolver, host string) ([]net.IP, error) {
	if ip := net.ParseIP(normal(host)); ip != nil {
		return []net.IP{ip}, nil
	}
	if IsLoopback(host) { // localhost: never asked of DNS
		return []net.IP{net.IPv4(127, 0, 0, 1)}, nil
	}
	addrs, err := r.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%s: no address", host)
	}
	ips := make([]net.IP, len(addrs))
	for i, a := range addrs {
		ips[i] = a.IP
	}
	return ips, nil
}

// localNetwork marks a failure to reach a private address as one macOS's
// Local Network privacy may cause.
func localNetwork(c net.Conn, err error) (net.Conn, error) {
	if err == nil || !Unreachable(err) {
		return c, err
	}
	var op *net.OpError
	if errors.As(err, &op) && op.Addr != nil {
		if ta, ok := op.Addr.(*net.TCPAddr); ok && !Private(ta.IP) {
			return nil, err
		}
	}
	return nil, &localNetErr{err}
}

// Unreachable reports whether err is "no route to host" or "network is
// unreachable".
func Unreachable(err error) bool {
	if errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "no route to host") || strings.Contains(s, "network is unreachable")
}

type localNetErr struct{ err error }

func (e *localNetErr) Error() string   { return e.err.Error() }
func (e *localNetErr) Unwrap() []error { return []error{ErrLocalNetwork, e.err} }
