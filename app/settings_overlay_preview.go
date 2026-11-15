package main

// Settings → Overlay style's preview (settings_overlay.go), after
// Inboxly's notch preview: at the top of the section, a switch of the
// overlay's states over a strip of desktop, and the overlay in the state
// chosen, drawn by the overlay's own code (overlayPill, cardFace,
// noticeFace) with the section's settings as they move: at the bottom or
// the top of the strip, its distance from that edge across the strip's
// room, as opaque, as round, with the time and the live text when they
// show. The strip is a desktop scaled down: the overlay keeps its size
// against it, so a narrow dialog shows both smaller. It is a picture: a
// cover over it takes the clicks, nothing in it runs.

import (
	_ "embed"
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/overlay/layout"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// The preview's states, in the order of its switch.
const (
	ovPreviewRecording = iota // the pill and its waveform; the live text under it
	ovPreviewWorking          // the loader; the answer being written under it
	ovPreviewAnswer           // a Rewrite answer's card
	ovPreviewNotice           // an error's notice
	ovPreviewStates
)

// ovPreviewNames name the states in tests and snapshots.
var ovPreviewNames = [ovPreviewStates]string{"recording", "working", "answer", "notice"}

// ovPreviewLabels are the switch's labels.
func ovPreviewLabels() []string {
	return []string{i18n.T("Recording"), i18n.X("overlay preview", "Processing"), i18n.X("overlay preview", "Answer"), i18n.X("overlay preview", "Error")}
}

// The stage: a piece of desktop ovStageW × ovStageH overlay DIPs, scaled
// down to the section's width (ovStageMinScale at the least), its corners
// radiusCard; the face at least ovPreviewInset from its edge, the
// distance from the edge, from none to ovMaxMargin, across the room the
// face leaves.
const (
	ovStageW        = 560
	ovStageH        = 156
	ovStageMinScale = 0.6
	ovPreviewInset  = 12
)

// previewSession is the recording the preview shows: Transcribe, seven
// seconds in, a few words on the waveform (bars 4 to 20 high, as speech
// moves them). The overlay check (overlay_check.go) takes its levels.
func previewSession() session.Snapshot {
	t0 := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	levels := []float64{0.22, 0.38, 0.52, 0.66, 0.3, 0.2, 0.36, 0.6, 0.76, 0.42, 0.22, 0.4, 0.58, 0.32, 0.24, 0.48}
	return session.Snapshot{Kind: store.KindTranscribe, State: session.Recording, StartedAt: t0, HeardAt: t0, StoppedAt: t0.Add(7 * time.Second), Levels: levels}
}

// ovPreviewTexts are the preview's sample texts in the UI's language:
// what is said, as it is recognised; the cleaned-up text being written;
// a Rewrite answer.
func ovPreviewTexts() (said, draft, answer string) {
	if i18n.Locale() == "zh-Hans" {
		return "明天下午三点开会，讨论一下发布计划", "明天下午 3 点开会，讨论发布计划。",
			"大家好，发布改到周四，引导页和历史页先完成。谢谢大家！"
	}
	return "Let's meet at three tomorrow", "Let's meet at 3 pm tomorrow.",
		"Hi team, the release moves to Thursday: the onboarding and the history page come first. Thanks!"
}

// previewPillSession is the session the pill shows in state, its live
// text the overlay's (pillSession): none when it is hidden.
func previewPillSession(state int, o settings.Overlay) session.Snapshot {
	s := previewSession()
	said, draft, _ := ovPreviewTexts()
	s.Partial = said
	if state == ovPreviewWorking {
		s.State, s.Draft = session.Composing, draft
	}
	if o.HideLiveText {
		s.Partial, s.Draft = "", ""
	}
	return s
}

// previewCard is the Rewrite answer the card shows: its actions as after
// a session whose answer could not be inserted.
func previewCard() cardView {
	_, _, answer := ovPreviewTexts()
	return cardView{kind: store.KindRewrite, title: uitext.CardTitle(store.KindRewrite, ""), text: answer,
		canInsert: true, canCopy: true, canFollowUp: true}
}

// previewNotice is the notice the preview shows: an error with an action.
func previewNotice() *sessionNotice {
	text, more, action := sessionProblemText(&session.Problem{Code: session.ProblemMicrophone})
	return &sessionNotice{text: text, more: more, action: action}
}

// The preview's background: a desktop picture (Voxt's, Apache-2.0: see
// NOTICE), decoded once.
//
//go:embed assets/overlay-preview-background.png
var ovPreviewPNG []byte

var ovPreviewBackground = sync.OnceValue(func() *ui.Bitmap {
	b, _ := ui.DecodeBitmap(ovPreviewPNG)
	return b
})

// overlayPreview is the preview: its switch, then the stage.
func (a *app) overlayPreview(c *ui.Context) {
	t := c.Theme()
	f := &a.settingsForm.ovStyle
	f.preview = min(max(f.preview, 0), ovPreviewStates-1)
	ui.Column(c.Key("ov-preview-group")).Gap(px(t, sp2)).Shrink(0).Children(func() {
		segmentedFit(c.Key("ov-preview-state"), &f.preview, ovPreviewLabels()...).Label(i18n.T("Preview state"))
		a.overlayStage(c)
	})
}

// overlayStage is the strip of desktop and the overlay on it, in the
// state chosen, scaled to the section's width (as last laid out).
func (a *app) overlayStage(c *ui.Context) {
	f := &a.settingsForm.ovStyle
	o := a.prefs.Overlay
	s := float32(0.8)
	if f.previewW > 0 {
		s = min(max(f.previewW/ovStageW, ovStageMinScale), 1)
	}
	box := ui.Box(c.Key("ov-preview")).FillWidth().MinWidth(0).Height(float32(math.Round(float64(ovStageH * s)))).Shrink(0).
		Radius(radiusCard).Clip().Background(ui.RGB(40, 90, 170)).Role(ui.RoleGroup).Label(i18n.T("Preview of the overlay"))
	f.previewW = box.Bounds().W
	box.Children(func() {
		if bg := ovPreviewBackground(); bg != nil {
			ui.Image(c, bg).Absolute().Left(0).Top(0).Right(0).Bottom(0).Fit(ui.Cover)
		}
		stageW := max(f.previewW/s, ovStageW)
		stage := ui.Box(c.Key("ov-preview-stage")).Absolute().Top(0).Left(0).Size(stageW, ovStageH).TransformOrigin(0, 0).Scale(s, s)
		stage.Children(func() {
			old := c.Theme()
			c.SetTheme(overlayTheme())
			a.previewFace(c, o, f.preview)
			c.SetTheme(old)
		})
		// Over it all, it takes the clicks: nothing in the preview runs.
		ui.Box(c.Key("ov-preview-cover")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	})
}

// previewFace places the overlay's face in state on the stage, where o
// puts it, and draws it with the overlay's code.
func (a *app) previewFace(c *ui.Context, o settings.Overlay, state int) {
	measure := func(k ovSized) (float32, float32) { return c.MeasureText(k.wrap, k.span()) }
	var w, h int
	var draw func()
	switch state {
	case ovPreviewAnswer:
		v := previewCard()
		body := float32(0)
		for _, b := range cardBlocks(v) {
			_, bh := measure(b.text)
			body += b.gap + bh
		}
		w, h = layout.AnswerW, layout.CardChrome+min(int(math.Ceil(float64(body))), layout.CardBodyMax)
		draw = func() {
			faceStyle(ui.Column(c.Key("ov-preview-face")), o, ovAnswer, h).Children(func() {
				cardFace{v: v, h: h, top: o.Position == "top"}.view(c)
			})
		}
	case ovPreviewNotice:
		n := previewNotice()
		tw, _ := measure(noticeText(n.text))
		w, h = layout.NoticeWidth(tw, noticeActionLabel(n.action) != ""), layout.PillH
		draw = func() { noticeFace(c, n, o, h, faceFit{}, nil) }
	default:
		mode := ovRecording
		if state == ovPreviewWorking {
			mode = ovWorking
		}
		s := previewPillSession(state, o)
		row := layout.RowWidth(mode == ovRecording && o.ShowTime, 0)
		w, h = row+2*layout.PillPadX, layout.PillH
		if text, _ := pillLine(s); text != "" {
			tw, _ := measure(liveTextKey(text))
			w, h = layout.LiveWidth(row, tw), layout.LiveH
		}
		draw = func() {
			faceStyle(ui.Column(c.Key("ov-preview-face")), o, mode, h).Children(func() {
				overlayPill(c, s, mode, pillLook{time: o.ShowTime, w: w})
			})
		}
	}
	room := max(ovStageH-h-2*ovPreviewInset, 0)
	off := float32(ovPreviewInset) + float32(room)*float32(min(max(o.Margin, 0), ovMaxMargin)/ovMaxMargin)
	place := ui.Row(c.Key("ov-preview-place")).Absolute().Left(0).Right(0).Height(float32(h)).Justify(ui.Center)
	if o.Position == "top" {
		place.Top(off)
	} else {
		place.Bottom(off)
	}
	place.Children(func() {
		ui.Box(c.Key("ov-preview-pill")).Size(float32(w), float32(h)).Shrink(0).Children(func() {
			// The blur of what is under it (the window's material, on the
			// Mac), then the face, positioned too to paint over the blur.
			ui.Box(c).Absolute().Fill().Radius(faceRadius(o, ovModeOf(state), h)).Material(glass.Blur{Radius: 8})
			ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Children(draw)
		})
	})
}

// ovModeOf is the overlay's mode for a preview state.
func ovModeOf(state int) overlayMode {
	return [ovPreviewStates]overlayMode{ovRecording, ovWorking, ovAnswer, ovNotice}[state]
}
