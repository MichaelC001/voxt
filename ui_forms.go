package main

// The kit's forms and dialogs (ui_kit.go): segmented controls, the
// dialog frame and modals.

import (
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
)

// segmentedFit is ui.Segmented as wide as its segments, for a form's
// column: there a plain Segmented stretches across, its segments keeping
// their width and leaving the rest of the track empty.
func segmentedFit(c *ui.Context, selected *int, labels ...string) ui.Element {
	return ui.Segmented(c, selected, labels...).AlignSelf(ui.Start)
}

// segmentedSome is ui.Segmented (as wide as its segments, as
// segmentedFit) whose segments may be off: off[i] true greys segment i
// out, and tips[i] ("" for none) says why, as its tooltip. Its Changed
// reports a new choice.
func segmentedSome(c *ui.Context, selected *int, labels []string, off []bool, tips []string) ui.Element {
	t := c.Theme()
	parts := ui.SegmentedBase(c, selected, len(labels))
	tr := parts.Track.AlignSelf(ui.Start).Shrink(0).AlignItems(ui.Stretch).Padding(t.Space(0.5)).Gap(t.Space(0.5)).
		Radius(t.Radius).Background(t.Surface).Border(1, t.Border)
	tr.Children(func() {
		for i, label := range labels {
			s := parts.Segment(i).Padding(t.Space(1), t.Space(3)).Radius(nestedRadius(t.Radius, t.Space(0.5))).Center().Label(label)
			dis := i < len(off) && off[i]
			if dis {
				s.Disabled(true)
			}
			if i < len(tips) && tips[i] != "" {
				s.Tooltip(tips[i])
			}
			switch {
			case i == *selected:
				face := t.Background
				if t.Dark {
					face = t.SurfacePressed
				}
				s.Background(face).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
			case dis:
			case s.Pressed():
				s.Background(t.SurfacePressed)
			case s.Hovered():
				s.Background(t.SurfaceHover)
			}
			s.Children(func() {
				tx := ui.Text(c, label).SingleLine()
				if dis {
					tx.TextColor(tok(t).Text3)
				}
			})
		}
	})
	return tr
}

// dialogFrame is a dialog in the look of the Settings dialog: a panel
// over the dimmed page, at most width wide and as tall as its content up
// to the window's height less margins, with a title row (title, a
// caption under it, "" for none, and ×), body in a scroll, and footer
// (the dialog's actions: one primaryButton at most) pinned under a
// hairline. Escape, a click beside it or × close it (*open false).
func dialogFrame(c *ui.Context, key string, open *bool, title, sub string, width float32, body, footer func()) ui.Element {
	t := c.Theme()
	k := tok(t)
	w, h := c.Size()
	margin := px(t, sp6)
	return ui.DialogBase(c.Key(key), open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.32))
		panel.Width(min(w-2*margin, width)).MaxHeight(h-2*margin).Radius(px(t, radiusDialog)).Background(k.Bg).Clip().
			Shadow(0, 16, 48, 0, ui.RGBA(0, 0, 0, 0.28)).Label(title)
		ui.Column(c).Grow(1).MinHeight(0).Children(func() {
			ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(px(t, sp4), px(t, sp3), px(t, sp2), px(t, sp6)).Gap(px(t, sp2)).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
					sectionHeading(c, title)
					if sub != "" {
						truncText(c, sub).FontSize(px(t, fsCaption)).TextColor(k.Text2)
					}
				})
				if iconButton(c.Key(key+"-close"), icons.Close, i18n.T("Close")).Clicked() {
					*open = false
				}
			})
			ui.Scroll(c.Key(key + "-body")).Grow(1).Shrink(1).MinHeight(0).Children(func() {
				ui.Column(c).Padding(px(t, sp2), px(t, sp6), px(t, sp4)).Gap(px(t, sp2)).Children(body)
			})
			ui.Box(c).Height(1).Background(k.Line).Shrink(0)
			ui.Row(c.Key(key+"-footer")).Shrink(0).AlignItems(ui.Center).Padding(px(t, sp3), px(t, sp6)).Gap(px(t, sp2)).Children(footer)
		})
	})
}

// modal is a confirmation (ui.Modal's look: fn's elements in a padded
// column over the dimmed window), rounded as the dialogs are
// (radiusDialog; ui.Modal's is MyGo's own). Escape or a click beside it
// close it (*open false).
func modal(c *ui.Context, open *bool, fn func()) ui.Element {
	t := c.Theme()
	w, h := c.Size()
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.4))
		panel.Padding(t.Space(5)).Gap(t.Space(3)).Radius(px(t, radiusDialog)).Background(t.Background).
			MaxWidth(w-t.Space(10)).MaxHeight(h-t.Space(10)).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3))
		fn()
	})
}
