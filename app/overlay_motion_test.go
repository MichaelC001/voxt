package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/deliver"
	"github.com/hehehai/VoxtMini/internal/overlay/layout"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// motionApp is a test app whose overlay moves (moveHook) on a clock the
// test sets, its posts queued until runPosts.
type motionApp struct {
	*app
	now   time.Time
	posts []func()
}

func newMotionApp(t *testing.T, pos string) *motionApp {
	a, _, _ := newTestApp(t)
	m := &motionApp{app: a, now: time.Unix(1_800_000_000, 0)}
	a.updateSettings(func(s *settings.Settings) { s.Overlay.Position = pos })
	a.moveHook = true
	a.clock = func() time.Time { return m.now }
	a.postQueue = func(fn func()) { m.posts = append(m.posts, fn) }
	return m
}

func (m *motionApp) runPosts() {
	for len(m.posts) > 0 {
		p := m.posts
		m.posts = nil
		for _, fn := range p {
			fn()
		}
	}
}

// render draws the overlay's window, as large as its frame now, at
// scale, and runs what its view posted.
func (m *motionApp) render(scale float32) *image.RGBA {
	tt := hudTester(m.overlayView, m.ov.frame.Width, m.ov.frame.Height)
	tt.SetScale(scale)
	frames(tt)
	img := tt.Image()
	m.runPosts()
	return img
}

