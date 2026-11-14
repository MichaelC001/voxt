package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/codex"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/remote"
)

func testJWT(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"none"}`)) + "." + enc.EncodeToString(b) + ".sig"
}

// fakeIssuer is auth.openai.com on loopback: the code exchange and the
// revoke.
type fakeIssuer struct {
	srv     *httptest.Server
	mu      sync.Mutex
	revoked bool
	code    string
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	is := &fakeIssuer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		is.mu.Lock()
		is.code = r.FormValue("code")
		is.mu.Unlock()
		access := testJWT(map[string]any{"exp": time.Now().Add(time.Hour).Unix(),
			"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acct-1"}})
		json.NewEncoder(w).Encode(map[string]string{"id_token": testJWT(map[string]any{"email": "me@example.com"}),
			"access_token": access, "refresh_token": "refresh-1"})
	})
	mux.HandleFunc("POST /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		is.mu.Lock()
		is.revoked = true
		is.mu.Unlock()
	})
	is.srv = httptest.NewServer(mux)
	t.Cleanup(is.srv.Close)
	return is
}

// TestCodexSignIn: the Codex service's dialog signs in in the "browser"
// (the callback answered on loopback), the card shows the account, the
// sign-in lives in the Keychain item as one JSON string, a test request
// carries its headers; signing out revokes and forgets it.
func TestCodexSignIn(t *testing.T) {
	is := newFakeIssuer(t)
	var chatAuth, chatAccount string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatAuth, chatAccount = r.Header.Get("Authorization"), r.Header.Get("ChatGPT-Account-ID")
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer api.Close()
	oldCfg, oldOpen := codexConfig, openBrowser
	codexConfig = func(*app) codex.Config { return codex.Config{Issuer: is.srv.URL, Port: codex.AnyPort} }
	var opened string
	openBrowser = func(u string) error {
		opened = u
		q, _ := url.Parse(u)
		go http.Get(q.Query().Get("redirect_uri") + "?code=code-1&state=" + url.QueryEscape(q.Query().Get("state")))
		return nil
	}
	t.Cleanup(func() { codexConfig, openBrowser = oldCfg, oldOpen })

	ring := &memKeyring{keys: map[string]string{}}
	a, tt := remoteApp(t, ring)
	a.rkeys = remote.NewKeysMask(ring, keyMaskOf)
	a.codexSrc = nil
	a.navigate(remotePath)
	a.mdTab = 1
	a.openProviderDialog(providers.LLM, "codex")
	frames(tt)
	if !has(tt, "Sign in with ChatGPT") || has(tt, "Key for ChatGPT (Codex)") {
		t.Fatalf("dialog %q", firstTexts(tt))
	}
	click(t, tt, "Sign in with ChatGPT")
	frames(tt)
	if !strings.HasPrefix(opened, is.srv.URL+"/oauth/authorize?") || is.code != "code-1" {
		t.Fatalf("opened %q code %q, err %q", opened, is.code, a.rmDlg.signIn.err)
	}
	tok, err := codex.Parse(ring.keys["llm.codex"])
	if err != nil || tok.RefreshToken != "refresh-1" || tok.Email != "me@example.com" {
		t.Fatalf("saved %+v %v", tok, err)
	}
	if !has(tt, "Signed in as me@example.com") {
		t.Fatalf("not signed in: %q", firstTexts(tt))
	}
	a.rmDlg.endpoint = api.URL + "/codex"
	click(t, tt, "Save")
	frames(tt)
	if !has(tt, "me@example.com") {
		t.Errorf("card %q", firstTexts(tt))
	}

	// A test sends the sign-in's headers.
	p, _ := providers.Setup(a.lib.settings.Get(), providers.LLM, "codex")
	a.testProvider(providers.LLM, p)
	if tr := a.rmTests["llm.codex"]; tr.res.Err != nil || !strings.HasPrefix(chatAuth, "Bearer ") || chatAccount != "acct-1" {
		t.Fatalf("test %v, auth %q account %q", tr.res.Err, chatAuth, chatAccount)
	}

	// Signed out: revoked and forgotten, the card says so.
	a.openProviderDialog(providers.LLM, "codex")
	frames(tt)
	click(t, tt, "Sign out")
	frames(tt)
	if !is.revoked || ring.keys["llm.codex"] != "" || a.hasKey("llm.codex") {
		t.Fatalf("signed out: revoked %v item %q", is.revoked, ring.keys["llm.codex"])
	}
	a.rmDlg.open = false
	frames(tt)
	if !has(tt, "Not signed in") {
		t.Errorf("card %q", firstTexts(tt))
	}
}
