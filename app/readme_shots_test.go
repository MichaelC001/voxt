//go:build readmeshots

package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestReadmeShotsPages renders the README shots' pages (readme_shots.go)
// with MyGo's renderer on Linux, to look at the made-up data before a
// Mac run:
//
//	VOXTMINI_README_SHOTS=/tmp/readme go test -tags readmeshots -run TestReadmeShotsPages ./app
func TestReadmeShotsPages(t *testing.T) {
	out := os.Getenv("VOXTMINI_README_SHOTS")
	for _, lang := range []string{"en", "zh-Hans"} {
		a, _, tt := newTestApp(t)
		tt.SetSize(readmeShotsW, readmeShotsH)
		tt.SetScale(2)
		if err := readmeSetUp(a, lang); err != nil {
			t.Fatal(err)
		}
		a.updateSettings(func(s *settings.Settings) { s.General.Language = lang })
		for _, st := range readmeSteps(lang, nil)[:4] {
			st.apply(a)
			frames(tt)
			frames(tt)
			if out == "" {
				continue
			}
			f, err := os.Create(filepath.Join(out, st.name+"-"+lang+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}
