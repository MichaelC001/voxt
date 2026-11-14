package main

// The permissions read again when VoxtMini comes to the front
// (permissions.go): with unchanged permissions no frame differs, while
// the helper's answer is on its way too. The app's own read (Fake:
// Microphone allowed) disagrees with the fake helper's (not asked yet),
// as the two can on a Mac; writing one, then the other made Home's list
// flash at every activation.

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/permission"
)

// statusCalls counts the permissions.status requests so far.
func statusCalls(fs *fakeSupervisor) int {
	n := 0
	for _, m := range fs.methods() {
		if m == proto.MethodPermissionsStatus {
			n++
		}
	}
	return n
}

// missingFrom is what of a is not in b.
func missingFrom(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

// activateSteady brings VoxtMini to the front (what OnDidBecomeActive
// posts) with the helper's answer held back, and checks that every frame
// before and after it lands is the one before.
func activateSteady(t *testing.T, a *app, fs *fakeSupervisor, tt *ui.Tester, what string) {
	t.Helper()
	drain := queuePosts(a)
	defer func() { a.postQueue = nil }()
	frames(tt)
	drain()
	frames(tt)
	before := slices.Clone(tt.Image().Pix)
	texts := tt.Texts()
	same := func(when string) {
		t.Helper()
		tt.Frame()
		if !bytes.Equal(before, tt.Image().Pix) {
			now := tt.Texts()
			t.Errorf("%s: the frame changed %s: shows %q, lost %q", what, when, missingFrom(now, texts), missingFrom(texts, now))
		}
	}
	calls := statusCalls(fs)
	for range 3 {
		a.recheckPermissions() // OnDidBecomeActive, several in a row
		same("while the helper's answer is on its way")
	}
	if n := statusCalls(fs) - calls; n != 1 {
		t.Errorf("%s: %d permissions.status in flight, want 1", what, n)
	}
	drain()
	for range 3 {
		same("after the helper's answer")
	}
	if a.perms.mic != permission.NotDetermined || a.perms.asking {
		t.Errorf("%s: perms %+v", what, a.perms)
	}
}

func TestPermissionsRecheckSteady(t *testing.T) {
	t.Run("home", func(t *testing.T) {
		a, fs, tt := newTestApp(t)
		a.navigate("/home")
		activateSteady(t, a, fs, tt, "Home")
		if !has(tt, "macOS asks for the microphone the first time you record.") {
			t.Errorf("Home lacks the helper's microphone: %q", tt.Texts())
		}
	})
	t.Run("settings", func(t *testing.T) {
		a, fs, tt := newTestApp(t)
		a.navigate("/settings/permissions")
		activateSteady(t, a, fs, tt, "Settings → Permissions")
	})
	t.Run("onboarding", func(t *testing.T) {
		a, fs, tt := newTestApp(t)
		a.openOnboarding()
		a.ob.step = obPermissions
		activateSteady(t, a, fs, tt, "the setup guide")
	})
}

// TestPermissionsRecheckChange: a permission that did change shows once
// the helper says so.
func TestPermissionsRecheckChange(t *testing.T) {
	a, fs, tt := newTestApp(t)
	a.navigate("/home")
	frames(tt)
	if !has(tt, "macOS asks for the microphone the first time you record.") {
		t.Fatalf("Home: %q", tt.Texts())
	}
	fs.handlers[proto.MethodPermissionsStatus] = func(json.RawMessage) (any, error) {
		return proto.PermissionsResult{Microphone: proto.PermDenied, Accessibility: true}, nil
	}
	a.recheckPermissions()
	if !has(tt, "Microphone access is not allowed.") || has(tt, "Needed to type results and for shortcuts like Right ⌥.") {
		t.Errorf("after the change: %q", tt.Texts())
	}
}

// TestPermissionsRecheckFails: a helper that cannot answer leaves what
// showed; with nothing read yet the app's own read shows.
func TestPermissionsRecheckFails(t *testing.T) {
	a, fs, _ := newTestApp(t)
	fs.handlers[proto.MethodPermissionsStatus] = func(json.RawMessage) (any, error) {
		return nil, proto.Errorf(proto.CodeInternal, "boom")
	}
	a.perms.checked = false
	a.recheckPermissions()
	if !a.perms.checked || a.perms.mic != permission.Granted || a.perms.asking {
		t.Errorf("first read: %+v", a.perms)
	}
	a.perms.mic = permission.Denied // the last answer
	a.recheckPermissions()
	if a.perms.mic != permission.Denied {
		t.Errorf("kept: %+v", a.perms)
	}
}
