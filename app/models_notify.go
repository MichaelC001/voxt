package main

// macOS notifications for a model's download that finished or failed
// while VoxtMini's window was not in front (Voxt's
// SystemNotificationSupport: MLXModelManager posts both). In front, the
// toast and the model's card say it. macOS asks whether VoxtMini may show
// notifications the first time one is shown (mygo.Notification.Show);
// System Settings → Notifications is the only switch. A click shows the
// Local models page.

import (
	"errors"
	"time"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// sysNote is a macOS notification: its id (one per model, so a newer
// replaces an older), title and text.
type sysNote struct{ ID, Title, Body string }

// sysNoteState holds the seams of the notifications: tests record what
// would show and say whether the window is in front.
type sysNoteState struct {
	sysNotify func(sysNote)
	winFront  func() bool
}

// downloadNotes are the notifications for the downloads that ended
// between two statuses: installed (but VAD models, which come with a
// speech model) or failed (not cancelled: Err stays nil).
func downloadNotes(before, now map[string]models.Status) []sysNote {
	var out []sysNote
	for _, m := range models.Catalog() {
		b, s := before[m.ID], now[m.ID]
		if !b.Downloading || m.Kind == models.VAD {
			continue // VoxtMini's own downloads only
		}
		name := uitext.ModelName(m.ID)
		switch {
		case s.Installed && !b.Installed:
			out = append(out, sysNote{ID: "download/" + m.ID, Title: i18n.T("Download finished"), Body: i18n.T("%s is installed.", name)})
		case !s.Downloading && s.Err != nil && b.Err == nil:
			msg, _ := uitext.DownloadErrorText(s.Err)
			out = append(out, sysNote{ID: "download/" + m.ID, Title: i18n.T("Download failed"), Body: i18n.T("%s: %s", name, msg)})
		}
	}
	return out
}

// noteDownloads shows the notifications for the downloads that ended,
// unless the window is in front.
func (a *app) noteDownloads(before map[string]models.Status) {
	notes := downloadNotes(before, a.modelStatus)
	if len(notes) == 0 || a.windowFront() {
		return
	}
	for _, n := range notes {
		a.showSysNote(n)
	}
}

// windowFront is whether the main window is in front, with the keyboard
// (the user sees the toast).
func (a *app) windowFront() bool {
	if a.winFront != nil {
		return a.winFront()
	}
	return a.win != nil && a.win.IsVisible() && a.win.IsFocused()
}

// showSysNote shows n: off the main thread, as the first one waits for
// the user's answer to macOS's question.
func (a *app) showSysNote(n sysNote) {
	if a.sysNotify != nil {
		a.sysNotify(n)
		return
	}
	if !a.hasMenu {
		return
	}
	a.async(func() {
		if !mygo.NotificationsSupported() {
			return
		}
		note := mygo.NewNotification(mygo.NotificationOptions{ID: n.ID, Title: n.Title, Body: n.Body})
		note.OnClick(func() {
			a.post(func() {
				a.showMainWindow()
				a.navigate(modelsPath)
			})
		})
		if err := note.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
			a.logs.Append(core.LogEntry{Time: time.Now(), Level: core.SeverityWarning, Source: omniSource, Message: "notification not shown: " + err.Error()})
		}
	})
}
