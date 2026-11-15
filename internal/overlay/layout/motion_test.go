package layout

import (
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/settings"
)

// run steps m every frame from t0 until it rests (at most 2 s), and gives
// the shapes it went through.
func run(m *Motion, t0 time.Time, frame time.Duration) ([]Shape, time.Duration) {
	var out []Shape
	for d := frame; d <= 2*time.Second; d += frame {
		out = append(out, m.Step(t0.Add(d)))
		if !m.Moving() {
			return out, d
		}
	}
	return out, 2 * time.Second
}

func TestMotionGrows(t *testing.T) {
	t0 := time.Unix(1000, 0)
	var m Motion
	pill := Shape{PillW, PillH, 24}
	live := Shape{LiveMaxW, LiveH, 24}
	m.Snap(pill)
	if m.Moving() || m.Step(t0) != pill {
		t.Fatalf("snapped: %+v moving %v", m.Now(), m.Moving())
	}
	m.Set(live, t0)
	if !m.Moving() || m.Now() != pill {
		t.Fatalf("set: %+v moving %v", m.Now(), m.Moving())
	}
	if w, h := Room(&m); w != LiveMaxW || h != LiveH {
		t.Errorf("room %d×%d", w, h)
	}
	shapes, took := run(&m, t0, 16*time.Millisecond)
	// Voxt's spring (response 0.38 s): about a quarter of a second.
	if took < 180*time.Millisecond || took > 400*time.Millisecond {
		t.Errorf("took %v", took)
	}
	if m.Now() != live {
		t.Errorf("rests at %+v", m.Now())
	}
	prev := pill
	for i, s := range shapes {
		// Never past the target nor back: the window holds the move.
		if s.W < prev.W || s.H < prev.H || s.W > live.W || s.H > live.H {
			t.Fatalf("frame %d: %+v after %+v", i, s, prev)
		}
		prev = s
	}
	// It eases: the first frames move more than the last.
	if len(shapes) < 6 {
		t.Fatalf("%d frames", len(shapes))
	}
	first := shapes[2].W - shapes[1].W
	last := shapes[len(shapes)-2].W - shapes[len(shapes)-3].W
	if first <= last {
		t.Errorf("does not ease out: %v then %v", first, last)
	}
}

func TestMotionShrinks(t *testing.T) {
	t0 := time.Unix(1000, 0)
	var m Motion
	card := Shape{AnswerW, 200, 24}
	small := Shape{AnswerW, 120, 24}
	m.Snap(card)
	m.Set(small, t0)
	if w, h := Room(&m); w != AnswerW || h != 200 {
		t.Errorf("room %d×%d", w, h)
	}
	shapes, _ := run(&m, t0, 8*time.Millisecond)
	for i, s := range shapes {
		if s.H > 200 || s.H < 120 || s.W != AnswerW {
			t.Fatalf("frame %d: %+v", i, s)
		}
	}
	if m.Now() != small || m.Moving() {
		t.Errorf("rests at %+v moving %v", m.Now(), m.Moving())
	}
}

// A target that changes on the way (live text growing word by word)
// keeps the face moving the same way, without a stop.
func TestMotionRetarget(t *testing.T) {
	t0 := time.Unix(1000, 0)
	var m Motion
	m.Snap(Shape{200, LiveH, 24})
	m.Set(Shape{230, LiveH, 24}, t0)
	mid := m.Step(t0.Add(80 * time.Millisecond))
	v := m.vel[0]
	if mid.W <= 200 || mid.W >= 230 || v <= 0 {
		t.Fatalf("on the way: %+v v %v", mid, v)
	}
	m.Set(Shape{260, LiveH, 24}, t0.Add(80*time.Millisecond))
	if m.vel[0] != v || m.Now() != mid {
		t.Errorf("retarget lost the move: %+v v %v", m.Now(), m.vel[0])
	}
	next := m.Step(t0.Add(96 * time.Millisecond))
	if next.W <= mid.W {
		t.Errorf("stopped: %v then %v", mid.W, next.W)
	}
	run(&m, t0.Add(96*time.Millisecond), 16*time.Millisecond)
	if m.Now().W != 260 {
		t.Errorf("rests at %+v", m.Now())
	}
	// Back the other way: it turns without passing where it was.
	m.Set(Shape{300, LiveH, 24}, t0.Add(2*time.Second))
	m.Step(t0.Add(2*time.Second + 60*time.Millisecond))
	at := m.Now().W
	m.Set(Shape{200, LiveH, 24}, t0.Add(2*time.Second+60*time.Millisecond))
	s := m.Step(t0.Add(2*time.Second + 76*time.Millisecond))
	if s.W > at || s.W < 200 {
		t.Errorf("turned to %v from %v", s.W, at)
	}
}

// A window that drew nothing for a while (hidden) finds the face at
// its target; time going back moves nothing.
func TestMotionGap(t *testing.T) {
	t0 := time.Unix(1000, 0)
	var m Motion
	m.Snap(Shape{PillW, PillH, 24})
	m.Set(Shape{AnswerW, 200, 20}, t0)
	if s := m.Step(t0.Add(-time.Second)); s != (Shape{PillW, PillH, 24}) || !m.Moving() {
		t.Errorf("back in time: %+v", s)
	}
	if s := m.Step(t0.Add(time.Second)); s != (Shape{AnswerW, 200, 20}) || m.Moving() {
		t.Errorf("after a gap: %+v moving %v", s, m.Moving())
	}
	m.Set(Shape{AnswerW, 200, 20}, t0.Add(2*time.Second))
	if m.Moving() {
		t.Error("the same target moves")
	}
}

func TestInset(t *testing.T) {
	s := Shape{W: 200, H: 50}
	top, right, bottom, left := Inset(settings.Overlay{Position: "bottom"}, 300, 76, s)
	if top != Pad+26 || bottom != Pad || left != Pad+50 || right != Pad+50 {
		t.Errorf("bottom: %v %v %v %v", top, right, bottom, left)
	}
	top, _, bottom, _ = Inset(settings.Overlay{Position: "top"}, 300, 76, s)
	if top != Pad || bottom != Pad+26 {
		t.Errorf("top: %v %v", top, bottom)
	}
}

func TestContentAlpha(t *testing.T) {
	if ContentAlpha(0) != 0 || ContentAlpha(FadeIn) != 1 || ContentAlpha(-time.Second) != 0 {
		t.Error("ends")
	}
	if a := ContentAlpha(FadeIn / 2); a < 0.8 || a >= 1 {
		t.Errorf("half way %v: eases out", a)
	}
}
