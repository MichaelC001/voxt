package core

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrRedirect: a redirect was refused (CheckRedirect, NoDowngrade).
var ErrRedirect = errors.New("redirect refused")

// maxRedirects is how many redirects a request follows, as Go's default.
const maxRedirects = 10

// CheckRedirect is the http.Client.CheckRedirect of clients whose
// requests carry credentials (remote services: API keys in x-api-key,
// api-key, x-goog-api-key, a sign-in's ChatGPT-Account-ID; Go drops only
// Authorization and cookies on a redirect to another host). A redirect is
// followed only to the same host and port, and never from https to
// http: a service, or a relay pretending to be one, cannot send the
// headers elsewhere or in clear text.
func CheckRedirect(req *http.Request, via []*http.Request) error {
	if err := NoDowngrade(req, via); err != nil {
		return err
	}
	if from := via[0].URL; !strings.EqualFold(req.URL.Host, from.Host) {
		return fmt.Errorf("%w: from %s to another host, %s", ErrRedirect, from.Host, req.URL.Host)
	}
	return nil
}

// NoDowngrade is the http.Client.CheckRedirect of clients without
// credentials (announcements): any host, but never from https to http.
func NoDowngrade(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("%w: stopped after %d redirects", ErrRedirect, maxRedirects)
	}
	if from := via[0].URL; from.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("%w: from https to %s (%s)", ErrRedirect, req.URL.Scheme, req.URL.Host)
	}
	return nil
}
