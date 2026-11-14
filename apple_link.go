package main

// Apple's on-device models (internal/apple, docs/apple-models.md): Apple
// Speech and Apple Intelligence, which macOS brings and the helper runs.
// a.apples lists them after the downloaded models once the helper said
// what this Mac has (apple.status): when the helper becomes ready, when
// the Models page shows, after a download of Apple's or a Speech
// Recognition prompt. Views read a.appleSt, the copy refreshApple takes
// on the main thread.

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/hehehai/VoxtMini/internal/apple"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// helperCaller is the helper's supervisor as apple.Caller, read when
// called: openModels runs before connectHelper makes it.
type helperCaller struct{ h *helperHost }

func (c helperCaller) Call(ctx context.Context, id, method string, params any, notify func(proto.Message)) (json.RawMessage, error) {
	if c.h == nil || c.h.sup == nil {
		return nil, proto.Errorf(proto.CodeMethodNotFound, "no helper")
	}
	return c.h.sup.Call(ctx, id, method, params, notify)
}

// appleDownload is Apple's download of speech languages in progress.
type appleDownload struct {
	busy     bool
	fraction float64
	err      error
}

// refreshApple asks the helper what Apple's models this Mac has now.
func (a *app) refreshApple() {
	ap := a.apples
	if ap == nil || !a.hstatus.Has(proto.MethodAppleStatus) {
		return
	}
	a.async(func() {
		st, _ := ap.Refresh(context.Background()) // a failure keeps what was known
		a.post(func() { a.appleChanged(st) })
	})
}

// appleChanged takes a new status: the lists and the sidebar's card
// follow it.
func (a *app) appleChanged(st apple.Status) {
	a.appleSt = st
	a.checkModels() // shell_notices.go
}

// appleWanted are the speech locales of the user's languages (the main
// one, then the others) Apple Speech is used for, in order.
func appleWanted(s settings.Settings, uiLocale string) []string {
	var out []string
	for _, l := range append([]string{session.MainLanguage(s, uiLocale)}, s.General.OtherLanguages...) {
		if loc := apple.Locale(l); loc != "" && !slices.Contains(out, loc) {
			out = append(out, loc)
		}
	}
	return out
}

// appleToDownload are the wanted locales Apple can download now.
func (a *app) appleToDownload() []string {
	var out []string
	for _, loc := range appleWanted(a.prefs, i18n.Locale()) {
		if a.appleSt.LocaleStatus(loc) == proto.LocaleAvailable {
			out = append(out, loc)
		}
	}
	return out
}

// downloadAppleLanguages asks Apple to download the user's languages for
// Apple Speech.
func (a *app) downloadAppleLanguages() {
	locs := a.appleToDownload()
	ap := a.apples
	if len(locs) == 0 || ap == nil || a.appleDl.busy {
		return
	}
	a.appleDl = appleDownload{busy: true}
	a.async(func() {
		var last time.Time
		err := ap.Install(context.Background(), locs, func(f float64) {
			if time.Since(last) < 200*time.Millisecond && f < 1 {
				return
			}
			last = time.Now()
			a.post(func() { a.appleDl.fraction = f })
		})
		st := ap.Status()
		a.post(func() {
			a.appleDl = appleDownload{err: err}
			a.appleChanged(st)
			if err == nil {
				a.toast(uitext.AppleLanguagesReady())
			}
		})
	})
}
