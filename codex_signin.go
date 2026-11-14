package main

// The ChatGPT sign-in of the Codex service (internal/codex; the "codex"
// language preset): the service dialog signs in in the browser (OAuth
// with PKCE, the callback on 127.0.0.1: no server of VoxtMini's) and out.
// The sign-in is one Keychain item, the service's key account
// ("llm.codex", behind remote.Keys), so "has a key" is "signed in" and
// the card shows the account's email in place of a key's mask. One
// codex.Source refreshes it for every request (internal/remote's
// Config.SignIn).

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/codex"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/providers"
	"github.com/hehehai/VoxtMini/internal/remote"
)

// openBrowser opens the sign-in page; tests answer it themselves.
var openBrowser = func(u string) error { return mygo.Shell.OpenExternal(u) }

// codexConfig is where the sign-in goes; tests point it at a fake issuer.
var codexConfig = func(a *app) codex.Config { return codex.Config{HTTP: a.remoteHTTP()} }

// signInState is the dialog's sign-in in progress.
type signInState struct {
	running bool
	cancel  context.CancelFunc
	err     string // why the last one failed, "" none
}

// codexStore keeps the sign-in as the service's Keychain item.
type codexStore struct {
	keys *remote.Keys
	acc  string
}

func (s codexStore) Get() (string, error) { return s.keys.Get(s.acc) }
func (s codexStore) Set(v string) error   { return s.keys.Set(s.acc, "VoxtMini ChatGPT", v) }

// remoteHTTP is an HTTP client on the family's proxy as it is at each
// request.
func (a *app) remoteHTTP() *http.Client {
	proxy := a.remoteProxy()
	return asr.NewHTTPClient(func(r *http.Request) (*url.URL, error) {
		if p := proxy(); p != nil {
			return p(r)
		}
		return http.ProxyFromEnvironment(r)
	}, 0)
}

// codexSource is the sign-in of the Codex service (made with the keys).
func (a *app) codexSource() *codex.Source {
	if a.codexSrc == nil && a.rkeys != nil {
		a.codexSrc = codex.NewSource(codexStore{keys: a.rkeys, acc: providers.Account(providers.LLM, "codex")}, codexConfig(a))
	}
	return a.codexSrc
}

// signInFor is the headers of a service that signs in (nil for the
// others): any goroutine, once the source is made.
func signInFor(src *codex.Source) func(account string) llm.HeaderSource {
	return func(string) llm.HeaderSource {
		if src == nil {
			return nil
		}
		return src
	}
}

// signInAuth is the dialog's or a card's sign-in headers for kind's
// service id, nil when it uses a key.
func (a *app) signInAuth(kind providers.Kind, id string) llm.HeaderSource {
	if pr, _ := providers.LookupPreset(kind, id); !pr.SignIn {
		return nil
	}
	if src := a.codexSource(); src != nil {
		return src
	}
	return nil
}

// keyMaskOf is how the card and the dialog show a saved item: a
// sign-in's email, else a key's ends.
func keyMaskOf(v string) string {
	if t, err := codex.Parse(v); err == nil && t.Valid() {
		if t.Email != "" {
			return t.Email
		}
		return "ChatGPT"
	}
	return remote.Mask(v)
}

// signIn signs in in the browser, off the main thread; the dialog shows
// it running with Cancel.
func (a *app) signIn() {
	d := &a.rmDlg
	src := a.codexSource()
	if src == nil || d.signIn.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.signIn = signInState{running: true, cancel: cancel}
	acc, cfg := providers.Account(d.kind, d.id), codexConfig(a)
	a.async(func() {
		t, err := codex.Login(ctx, cfg, openBrowser)
		if err == nil {
			err = src.Save(ctx, t)
		}
		cancel()
		a.post(func() {
			a.rmDlg.signIn = signInState{err: signInError(err)}
			a.loadRemoteKeys([]string{acc})
		})
	})
}

// cancelSignIn stops a sign-in waiting for the browser.
func (a *app) cancelSignIn() {
	if s := a.rmDlg.signIn; s.cancel != nil {
		s.cancel()
	}
}

// signOut forgets the sign-in (and revokes it with the issuer).
func (a *app) signOut() {
	src := a.codexSource()
	if src == nil {
		return
	}
	acc := providers.Account(a.rmDlg.kind, a.rmDlg.id)
	a.async(func() {
		err := src.SignOut(context.Background())
		a.post(func() {
			if err != nil {
				a.rmDlg.signIn.err = i18n.T("Could not sign out.")
			}
			delete(a.rmTests, acc)
			a.loadRemoteKeys([]string{acc})
		})
	})
}

// signInError is what the dialog says of a failed sign-in ("" for none
// or a cancel).
func signInError(err error) string {
	var ce *codex.CallbackError
	switch {
	case err == nil, errors.Is(err, context.Canceled):
		return ""
	case errors.Is(err, codex.ErrPortInUse):
		return i18n.T("Another sign-in is using the ports it needs (often the Codex CLI's). Finish or close it, then try again.")
	case errors.As(err, &ce):
		return i18n.T("The sign-in was declined in the browser.")
	}
	return i18n.T("Could not sign in.")
}
