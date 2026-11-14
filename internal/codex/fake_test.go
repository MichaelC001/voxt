package codex

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// jwt makes an unsigned token with the given claims.
func jwt(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"none"}`)) + "." + enc.EncodeToString(b) + ".sig"
}

func idToken(account, email string) string {
	return jwt(map[string]any{
		"email":                       email,
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": account},
	})
}

func accessToken(name string, exp time.Time) string {
	return jwt(map[string]any{"exp": exp.Unix(), "name": name})
}

// issuer is a fake auth.openai.com on loopback.
type issuer struct {
	t   *testing.T
	srv *httptest.Server
	now time.Time

	mu        sync.Mutex
	exchanges []url.Values
	refreshes []map[string]string
	revokes   []map[string]string
	challenge string // from the last authorize URL the test opened

	// refreshStatus/refreshBody override the refresh answer.
	refreshStatus int
	refreshBody   string
	refreshDelay  time.Duration
	n             int
}

func newIssuer(t *testing.T) *issuer {
	is := &issuer{t: t, now: time.Unix(1_800_000_000, 0)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/token", is.token)
	mux.HandleFunc("POST /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		var m map[string]string
		_ = json.NewDecoder(r.Body).Decode(&m)
		is.mu.Lock()
		is.revokes = append(is.revokes, m)
		is.mu.Unlock()
	})
	is.srv = httptest.NewServer(mux)
	t.Cleanup(is.srv.Close)
	return is
}

func (is *issuer) cfg() Config {
	return Config{Issuer: is.srv.URL, HTTP: is.srv.Client(), Port: AnyPort, Now: func() time.Time { return is.now }}
}

func (is *issuer) token(w http.ResponseWriter, r *http.Request) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.n++
	switch r.Header.Get("Content-Type") {
	case "application/x-www-form-urlencoded":
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		is.exchanges = append(is.exchanges, form)
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		if form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != is.challenge {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"id_token":      idToken("acct-1", "me@example.com"),
			"access_token":  accessToken("a1", is.now.Add(time.Hour)),
			"refresh_token": "r1",
		})
	case "application/json":
		var m map[string]string
		_ = json.NewDecoder(r.Body).Decode(&m)
		is.refreshes = append(is.refreshes, m)
		n := len(is.refreshes)
		if is.refreshDelay > 0 {
			is.mu.Unlock()
			time.Sleep(is.refreshDelay)
			is.mu.Lock()
		}
		if is.refreshStatus != 0 {
			w.WriteHeader(is.refreshStatus)
			fmt.Fprint(w, is.refreshBody)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"access_token":  accessToken(fmt.Sprintf("a%d", n+1), is.now.Add(time.Hour)),
			"refresh_token": fmt.Sprintf("r%d", n+1),
		})
	default:
		w.WriteHeader(http.StatusUnsupportedMediaType)
	}
}

func (is *issuer) counts() (exchanges, refreshes, revokes int) {
	is.mu.Lock()
	defer is.mu.Unlock()
	return len(is.exchanges), len(is.refreshes), len(is.revokes)
}

// memStore is a Store in memory.
type memStore struct {
	mu   sync.Mutex
	v    string
	sets int
	fail error
}

func (m *memStore) Get() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.v, nil
}

func (m *memStore) Set(v string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.v = v
	m.sets++
	return nil
}

func (m *memStore) tokens(t *testing.T) Tokens {
	t.Helper()
	v, _ := m.Get()
	tok, err := Parse(v)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}
