package omni

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/omni/omnitest"
)

// endpointOf is the endpoint of f's running server for model id.
func (f *fake) endpointOf(t *testing.T, id string) endpoint {
	t.Helper()
	f.rt.mu.Lock()
	defer f.rt.mu.Unlock()
	for _, s := range f.rt.servers {
		s.mu.Lock()
		p := s.proc
		s.mu.Unlock()
		if s.spec.ID == id && p != nil {
			return p.ep
		}
	}
	t.Fatalf("no server runs %s", id)
	return endpoint{}
}

// get is a GET of path on ep with header h.
func get(t *testing.T, ep endpoint, path string, h http.Header) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ep.base()+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = h
	resp, err := loopback.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// TestToken: each launch gets its own token on stdin, never in its
// arguments; every request (Final, VAD, sockets) carries it, and the
// server refuses requests without it or with an Origin.
func TestToken(t *testing.T) {
	t.Parallel()
	f := newFake(t, omnitest.Opts{}, nil)
	a := f.asr(t, ASRFast)
	if _, err := a.Transcribe(context.Background(), asr.FileRequest{Audio: tone(2*time.Second, 0.3), Format: "pcm"}); err != nil {
		t.Fatal(err)
	}
	v, err := NewVAD(f.rt, f.model(VADSilero))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Speech(context.Background(), tone(time.Second, 0.3), 0); err != nil {
		t.Fatal(err)
	}
	s, err := v.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Feed(tone(100*time.Millisecond, 0.3)); err != nil {
		t.Fatal(err)
	}
	s.Close()

	secret := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, l := range f.records("launch") {
		if i := slices.Index(l.Args, "--auth-token-fd"); i < 0 || l.Args[i+1] != "0" {
			t.Errorf("launch without --auth-token-fd 0: %v", l.Args)
		}
		if slices.ContainsFunc(l.Args, secret.MatchString) {
			t.Errorf("a token in the arguments: %v", l.Args)
		}
	}
	asrEP, vadEP := f.endpointOf(t, ASRFast), f.endpointOf(t, VADSilero)
	if asrEP.token == "" || asrEP.token == vadEP.token {
		t.Errorf("tokens %q and %q: want one per launch", asrEP.token, vadEP.token)
	}
	if got := get(t, asrEP, "/health", asrEP.header()); got != http.StatusOK {
		t.Errorf("with the token: %d", got)
	}
	if got := get(t, asrEP, "/health", http.Header{}); got != http.StatusUnauthorized {
		t.Errorf("without a token: %d", got)
	}
	if got := get(t, asrEP, "/health", vadEP.header()); got != http.StatusUnauthorized {
		t.Errorf("with another server's token: %d", got)
	}
	web := asrEP.header()
	web.Set("Origin", "https://example.com")
	if got := get(t, asrEP, "/health", web); got != http.StatusForbidden {
		t.Errorf("with an Origin: %d", got)
	}
}

// TestNoTokenServer: a runtime from before --auth-token-fd still runs,
// without a token, and the runtime says so.
func TestNoTokenServer(t *testing.T) {
	t.Parallel()
	f := newFake(t, omnitest.Opts{NoToken: true}, nil)
	a := f.asr(t, ASRFast)
	if _, err := a.Transcribe(context.Background(), asr.FileRequest{Audio: tone(2*time.Second, 0.3), Format: "pcm"}); err != nil {
		t.Fatal(err)
	}
	launches := f.records("launch")
	if len(launches) != 1 || slices.Contains(launches[0].Args, "--auth-token-fd") {
		t.Fatalf("launches %+v", launches)
	}
	if !slices.Contains(launches[0].Args, "--cache-limit-mb") {
		t.Errorf("--cache-limit-mb left out: %v", launches[0].Args)
	}
	if !f.logged("does not know --auth-token-fd") {
		t.Error("the older runtime is not logged")
	}
	if ep := f.endpointOf(t, ASRFast); ep.token != "" || strings.Contains(ep.header().Get("Authorization"), "Bearer") {
		t.Errorf("a token for a server that takes none: %q", ep.token)
	}
}
