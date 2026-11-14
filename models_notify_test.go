package main

import (
	"errors"
	"maps"
	"testing"

	"github.com/hehehai/VoxtMini/internal/models"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

func TestDownloadNotes(t *testing.T) {
	a, _, _ := newTestApp(t)
	var shown []sysNote
	front := false
	a.sysNotify = func(n sysNote) { shown = append(shown, n) }
	a.winFront = func() bool { return front }
	down := models.Status{Downloading: true, Total: 10}
	base := map[string]models.Status{
		models.ASRFast:     down,
		models.VADSilero:   down,
		models.LLMFast:     down,
		models.LLMBalanced: down,
		models.ASRAccurate: {Installed: true}, // another app's, used: no download
	}
	a.modelStatus = maps.Clone(base)
	a.modelStatus[models.ASRFast] = models.Status{Installed: true}
	a.modelStatus[models.VADSilero] = models.Status{Installed: true}                   // comes with a speech model
	a.modelStatus[models.LLMFast] = models.Status{Err: errors.New("connection reset")} // failed
	a.modelStatus[models.LLMBalanced] = models.Status{}                                // cancelled
	a.noteDownloads(base)
	want := []sysNote{
		{"download/" + models.ASRFast, "Download finished", uitext.ModelName(models.ASRFast) + " is installed."},
	}
	msg, _ := uitext.DownloadErrorText(errors.New("connection reset"))
	want = append(want, sysNote{"download/" + models.LLMFast, "Download failed", uitext.ModelName(models.LLMFast) + ": " + msg})
	if len(shown) != len(want) {
		t.Fatalf("shown %+v", shown)
	}
	for _, w := range want {
		found := false
		for _, n := range shown {
			found = found || n == w
		}
		if !found {
			t.Errorf("missing %+v in %+v", w, shown)
		}
	}
	// In front: the toast says it, no notification.
	shown, front = nil, true
	a.noteDownloads(base)
	if len(shown) != 0 {
		t.Errorf("window in front: %+v", shown)
	}
	// A status that only changed again (still installed, still failed):
	// nothing.
	front = false
	a.noteDownloads(a.modelStatus)
	if len(shown) != 0 {
		t.Errorf("no change: %+v", shown)
	}
}