// faceBox is the face's box in img (scale 1): its opaque pixels'.
func faceBox(img image.Image) image.Rectangle {
	b := img.Bounds()
	r := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a>>8 > 128 {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

// TestOverlaySizeMoves: a size change of the shown overlay moves (Voxt's
// spring, about a quarter of a second), not jumps: the pill growing with
// live text, the card's line going. The window takes the move's room at
// once, the face staying on its anchor edge and centred in it, growing
// frame by frame, and shrinks to the face once it rests; reduced motion
// jumps.
func TestOverlaySizeMoves(t *testing.T) {
	for _, pos := range []string{"bottom", "top"} {
		t.Run(pos, func(t *testing.T) {
			m := newMotionApp(t, pos)
			o := m.prefs.Overlay
			snap(m.app, session.Snapshot{ID: "m1", State: session.Recording})
			if m.ov.motion.Moving() || m.ov.frame != pillFrame(o) {
				t.Fatalf("shows moving: %+v", m.ov.frame)
			}
			m.render(1)
			snap(m.app, session.Snapshot{ID: "m1", State: session.Recording, Partial: "Meet at three tomorrow, then lunch"})
			if !m.ov.motion.Moving() || m.ov.h != layout.LiveH {
				t.Fatalf("the live text: moving %v, %d×%d", m.ov.motion.Moving(), m.ov.w, m.ov.h)
			}
			target := layout.Frame(ovTestArea, o, m.ov.w, m.ov.h)
			if m.ov.frame != target {
				t.Errorf("the window %+v, the move's room %+v", m.ov.frame, target)
			}
			anchored(t, "room", o, m.ov.frame)
			prev := image.Rect(0, 0, 0, 0)
			moved := 0
			start := m.now
			for i := range 20 {
				m.now = start.Add(time.Duration(i) * 25 * time.Millisecond)
				box := faceBox(m.render(1))
				w, h := box.Dx(), box.Dy()
				if i == 0 && (w > layout.PillW+1 || h > layout.PillH+1) {
					t.Errorf("starts at %d×%d", w, h)
				}
				if w < prev.Dx() || h < prev.Dy() || w > m.ov.w+1 || h > m.ov.h+1 {
					t.Errorf("frame %d: %v after %v", i, box, prev)
				}
				// On its anchor edge, centred.
				if pos == "top" && box.Min.Y != layout.Pad || pos == "bottom" && box.Max.Y != m.ov.frame.Height-layout.Pad {
					t.Errorf("frame %d: %v off its edge", i, box)
				}
				if d := box.Min.X + box.Max.X - m.ov.frame.Width; d < -1 || d > 1 {
					t.Errorf("frame %d: %v off centre", i, box)
				}
				if box != prev && i > 0 {
					moved++
				}
				prev = box
			}
			if moved < 5 {
				t.Errorf("moved in %d frames", moved)
			}
			if m.ov.motion.Moving() || prev.Dx() < m.ov.w-1 || m.ov.frame != target {
				t.Errorf("rests at %v, moving %v, frame %+v", prev, m.ov.motion.Moving(), m.ov.frame)
			}

			// The card, then its line going: the window keeps the room until
			// the face rests, then shrinks to it on its anchor.
			snap(m.app, session.Snapshot{ID: "m1", State: session.Done, Card: true, Delivery: deliver.ReasonNotEditable, Result: "Meet at three."})
			m.now = m.now.Add(time.Second)
			m.render(1)
			m.answer.msg = "Not inserted"
			m.overlayRestyle()
			m.now = m.now.Add(time.Second)
			m.render(1)
			high := m.ov.frame
			m.answer.msg = ""
			m.overlayRestyle()
			if !m.ov.motion.Moving() || m.ov.frame != high {
				t.Fatalf("shrinking: moving %v, frame %+v (was %+v)", m.ov.motion.Moving(), m.ov.frame, high)
			}
			m.now = m.now.Add(100 * time.Millisecond)
			box := faceBox(m.render(1))
			if box.Dy() >= high.Height-2*layout.Pad || box.Dy() <= m.ov.h {
				t.Errorf("half way: %v, from %d to %d", box, high.Height-2*layout.Pad, m.ov.h)
			}
			m.now = m.now.Add(time.Second)
			m.render(1)
			if m.ov.frame != layout.Frame(ovTestArea, o, m.ov.w, m.ov.h) {
				t.Errorf("rested: frame %+v", m.ov.frame)
			}
			anchored(t, "shrunk", o, m.ov.frame)

			// Reduced motion: at once.
			m.prefs.Appearance.ReduceMotion = true
			m.answer.msg = "Not inserted"
			m.overlayRestyle()
			if m.ov.motion.Moving() || m.ov.frame != layout.Frame(ovTestArea, o, m.ov.w, m.ov.h) {
				t.Errorf("reduced motion moves: %+v", m.ov.frame)
			}
		})
	}
}

// TestOverlayMotionShots draws frame strips of the overlay's size changes
// (en and zh-Hans, over a light desktop, every 50 ms), with the motion
// (after) and without (before):
//
//	VOXTMINI_MOTION_SHOTS=/tmp/mo go test -run TestOverlayMotionShots ./app
func TestOverlayMotionShots(t *testing.T) {
	out := os.Getenv("VOXTMINI_MOTION_SHOTS")
	if out == "" {
		t.Skip("set VOXTMINI_MOTION_SHOTS to a directory")
	}
	requireCJKFont(t)
	for _, run := range []struct {
		lang, live, result string
		move               bool
	}{
		{"en", "Meet at three tomorrow to plan the release", "Hello team, the release moves to Thursday: the onboarding and the history page first.", false},
		{"en", "Meet at three tomorrow to plan the release", "Hello team, the release moves to Thursday: the onboarding and the history page first.", true},
		{"zh-Hans", "明天下午三点开会，讨论发布计划", "大家好，发布推迟到周四：先把引导页和历史页做完，再看翻译卡片。", false},
		{"zh-Hans", "明天下午三点开会，讨论发布计划", "大家好，发布推迟到周四：先把引导页和历史页做完，再看翻译卡片。", true},
	} {
		name := run.lang + "-before"
		if run.move {
			name = run.lang + "-after"
		}
		live := run.live
		strips := []struct {
			name          string
			before, after session.Snapshot
		}{
			{"pill-to-live", session.Snapshot{ID: "m1", State: session.Recording, Levels: wave()},
				session.Snapshot{ID: "m1", State: session.Recording, Levels: wave(), Partial: live}},
			{"live-to-card", session.Snapshot{ID: "m1", State: session.Composing, Partial: live},
				session.Snapshot{ID: "m1", State: session.Done, Card: true, Delivery: deliver.ReasonNotEditable,
					Result: run.result}},
		}
		for _, st := range strips {
			m := newMotionApp(t, "bottom")
			m.updateSettings(func(s *settings.Settings) { s.General.Language = run.lang })
			m.moveHook = run.move
			m.measureHook = nil
			snap(m.app, session.Snapshot{ID: "m1", State: session.Recording, Levels: wave()})
			snap(m.app, st.before)
			for range 3 { // the texts measured, the face at rest
				m.now = m.now.Add(time.Second)
				m.render(2)
			}
			var shots []*image.RGBA
			var frames []mygo.Rectangle
			shot := func() {
				shots = append(shots, m.render(2))
				frames = append(frames, m.ov.frame)
			}
			shot()
			heard := st.after
			heard.HeardAt = m.now
			snap(m.app, heard)
			start := m.now
			for i := range 7 {
				m.now = start.Add(time.Duration(i) * 50 * time.Millisecond)
				shot()
			}
			writeStrip(t, filepath.Join(out, st.name+"-"+name+".png"), shots, frames)
		}
	}
}

// writeStrip lays the overlay's windows side by side over a light
// desktop, each where it is on the screen (its frame), at scale 2.
func writeStrip(t *testing.T, path string, shots []*image.RGBA, frames []mygo.Rectangle) {
	top, bottom, w := frames[0].Y, frames[0].Y+frames[0].Height, 0
	for _, f := range frames {
		top, bottom, w = min(top, f.Y), max(bottom, f.Y+f.Height), max(w, f.Width)
	}
	const gap = 12
	img := image.NewRGBA(image.Rect(0, 0, 2*(len(shots)*(w+gap)+gap), 2*(bottom-top+2*gap)))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{246, 246, 248, 255}}, image.Point{}, draw.Src)
	for i, s := range shots {
		f := frames[i]
		x := 2 * (gap + i*(w+gap) + (w-f.Width)/2)
		y := 2 * (gap + f.Y - top)
		draw.Draw(img, s.Bounds().Add(image.Pt(x, y)), s, image.Point{}, draw.Over)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}
