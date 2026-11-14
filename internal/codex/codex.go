// Package codex signs in with a ChatGPT account the way the Codex CLI
// does (OAuth 2 with PKCE against auth.openai.com, the CLI's public client
// id) so that VoxtMini can call the account's Codex models, and keeps that
// sign-in fresh: Source refreshes the tokens and hands out the request
// headers. The tokens are one JSON string (Tokens.Marshal) that the app
// keeps as one Keychain item behind Store; this package never writes a
// file, logs, or puts a token into an error.
//
// The OAuth facts follow the open-source Codex CLI (github.com/openai/codex,
// codex-rs/login): the issuer, client id, scopes, the extra authorize
// parameters, the loopback callback on port 1455 (1457 when that is
// taken), a form-encoded code exchange, a JSON refresh, and a revoke on
// sign-out. Voxt read the CLI's ~/.codex/auth.json instead; VoxtMini
// signs in itself (sharing the CLI's rotating refresh token would sign one
// of the two out).
//
// Imports: nothing internal.
package codex

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// The Codex CLI's public OAuth client.
const (
	DefaultIssuer   = "https://auth.openai.com"
	DefaultClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	// DefaultPort is the callback port registered for the client; the
	// registered FallbackPort is used when it is taken.
	DefaultPort  = 1455
	FallbackPort = 1457
	// AnyPort makes Login listen on any free port (tests only: the issuer
	// accepts only the registered ports).
	AnyPort = -1

	originator = "codex_cli_rs"
	userAgent  = "codex_cli_rs/0.0.0 (VoxtMini)"
	scope      = "openid profile email offline_access"
)

// Errors a caller tells apart with errors.Is.
var (
	// ErrSignedOut: there is no sign-in, or the issuer refused to refresh
	// it (expired, reused, revoked): the user has to sign in again.
	ErrSignedOut = errors.New("codex: not signed in")
	// ErrUnavailable: the issuer could not be reached or answered
	// something unusable; the sign-in itself may still be good.
	ErrUnavailable = errors.New("codex: sign-in service unavailable")
	// ErrPortInUse: Login could not listen on the registered callback
	// ports (another sign-in, often the Codex CLI's, holds them).
	ErrPortInUse = errors.New("codex: sign-in callback port in use")
)

// Config says where to sign in. The zero value is the Codex CLI's.
type Config struct {
	Issuer   string       // DefaultIssuer
	ClientID string       // DefaultClientID
	HTTP     *http.Client // http.DefaultClient; the app passes its proxied one
	Port     int          // 0: DefaultPort, then FallbackPort; AnyPort in tests
	Now      func() time.Time
}

func (c Config) withDefaults() Config {
	if c.Issuer == "" {
		c.Issuer = DefaultIssuer
	}
	c.Issuer = strings.TrimRight(c.Issuer, "/")
	if c.ClientID == "" {
		c.ClientID = DefaultClientID
	}
	if c.HTTP == nil {
		c.HTTP = http.DefaultClient
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Tokens is a ChatGPT sign-in.
type Tokens struct {
	IDToken      string    `json:"id_token,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	Email        string    `json:"email,omitempty"`
	LastRefresh  time.Time `json:"last_refresh,omitzero"`
}

// Valid reports whether t can make or renew requests at all.
func (t Tokens) Valid() bool { return t.AccessToken != "" || t.RefreshToken != "" }

// Marshal is the one string the app stores.
func (t Tokens) Marshal() string {
	b, _ := json.Marshal(t) // strings and a time: cannot fail
	return string(b)
}

// Parse reads what Marshal wrote. The error never quotes the input.
func Parse(s string) (Tokens, error) {
	var t Tokens
	if err := json.Unmarshal([]byte(s), &t); err != nil {
		return Tokens{}, errors.New("codex: stored sign-in is not valid JSON")
	}
	return t, nil
}

// fill sets AccountID and Email from the tokens' claims where they are
// missing: the id token first, then the access token.
func (t *Tokens) fill() {
	for _, jwt := range []string{t.IDToken, t.AccessToken} {
		c := parseClaims(jwt)
		if t.AccountID == "" {
			t.AccountID = c.Auth.AccountID
		}
		if t.Email == "" {
			t.Email = c.Email
			if t.Email == "" {
				t.Email = c.Profile.Email
			}
		}
	}
}

type claims struct {
	Exp   float64 `json:"exp"`
	Email string  `json:"email"`
	Auth  struct {
		AccountID string `json:"chatgpt_account_id"`
	} `json:"https://api.openai.com/auth"`
	Profile struct {
		Email string `json:"email"`
	} `json:"https://api.openai.com/profile"`
}

// parseClaims decodes a JWT's payload without checking its signature: the
// tokens come straight from the issuer over TLS and are only read for
// display and timing.
func parseClaims(jwt string) claims {
	var c claims
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return c
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return c
	}
	_ = json.Unmarshal(raw, &c)
	return c
}

// expiry is the access token's exp, zero when it has none.
func expiry(jwt string) time.Time {
	exp := parseClaims(jwt).Exp
	if exp <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(exp), 0)
}
