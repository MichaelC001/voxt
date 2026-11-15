package layout

import (
	"math"
	"time"

	"github.com/hehehai/VoxtMini/internal/settings"
)

// The face's size changes (live text that comes and grows, the pill that
// turns into the answer card, the time, a card's line) move with a spring:
// Voxt's card (WaveformView.swift) springs its size with response 0.38 s
// and damping 0.78 when its mode changes. Ours never overshoots: the
// window holds the face's largest size of the move (Room), and a face
// past its target would leave it; each dimension stops where it reaches
// its target.
const (
	SpringResponse = 0.38 // seconds
	SpringDamping  = 0.8
	// FadeIn is how long a new mode's content takes to show (Voxt's
	// opacity transition), from none.
	FadeIn = 200 * time.Millisecond
	// motionStep is the spring's integration step; motionGap the longest
	// time between two steps it follows (a window that drew no frame for
	// longer goes to its target).
	motionStep = 2 * time.Millisecond
	motionGap  = 500 * time.Millisecond
)

// Shape is the face's shape: its width and height and its corners' radius,
// in DIPs.
type Shape struct{ W, H, R float64 }

func (s Shape) vec() [3]float64 { return [3]float64{s.W, s.H, s.R} }

func shapeOf(v [3]float64) Shape { return Shape{v[0], v[1], v[2]} }

// Motion is the face's shape on its way to a target. The zero value is a
// face at rest at the zero shape.
type Motion struct {
	pos, vel [3]float64
	// from is where the move to the target started: each dimension stays
	// between it and the target.
	from, to [3]float64
	at       time.Time
	moving   bool
}

// Snap puts the face at to, at rest.
func (m *Motion) Snap(to Shape) {
	m.pos, m.from, m.to = to.vec(), to.vec(), to.vec()
	m.vel = [3]float64{}
	m.moving = false
}

// Set gives the face a new target at now: it moves there from where it
// is, keeping its velocity (a target that changes on the way, as live
// text grows word by word, does not make it stop and start again).
func (m *Motion) Set(to Shape, now time.Time) {
	if to.vec() == m.to {
		return
	}
	m.Step(now)
	m.from, m.to = m.pos, to.vec()
	if !m.moving {
		m.vel = [3]float64{}
	}
	m.at, m.moving = now, true
}

// Step moves the face to now and says where it is.
func (m *Motion) Step(now time.Time) Shape {
	if !m.moving {
		return shapeOf(m.pos)
	}
	d := now.Sub(m.at)
	if d < 0 {
		return shapeOf(m.pos)
	}
	if d > motionGap {
		m.Snap(shapeOf(m.to))
		return shapeOf(m.pos)
	}
	w := 2 * math.Pi / SpringResponse
	k, c := w*w, 2*SpringDamping*w
	n := int(d / motionStep)
	dt := motionStep.Seconds()
	for range n {
		for i := range m.pos {
			a := -k*(m.pos[i]-m.to[i]) - c*m.vel[i]
			m.vel[i] += a * dt
			m.pos[i] += m.vel[i] * dt
			lo, hi := min(m.from[i], m.to[i]), max(m.from[i], m.to[i])
			if m.pos[i] <= lo || m.pos[i] >= hi {
				m.pos[i] = min(max(m.pos[i], lo), hi)
				if m.pos[i] == m.to[i] || (m.vel[i] < 0) == (m.pos[i] == lo) {
					m.vel[i] = 0
				}
			}
		}
	}
	m.at = m.at.Add(time.Duration(n) * motionStep)
	if m.near() {
		m.Snap(shapeOf(m.to))
	}
	return shapeOf(m.pos)
}

// near says whether the face is close enough to its target, and slow
// enough, to be there.
func (m *Motion) near() bool {
	for i := range m.pos {
		if math.Abs(m.pos[i]-m.to[i]) > 0.25 || math.Abs(m.vel[i]) > 4 {
			return false
		}
	}
	return true
}

// Moving says whether the face is on its way.
func (m *Motion) Moving() bool { return m.moving }

// Now is where the face is, as of the last step.
func (m *Motion) Now() Shape { return shapeOf(m.pos) }

// Target is where the face goes.
func (m *Motion) Target() Shape { return shapeOf(m.to) }

// Room is the face's room in the window while it moves: the larger of
// where it is and its target, whole DIPs; the window keeps the largest
// room of a move until the face is at rest.
func Room(m *Motion) (w, h int) {
	p, t := m.Now(), m.Target()
	return int(math.Ceil(max(p.W, t.W) - 1e-9)), int(math.Ceil(max(p.H, t.H) - 1e-9))
}

// Inset is where the face of shape s is in a window holding a room of
// roomW × roomH (and the shadow's margin around it): the material's
// insets from the window's edges. The face is centred, and on the room's
// anchor edge (Overlay.Position): it grows and shrinks from that edge,
// as the window does.
func Inset(o settings.Overlay, roomW, roomH int, s Shape) (top, right, bottom, left float64) {
	side := Pad + (float64(roomW)-s.W)/2
	free := float64(roomH) - s.H
	top, bottom = Pad+free, Pad
	if o.Position == "top" {
		top, bottom = Pad, Pad+free
	}
	return top, side, bottom, side
}

// ContentAlpha is how opaque a new mode's content is d after the mode
// changed: from none to whole over FadeIn, easing out.
func ContentAlpha(d time.Duration) float64 {
	if d >= FadeIn {
		return 1
	}
	if d <= 0 {
		return 0
	}
	u := 1 - float64(d)/float64(FadeIn)
	return 1 - u*u*u
}
