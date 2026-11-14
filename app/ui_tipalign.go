package main

// Where a ⓘ stands beside its label (infoTip, ui_marks.go).
//
// A row that centres a label and its ⓘ centres the label's line box, whose
// middle is (ascent − descent) / 2 above the baseline: the ascent and
// descent of every font on the line, a CJK fallback's (taller) as well as
// the Latin font's, which some engines round to whole DIPs. The glyphs'
// middle is elsewhere: about a third of the size above the baseline. So
// the ⓘ sat lower than English labels and higher than Chinese ones, by
// different amounts on each platform. tipMargins puts the ⓘ's centre on
// the middle of the glyphs of the label's first line, measured from the
// baseline the text engine gives that line: the label and its ⓘ go in a
// row aligned at the start, each with the top margin tipMargins returns.

import (
	"runtime"
	"sync"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// The middle of a label's glyphs above its baseline, as a share of the
// font's size: Latin capitals (cap height 0.70 in SF, 0.73 in DejaVu, the
// tests' font) and CJK ideographs, whose em box stands about 0.12 below
// the baseline. PingFang SC (the Mac's Chinese font) draws its ideographs
// lower in the em box than Noto Sans CJK (the tests' Linux font): measured
// on mac-air, 0.37 put the ⓘ 0.5 DIP high beside 14-DIP labels and 1 DIP
// high beside page titles; 0.33 centres it (TestInfoTipCentred).
const latinMiddle = 0.36

var cjkMiddle = func() float32 {
	if runtime.GOOS == "darwin" {
		return 0.33
	}
	return 0.37
}()

// infoTipBox is the ⓘ's side, in DIPs at 100 %.
const infoTipBox = 18

// baselineKey is a font whose first baseline glyphMiddle measured.
type baselineKey struct {
	family string
	size   float32
	weight int
	cjk    bool
}

// baselines caches the first line's baseline below the text's top, per
// font: shaping once per font and size, not per label and frame. Several
// windows' and tests' build passes may read it at once.
var baselines struct {
	sync.Mutex
	m map[baselineKey]float32
}

// glyphMiddle is how far below the top of label's text, in a font of size
// and weight, the middle of its first line's glyphs is.
func glyphMiddle(t *ui.Theme, label string, size float32, weight int) float32 {
	k := baselineKey{t.Font, size, weight, hasCJK(label)}
	share := float32(latinMiddle)
	if k.cjk {
		share = cjkMiddle
	}
	baselines.Lock()
	defer baselines.Unlock()
	b, ok := baselines.m[k]
	if !ok {
		b = firstBaseline(k)
		if baselines.m == nil {
			baselines.m = map[baselineKey]float32{}
		}
		baselines.m[k] = b
	}
	if b <= 0 { // an engine that cannot tell: the line box's middle
		return lineHeight(k) / 2
	}
	return b - share*size
}

// firstBaseline shapes a sample of k's script followed by an empty inline
// object, which stands on the baseline: its top is the baseline (0: none).
func firstBaseline(k baselineKey) float32 {
	sample := "H"
	if k.cjk {
		sample = "中"
	}
	l := ui.ShapeRichText([]ui.Span{{Text: sample}, {Text: "￼", ObjectWidth: 1}}, ui.Font{Family: k.family, Size: k.size, Weight: k.weight}, 0)
	if o := l.Objects(); len(o) == 1 {
		return o[0].Rect.Y
	}
	return 0
}

// lineHeight is the height of a line of k's sample.
func lineHeight(k baselineKey) float32 {
	sample := "H"
	if k.cjk {
		sample = "中"
	}
	_, h := ui.ShapeText(sample, ui.Font{Family: k.family, Size: k.size, Weight: k.weight}, 0).Size()
	return h
}

// hasCJK reports whether s holds Chinese, Japanese or Korean letters,
// which bring a CJK font onto the line.
func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return true
		}
	}
	return false
}

// tipMargins are the top margins of a label (its text in a font of size
// and weight) and of its ⓘ in a row aligned at the start, which put the
// ⓘ's centre on the middle of the glyphs of the label's first line.
func tipMargins(t *ui.Theme, label string, size float32, weight int) (text, tip float32) {
	d := glyphMiddle(t, label, size, weight) - px(t, infoTipBox)/2
	return max(0, -d), max(0, d)
}

// labelTip gives text, a label's text just built in a row aligned at the
// start, and the ⓘ it builds after it (none for an empty tip) their top
// margins (tipMargins). size and weight are text's font's.
func labelTip(c *ui.Context, text ui.Element, label string, size float32, weight int, tip string) ui.Element {
	if tip == "" {
		return ui.Element{}
	}
	tm, im := tipMargins(c.Theme(), label, size, weight)
	text.Margin(tm, 0, 0, 0)
	return infoTip(c, tip).Margin(im, 0, 0, 0)
}
