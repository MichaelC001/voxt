// Package netcheck tells whether hosts answer through a client (the
// family's proxy): Settings → Diagnostics' self-check and the Network
// dialog's "Test connection" use it. A host answers when it sends any
// response below 500 to a HEAD request in time.
//
// Imports: nothing internal.
package netcheck

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Timeout is how long a host has to answer.
const Timeout = 6 * time.Second

// ErrTimeout is Result.Err when the host did not answer in time.
const ErrTimeout = "timeout"

// Result is one host's answer.
type Result struct {
	// URL is what was asked; Host its host, for the views.
	URL, Host string
	OK        bool
	// Latency is the time to the response's headers (OK only).
	Latency time.Duration
	// Err is why it did not answer: a short reason (ErrTimeout,
	// "connection refused", "502 Bad Gateway").
	Err string
}

// Reach sends a HEAD request to u through client, Timeout at most.
func Reach(ctx context.Context, client *http.Client, u string) Result {
	r := Result{URL: u, Host: hostOf(u)}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		r.Err = err.Error()
		return r
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		r.Err = reason(err)
		return r
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		r.Err = resp.Status
		return r
	}
	r.OK, r.Latency = true, time.Since(start)
	return r
}

// All reaches every URL at once; the results are in urls' order.
func All(ctx context.Context, client *http.Client, urls []string) []Result {
	out := make([]Result, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Go(func() { out[i] = Reach(ctx, client, u) })
	}
	wg.Wait()
	return out
}

// AnyOK says whether one of rs answered.
func AnyOK(rs []Result) bool {
	for _, r := range rs {
		if r.OK {
			return true
		}
	}
	return false
}

// reason is the end of a client's error: what went wrong, without the
// request's method and URL.
func reason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return ErrTimeout
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		msg = msg[i+2:]
	}
	return msg
}

// hostOf is u's host, u itself when it has none.
func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return u
}
