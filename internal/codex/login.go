package codex

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

// CallbackError is the issuer's refusal sent to the callback (the user
// declined, or the account may not use Codex).
type CallbackError struct {
	Code        string // e.g. "access_denied"
	Description string
}

func (e *CallbackError) Error() string {
	if e.Description != "" {
		return "codex: sign-in refused: " + e.Code + ": " + e.Description
	}
	return "codex: sign-in refused: " + e.Code
}

// Login signs in in the browser: it listens on 127.0.0.1, calls open with
// the authorize URL (the app opens the browser), waits for the issuer to
// redirect back, exchanges the code and answers the browser with a short
// page. Cancelling ctx ends it; the listener is always closed on return.
func Login(ctx context.Context, cfg Config, open func(url string) error) (Tokens, error) {
	cfg = cfg.withDefaults()
	ln, err := listen(cfg.Port)
	if err != nil {
		return Tokens{}, err
	}
	defer ln.Close()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/auth/callback", ln.Addr().(*net.TCPAddr).Port)
	state, verifier := randomString(32), randomString(64)

	type result struct {
		t   Tokens
		err error
	}
	done := make(chan result, 1)
	var once sync.Once
	finish := func(t Tokens, err error) { once.Do(func() { done <- result{t, err} }) }
	var claimed sync.Mutex // one exchange at a time; later callbacks see it finished
	finished := false

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			// A stale tab or someone else's request: refuse it, keep waiting.
			page(w, http.StatusBadRequest, "Sign-in failed", "This sign-in link is out of date. Start again from VoxtMini.")
			return
		}
		claimed.Lock()
		defer claimed.Unlock()
		if finished {
			page(w, http.StatusOK, "Signed in", "You can close this tab.")
			return
		}
		if code := q.Get("error"); code != "" {
			finished = true
			page(w, http.StatusOK, "Sign-in failed", "You can close this tab and try again in VoxtMini.")
			finish(Tokens{}, &CallbackError{Code: code, Description: q.Get("error_description")})
			return
		}
		code := q.Get("code")
		if code == "" {
			page(w, http.StatusBadRequest, "Sign-in failed", "The sign-in answer had no code. Try again in VoxtMini.")
			return
		}
		t, err := exchange(r.Context(), ctx, cfg, code, redirect, verifier)
		finished = true
		if err != nil {
			page(w, http.StatusOK, "Sign-in failed", "You can close this tab and try again in VoxtMini.")
		} else {
			page(w, http.StatusOK, "Signed in to VoxtMini", "You can close this tab.")
		}
		finish(t, err)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx) // lets the last page reach the browser
	}()

	if err := open(authorizeURL(cfg, redirect, state, verifier)); err != nil {
		return Tokens{}, fmt.Errorf("codex: opening the browser: %w", err)
	}
	select {
	case res := <-done:
		return res.t, res.err
	case <-ctx.Done():
		return Tokens{}, ctx.Err()
	}
}

// listen takes the registered port, or the fallback one, on 127.0.0.1.
func listen(port int) (net.Listener, error) {
	ports := []int{port}
	switch port {
	case 0:
		ports = []int{DefaultPort, FallbackPort}
	case AnyPort:
		ports = []int{0}
	}
	var err error
	for _, p := range ports {
		var ln net.Listener
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, nil
		}
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return nil, fmt.Errorf("%w (%s)", ErrPortInUse, portList(ports))
	}
	return nil, fmt.Errorf("codex: listening for the sign-in callback: %w", err)
}

func portList(ports []int) string {
	s := make([]string, len(ports))
	for i, p := range ports {
		s[i] = fmt.Sprint(p)
	}
	return strings.Join(s, ", ")
}

func authorizeURL(cfg Config, redirect, state, verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"response_type":              {"code"},
		"client_id":                  {cfg.ClientID},
		"redirect_uri":               {redirect},
		"scope":                      {scope},
		"code_challenge":             {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method":      {"S256"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"state":                      {state},
		"originator":                 {originator},
	}
	return cfg.Issuer + "/oauth/authorize?" + q.Encode()
}

// exchange trades the code for tokens. It stops when either the browser's
// request or the login is cancelled.
func exchange(reqCtx, loginCtx context.Context, cfg Config, code, redirect, verifier string) (Tokens, error) {
	ctx, cancel := context.WithCancel(reqCtx)
	defer cancel()
	stop := context.AfterFunc(loginCtx, cancel)
	defer stop()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"code":          {code},
		"redirect_uri":  {redirect},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Issuer+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("codex: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := tokenCall(cfg, req)
	if err != nil {
		var rej *rejection
		if errors.As(err, &rej) {
			return Tokens{}, fmt.Errorf("codex: code exchange refused: %w", err)
		}
		return Tokens{}, err
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		return Tokens{}, fmt.Errorf("%w: token answer without tokens", ErrUnavailable)
	}
	t := Tokens{IDToken: resp.IDToken, AccessToken: resp.AccessToken, RefreshToken: resp.RefreshToken, LastRefresh: cfg.Now()}
	t.fill()
	return t, nil
}

type tokenAnswer struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// rejection is a non-2xx answer of the token endpoint: its status and
// OAuth error code only, never the body (it may echo a token).
type rejection struct {
	Status int
	Code   string
}

func (r *rejection) Error() string {
	if r.Code != "" {
		return fmt.Sprintf("HTTP %d (%s)", r.Status, r.Code)
	}
	return fmt.Sprintf("HTTP %d", r.Status)
}

// tokenCall posts to the token endpoint. Transport failures and unusable
// answers wrap ErrUnavailable; a refusal is a *rejection.
func tokenCall(cfg Config, req *http.Request) (tokenAnswer, error) {
	resp, err := cfg.HTTP.Do(req)
	if err != nil {
		if req.Context().Err() != nil {
			return tokenAnswer{}, req.Context().Err()
		}
		return tokenAnswer{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return tokenAnswer{}, fmt.Errorf("%w: reading the answer: %w", ErrUnavailable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return tokenAnswer{}, &rejection{Status: resp.StatusCode, Code: errorCode(body)}
	}
	var a tokenAnswer
	if json.Unmarshal(body, &a) != nil {
		return tokenAnswer{}, fmt.Errorf("%w: unreadable token answer", ErrUnavailable)
	}
	return a, nil
}

// errorCode finds the OAuth error code in an error body, as codex-rs does:
// "error" as a string, "error.code", or a top-level "code". Codes are
// short identifiers; anything else is dropped.
func errorCode(body []byte) string {
	var v struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
	}
	if json.Unmarshal(body, &v) != nil {
		return ""
	}
	var code string
	if json.Unmarshal(v.Error, &code) != nil || code == "" {
		var nested struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(v.Error, &nested)
		code = nested.Code
	}
	if code == "" {
		code = v.Code
	}
	if len(code) > 64 || strings.ContainsAny(code, " \t\r\n\"<>") {
		return ""
	}
	return code
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // never fails (crypto/rand)
	return base64.RawURLEncoding.EncodeToString(b)
}

func page(w http.ResponseWriter, status int, title, text string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>%s</title></head>`+
		`<body style="font-family:-apple-system,sans-serif;text-align:center;margin-top:20vh">`+
		`<h2>%s</h2><p>%s</p></body></html>`, title, title, text)
}
