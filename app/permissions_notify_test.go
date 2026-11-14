package main

import (
	"testing"

	"github.com/egoist/mygo"
)

// TestNotificationsRow: Settings → Permissions shows macOS's answer once
// it is known, offers System Settings when notifications cannot be seen,
// and sends a test notification (shown even with the window in front;
// a click shows Permissions).
func TestNotificationsRow(t *testing.T) {
	a, _, tt := newTestApp(t)
	var st mygo.NotificationStatus
	reads := 0
	a.sysNoteStatus = func(fn func(mygo.NotificationStatus)) { reads++; fn(st) }
	var shown []sysNote
	a.sysNotify = func(n sysNote) { shown = append(shown, n) }
	a.winFront = func() bool { return true }

	show := func() {
		a.navigate("/dictionary")
		a.navigate(permissionsPath)
		a.recheckPermissions()
		frames(tt)
	}
	show()
	if reads == 0 || hasExact(tt, "Notifications") {
		t.Fatalf("unknown status: reads %d, %q", reads, tt.Texts())
	}

	st = mygo.NotificationStatus{Known: true}
	show()
	if !hasExact(tt, "Notifications") || !hasExact(tt, "Not asked yet") || hasExact(tt, "No banners") {
		t.Fatalf("not asked: %q", tt.Texts())
	}
	click(t, tt, "Test")
	if len(shown) != 1 || shown[0].ID != "test" || shown[0].Path != permissionsPath || shown[0].Body != "This is how VoxtMini's notifications look." {
		t.Fatalf("test notification %+v", shown)
	}

	st = mygo.NotificationStatus{Known: true, Asked: true, Denied: true}
	show()
	if !hasExact(tt, "Not allowed") || count(tt, "Open System Settings") == 0 {
		t.Fatalf("denied: %q", tt.Texts())
	}
	before := count(tt, "Open System Settings")
	st = mygo.NotificationStatus{Known: true, Asked: true, Allowed: true, Style: 0}
	show()
	if !hasExact(tt, "No banners") || count(tt, "Open System Settings") != before {
		t.Fatalf("no banners: %q", tt.Texts())
	}
	st = mygo.NotificationStatus{Known: true, Asked: true, Allowed: true, Style: 1}
	show()
	if hasExact(tt, "No banners") || count(tt, "Open System Settings") >= before { // its button gone
		t.Fatalf("banners: %q", tt.Texts())
	}
}

// count is how many texts are s.
func count(tt interface{ Texts() []string }, s string) int {
	n := 0
	for _, x := range tt.Texts() {
		if x == s {
			n++
		}
	}
	return n
}
