package main

import (
	"image"
	"testing"

	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestInfoTipCentred: a ⓘ's centre is on the middle of its label's
// glyphs, in English and Chinese, in a wide and a narrow window, for each
// kind of row (a page's title, a section's title, a setting's row, a row
// with a note under it): measured on the pixels, the ⓘ's ink against the
// ink of the label left of it. The labels have no descenders, so their
// ink is their capitals' or ideographs' height.
func TestInfoTipCentred(t *testing.T) {
	rows := []struct{ path, label, tip string }{
		{"/translate", "Translate", "Select text to translate it in place; with nothing selected, speak."},
		{"/models", "Models", "Local models run on this Mac. A remote service gets your audio or text only for a feature you choose it for."},
		{"/transcribe", "Models", "Automatic: the best installed local model. A remote service is used only if you choose it."},
		{"/settings/overlay", "Position", "On the display under the pointer."},
		{"/settings/general", "Show icon in Dock", "Off: the icon shows only while the window is open."},
	}
	const scale = 2
	for _, lang := range []string{"en", "zh-Hans"} {
		for _, w := range []int{windowWidth, 680} {
			a, _, tt := newTestApp(t)
			tt.SetSize(w, 1000)
			tt.SetScale(scale)
			if lang != "en" {
				a.updateSettings(func(s *settings.Settings) { s.General.Language = lang })
			}
			for _, r := range rows {
				a.navigate(r.path)
				frames(tt)
				tip, ok := tt.Find(i18n.T(r.tip))
				if !ok {
					t.Fatalf("%s %d %s: no ⓘ %q in %q", lang, w, r.path, i18n.T(r.tip), tt.Texts())
				}
				img := tt.Image()
				iy0, iy1, ok1 := inkRows(img, tip.X, tip.X+tip.W, tip.Y, tip.Y+tip.H, scale)
				// The label: the ink left of the ⓘ, up to a gap of 12 DIPs.
				band := float32(14)
				lx := labelLeft(img, tip.X-1, tip.Y-band, tip.Y+tip.H+band, scale)
				ly0, ly1, ok2 := inkRows(img, lx, tip.X-1, tip.Y-band, tip.Y+tip.H+band, scale)
				if !ok1 || !ok2 {
					t.Fatalf("%s %d %s: no ink: ⓘ %v, label %v", lang, w, r.path, ok1, ok2)
				}
				d := (iy0+iy1)/2 - (ly0+ly1)/2
				t.Logf("%s %d %s: %+.2f DIPs", lang, w, r.path, d) // go test -v on a Mac: the real fonts
				if d < -0.5 || d > 0.5 {
					t.Errorf("%s %d %s %q: ⓘ centre %.2f, label's glyphs %.2f (%.2f..%.2f): %+.2f DIPs off", lang, w, r.path, i18n.T(r.label), (iy0+iy1)/2, (ly0+ly1)/2, ly0, ly1, d)
				}
			}
		}
	}
}

// lumAt is the luminance of the pixel at (x, y) DIPs.
func lumAt(img image.Image, x, y int) float32 {
	r, g, b, _ := img.At(x, y).RGBA()
	return (0.299*float32(r) + 0.587*float32(g) + 0.114*float32(b)) / 65535
}

// isInk reports whether the pixel differs from the background bg enough
// to be a glyph's or an icon's.
func isInk(img image.Image, x, y int, bg float32) bool {
	d := bg - lumAt(img, x, y)
	return d > 0.2 || d < -0.2
}

// labelLeft is the left edge (DIPs) of the ink that ends just left of x,
// between top and bottom: columns of ink until 12 DIPs without any.
func labelLeft(img image.Image, x, top, bottom float32, s int) float32 {
	x1, y0, y1 := int(x*float32(s)), int(top*float32(s)), int(bottom*float32(s))
	bg := lumAt(img, x1, y0)
	left, gap := x1, 0
	for px := x1; px >= 0 && gap < 12*s; px-- {
		ink := false
		for py := y0; py < y1 && !ink; py++ {
			ink = isInk(img, px, py, bg)
		}
		if ink {
			left, gap = px, 0
		} else {
			gap++
		}
	}
	return float32(left) / float32(s)
}

// inkRows is the first and last row (DIPs) of ink in the box.
func inkRows(img image.Image, x0, x1, y0, y1 float32, s int) (top, bottom float32, ok bool) {
	px0, px1, py0, py1 := int(x0*float32(s)), int(x1*float32(s)), int(y0*float32(s)), int(y1*float32(s))
	bg := lumAt(img, px0, py0)
	first, last := -1, -1
	for y := py0; y < py1; y++ {
		for x := px0; x < px1; x++ {
			if isInk(img, x, y, bg) {
				if first < 0 {
					first = y
				}
				last = y
				break
			}
		}
	}
	if first < 0 {
		return 0, 0, false
	}
	return float32(first) / float32(s), float32(last+1) / float32(s), true
}
