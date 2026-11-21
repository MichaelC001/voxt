package main

// The overlay's size changes move (overlay.go): the live text that comes
// and grows, the pill that turns into the answer card, the time, a
// notice, a card's line. The face springs to its new size
// (layout.Motion, Voxt's spring) about its anchor edge, as the window did
// at once before.
//
// The window does not resize every frame: when a change starts, it takes
// the room the move needs (the larger of the face and its target, the
// face staying where it is in it), and the face's shape moves inside, the
// window's material (SetVibrancyStyle) inset to it frame by frame from the
// overlay's view, which draws that frame; once the face rests, the window
// shrinks to it again. The content keeps its new size from the start,
// centred and from the face's top, the face clipping it: text does not
// reflow on the way; a new face (the pill, a notice, the card) fades in.
// Frames come only while the face moves.
//
// Nothing moves with reduced motion (the app's setting, or macOS's), when
// the window shows again (it shows the new face at once, overlay.go's
// reveal), nor in tests without moveHook.

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/overlay/layout"
)

// ovTime is the overlay's time: the clock's in tests.
func (a *app) ovTime() time.Time {
	if a.clock != nil {
		return a.clock()
	}
	return time.Now()
}

// overlayFit gives the face its new size w × h in mode: it moves there
// when it shows and may move (move), else it is there at once. It says
// the window's frame: the face's room while it moves.
func (a *app) overlayFit(mode overlayMode, w, h int, move bool) mygo.Rectangle {
	to := layout.Shape{W: float64(w), H: float64(h), R: float64(faceRadius(a.prefs.Overlay, mode, h))}
	move = move && (a.ov.opaque || !a.overlayNative()) && (a.overlayNative() || a.moveHook) && !a.prefs.Appearance.ReduceMotion
	if move {
		now := a.ovTime()
		if faceKind(mode) != faceKind(a.ov.mode) {
			a.ov.modeAt = now
		}
		a.ov.motion.Set(to, now)
	} else {
		a.ov.motion.Snap(to)
		a.ov.modeAt = time.Time{}
	}
	rw, rh := w, h
	if a.ov.motion.Moving() {
		mw, mh := layout.Room(&a.ov.motion)
		rw, rh = max(mw, a.ov.roomW), max(mh, a.ov.roomH)
	}
	a.ov.roomW, a.ov.roomH = rw, rh
	return layout.Frame(a.ov.area, a.prefs.Overlay, rw, rh)
}

// faceKind tells the faces apart whose content differs: the pill (the
// recording's and the work's), a notice, the card.
func faceKind(m overlayMode) overlayMode {
	if m == ovWorking {
		return ovRecording
	}
	return m
}

// overlayShape is the face's shape in this frame of the overlay's view:
// it moves the face to now, keeps frames coming while it moves, insets
// the window's material to it in the same frame, and once the face
// rests shrinks the window to it (on the next turn of the main loop: the
// build resizes nothing). Main thread, in the build.
func (a *app) overlayShape(c *ui.Context) layout.Shape {
	m := &a.ov.motion
	if m.Moving() && c.Preferences().ReduceMotion {
		m.Snap(m.Target())
		a.ov.modeAt = time.Time{}
	}
	s := m.Step(a.ovTime())
	if m.Moving() || a.contentAlpha() < 1 {
		c.AnimationFrame()
	}
	a.syncOverlayMaterial()
	if !m.Moving() && (a.ov.roomW != a.ov.w || a.ov.roomH != a.ov.h) {
		a.post(a.overlaySettle)
	}
	return s
}

// overlaySettle shrinks the window to the face once it rests.
func (a *app) overlaySettle() {
	if !a.ov.shown || a.ov.motion.Moving() || (a.ov.roomW == a.ov.w && a.ov.roomH == a.ov.h) {
		return
	}
	a.ov.roomW, a.ov.roomH = a.ov.w, a.ov.h
	frame := layout.Frame(a.ov.area, a.prefs.Overlay, a.ov.w, a.ov.h)
	moved := frame != a.ov.frame
	a.ov.frame = frame
	if a.ov.win == nil {
		return
	}
	if moved {
		a.ov.win.SetContentBounds(frame)
	}
	a.syncOverlayMaterial()
	a.ov.win.Invalidate()
}

// contentAlpha is how opaque the face's content is: a new face's fades in.
func (a *app) contentAlpha() float64 {
	if a.ov.modeAt.IsZero() {
		return 1
	}
	v := layout.ContentAlpha(a.ovTime().Sub(a.ov.modeAt))
	if v >= 1 {
		a.ov.modeAt = time.Time{}
	}
	return v
}

// syncOverlayMaterial shapes the window's material to the face where it
// is: inside the shadow's margin and its room's free part, with the
// face's corners, dark.
func (a *app) syncOverlayMaterial() {
	if a.ov.win == nil {
		return
	}
	s := a.ov.motion.Now()
	top, right, bottom, left := layout.Inset(a.prefs.Overlay, a.ov.roomW, a.ov.roomH, s)
	style := mygo.VibrancyStyle{Top: top, Right: right, Bottom: bottom, Left: left, Radius: s.R, Dark: true}
	if style != a.ov.style {
		a.ov.style = style
		a.ov.win.SetVibrancyStyle(style)
	}
}

// faceFit sizes the face as it moves: s is its shape now, w × h its
// target, at which its content lays out (centred, from the face's top,
// clipped by it), alpha the content's opacity. The zero value leaves the
// face filling its parent (the settings' preview).
type faceFit struct {
	s     layout.Shape
	w, h  int
	alpha float32
}

// face sizes e, the face.
func (f faceFit) face(e ui.Element) ui.Element {
	if f.w == 0 {
		return e
	}
	e = e.Size(float32(f.s.W), float32(f.s.H)).Shrink(0).Radius(float32(f.s.R)).AlignItems(ui.Center).Justify(ui.Start)
	if f.s.W < float64(f.w) || f.s.H < float64(f.h) {
		e = e.Clip() // only while smaller than its content: a clip shades the edge's pixels a little
	}
	return e
}

// content lays out fn, the face's content, at the target size.
func (f faceFit) content(c *ui.Context, fn func()) {
	if f.w == 0 {
		fn()
		return
	}
	e := ui.Column(c).Size(float32(f.w-2*layout.Border), float32(f.h-2*layout.Border)).Shrink(0)
	if f.alpha < 1 {
		e.Opacity(f.alpha)
	}
	e.Children(fn)
}

// overlayFaceFit is the face's fit in this frame (overlayShape); none
// before the overlay showed.
func (a *app) overlayFaceFit(c *ui.Context) faceFit {
	if a.ov.w == 0 {
		return faceFit{}
	}
	s := a.overlayShape(c)
	return faceFit{s: s, w: a.ov.w, h: a.ov.h, alpha: float32(a.contentAlpha())}
}

// overlayJustify places the face in its room: on the anchor edge.
func (a *app) overlayJustify() ui.Align {
	if a.prefs.Overlay.Position == "top" {
		return ui.Start
	}
	return ui.End
}
