package main

// Snapshots of the Network dialog (models_network.go): as it opens, and
// after Test connection (settings_about_net.go), one host down.

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hehehai/VoxtMini/internal/settings"
)

func TestSnapshotsNetwork(t *testing.T) {
	runSnapshots(t, "VOXTMINI_SNAPSHOTS", snapshotsNetwork)
}

func snapshotsNetwork(t *testing.T, out string) {
	for _, run := range []struct {
		lang string
		dark bool
	}{{"zh-Hans", false}, {"en", false}} {
		a, _, tt := newTestApp(t)
		tt.SetSize(windowWidth, windowHeight)
		if run.lang != "en" {
			a.updateSettings(func(s *settings.Settings) { s.General.Language = run.lang })
		}
		tt.SetScale(2)
		a.clearNotices()
		a.openURL("voxtnew://models/network")
		save := func(name string) {
			t.Helper()
			a.toasts = nil
			frames(tt)
			frames(tt)
			f, err := os.Create(filepath.Join(out, fmt.Sprintf("network-%s-%s.png", name, run.lang)))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
		save("open")
		snapshotsNetworkTested(t, a, save)
	}
}

// snapshotsNetworkTested: after Test connection, huggingface.co down.
func snapshotsNetworkTested(t *testing.T, a *app, save func(string)) {
	p := &probed{fail: []string{"huggingface.co"}} // settings_about_net_test.go
	old := reachHosts
	reachHosts = p.reach
	defer func() { reachHosts = old }()
	a.testNetwork()
	save("tested")
}
