package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// When Source refreshes, as codex-rs: the access token expires within
// refreshWindow, or (a token without exp) the last refresh is older than
// refreshAge.
const (
	refreshWindow = 5 * time.Minute
	refreshAge    = 8 * 24 * time.Hour
)

// Store is where the sign-in lives: the app's Keychain item. Get returns
// "" when there is none; Set("") deletes it. Both may block.
type Store interface {
	Get() (string, error)
	Set(string) error
}

// Source hands out the headers of requests made with the sign-in, and
// refreshes it when due. It is safe for concurrent use; one refresh runs
// at a time and the others wait for its result. Its methods block (Store
// and network): call them off the main thread.
type Source struct {
	store Store
	cfg   Config
	sem   chan struct{} // held while reading, refreshing or saving

	loaded   bool
	cur      Tokens
	unsaved  bool   // cur was refreshed but Store.Set failed: retried
	rejected string // a refresh token the issuer refused for good
}

// NewSource reads the sign-in from store on first use.
func NewSource(store Store, cfg Config) *Source {
	return &Source{store: store, cfg: cfg.withDefaults(), sem: make(chan struct{}, 1)}
}

func (s *Source) lock(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Source) unlock() { <-s.sem }

// load reads the store once (locked).
func (s *Source) load() error {
	if s.loaded {
		return nil
	}
	raw, err := s.store.Get()
	if err != nil {
		return fmt.Errorf("codex: reading the sign-in: %w", err)
	}
	s.cur = Tokens{}
	if raw != "" {
		t, err := Parse(raw)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrSignedOut, err)
		}
		t.fill()
		s.cur = t
	}
	s.loaded = true
	return nil
}

// Current is the sign-in as stored (for the UI: Email, AccountID), without
// refreshing it; the zero Tokens when signed out.
func (s *Source) Current(ctx context.Context) (Tokens, error) {
	if err := s.lock(ctx); err != nil {
		return Tokens{}, err
	}
	defer s.unlock()
	if err := s.load(); err != nil {
		return Tokens{}, err
	}
	return s.cur, nil
}

// Save stores a new sign-in (Login's result).
func (s *Source) Save(ctx context.Context, t Tokens) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	t.fill()
	if err := s.store.Set(t.Marshal()); err != nil {
		return fmt.Errorf("codex: saving the sign-in: %w", err)
	}
	s.cur, s.loaded, s.unsaved, s.rejected = t, true, false, ""
	return nil
}

// Headers are the headers of a request with the sign-in, refreshing it
// first when due. ErrSignedOut means the user must sign in again.
func (s *Source) Headers(ctx context.Context) (map[string]string, error) {
	t, err := s.fresh(ctx)
	if err != nil {
		return nil, err
	}
	h := map[string]string{
		"Authorization": "Bearer " + t.AccessToken,
		"originator":    originator,
		"User-Agent":    userAgent,
	}
	if t.AccountID != "" {
		h["ChatGPT-Account-ID"] = t.AccountID
	}
	return h, nil
}

// Refresh renews the sign-in now (after a request was refused with 401).
func (s *Source) Refresh(ctx context.Context) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	if err := s.load(); err != nil {
		return err
	}
	return s.refresh(ctx)
}

func (s *Source) fresh(ctx context.Context) (Tokens, error) {
	if err := s.lock(ctx); err != nil {
		return Tokens{}, err
	}
	defer s.unlock()
	if err := s.load(); err != nil {
		return Tokens{}, err
	}
	if !s.cur.Valid() {
		return Tokens{}, ErrSignedOut
	}
	if s.unsaved && s.store.Set(s.cur.Marshal()) == nil {
		s.unsaved = false
	}
	if s.due() {
		if err := s.refresh(ctx); err != nil {
			return Tokens{}, err
		}
	}
	return s.cur, nil
}

func (s *Source) due() bool {
	if s.cur.AccessToken == "" {
		return true
	}
	now := s.cfg.Now()
	if exp := expiry(s.cur.AccessToken); !exp.IsZero() {
		return !exp.After(now.Add(refreshWindow))
	}
	return !s.cur.LastRefresh.IsZero() && now.Sub(s.cur.LastRefresh) > refreshAge
}

// refresh renews s.cur (locked). A refusal for good is remembered so that
// later calls fail without asking the issuer again.
func (s *Source) refresh(ctx context.Context) error {
	old := s.cur
	if old.RefreshToken == "" || old.RefreshToken == s.rejected {
		return ErrSignedOut
	}
	body, _ := json.Marshal(map[string]string{
		"client_id":     s.cfg.ClientID,
		"grant_type":    "refresh_token",
		"refresh_token": old.RefreshToken,
	})
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Issuer+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("codex: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	a, err := tokenCall(s.cfg, req)
	if err != nil {
		var rej *rejection
		if errors.As(err, &rej) {
			if permanent(rej) {
				s.rejected = old.RefreshToken
				return fmt.Errorf("%w: refresh refused: %w", ErrSignedOut, rej)
			}
			return fmt.Errorf("%w: refresh failed: %w", ErrUnavailable, rej)
		}
		return err
	}
	if a.AccessToken == "" {
		return fmt.Errorf("%w: refresh answer without an access token", ErrUnavailable)
	}
	t := old
	t.AccessToken = a.AccessToken
	if a.IDToken != "" {
		t.IDToken = a.IDToken
	}
	if a.RefreshToken != "" {
		t.RefreshToken = a.RefreshToken
	}
	t.LastRefresh = s.cfg.Now()
	t.fill()
	s.cur = t
	// The refresh token may have rotated: keep it in memory even if the
	// Keychain refuses it now, and try again on the next call.
	s.unsaved = s.store.Set(t.Marshal()) != nil
	return nil
}

// permanent mirrors codex-rs: 401, 400 invalid_grant, or a refresh-token
// code means the sign-in is gone.
func permanent(r *rejection) bool {
	switch r.Code {
	case "refresh_token_expired", "refresh_token_reused", "refresh_token_invalidated":
		return true
	case "invalid_grant":
		return r.Status == http.StatusBadRequest || r.Status == http.StatusUnauthorized
	}
	return r.Status == http.StatusUnauthorized
}

// SignOut revokes the sign-in at the issuer (best effort, as codex-rs:
// a failed revoke still signs out) and deletes it from the store.
func (s *Source) SignOut(ctx context.Context) error {
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	if s.load() == nil && s.cur.Valid() {
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		revoke(rctx, s.cfg, s.cur)
		cancel()
	}
	if err := s.store.Set(""); err != nil {
		return fmt.Errorf("codex: deleting the sign-in: %w", err)
	}
	s.cur, s.loaded, s.unsaved, s.rejected = Tokens{}, true, false, ""
	return nil
}

// revoke posts the refresh token (or else the access token) to
// /oauth/revoke, as codex-rs does on logout. Errors are ignored.
func revoke(ctx context.Context, cfg Config, t Tokens) {
	body := map[string]string{"token": t.RefreshToken, "token_type_hint": "refresh_token", "client_id": cfg.ClientID}
	if t.RefreshToken == "" {
		body = map[string]string{"token": t.AccessToken, "token_type_hint": "access_token"}
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Issuer+"/oauth/revoke", bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if resp, err := cfg.HTTP.Do(req); err == nil {
		resp.Body.Close()
	}
}
