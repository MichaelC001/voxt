package main

// Settings → Overlay style (/settings/overlay): where the recording
// overlay shows (the bottom or the top of the display, how far from its
// edge), how opaque its face is, how round its corners are, whether it
// shows the recording's time and the live text (settings.Overlay), under
// a preview of the overlay in the state chosen (settings_overlay_preview.go).
// A change moves the overlay at once when it shows; it is saved once the
// sliders rest.

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// overlayForm is the section's controls.
type overlayForm struct {
	position                int // 0 bottom, 1 top
	opacity, radius, margin float64
	showTime, liveText      bool
	// saveAt is when the last change waits to be saved (zero: saved).
	saveAt time.Time
	// preview is the state the preview shows (ovPreviewRecording…);
	// previewW its width as last laid out (0: not yet).
	preview  int
	previewW float32
}

// Bounds of the sliders (settings.normalize keeps wider ones).
const (
	ovMaxRadius = 26 // the pill's half height: rounder changes nothing
	ovMaxMargin = 240
	ovSaveDelay = 400 * time.Millisecond
)

// overlayPositions are the values of Overlay.Position, by segment.
var overlayPositions = []string{"bottom", "top"}

// loadOverlayForm sets the controls from the settings, unless a change
// waits to be saved (the sliders move).
func (a *app) loadOverlayForm() {
	f := &a.settingsForm.ovStyle
	if !f.saveAt.IsZero() {
		return
	}
	o := a.prefs.Overlay
	f.position = 0
	if o.Position == "top" {
		f.position = 1
	}
	f.opacity, f.radius, f.margin, f.showTime = o.Opacity, o.Radius, o.Margin, o.ShowTime
	f.liveText = !o.HideLiveText
}

// overlayEdited applies the controls: at once to the overlay and the
// preview, to the settings after ovSaveDelay without another change.
func (a *app) overlayEdited() {
	f := &a.settingsForm.ovStyle
	o := settings.Overlay{
		Position: overlayPositions[min(max(f.position, 0), 1)],
		Opacity:  math.Round(f.opacity*100) / 100,
		Radius:   math.Round(f.radius),
		Margin:   math.Round(f.margin),
		ShowTime: f.showTime,
		// HideLiveText: the overlay's look aside, the section's too.
		HideLiveText: !f.liveText,
	}
	a.prefs.Overlay = o
	a.overlayRestyle() // overlay.go
	if a.win == nil {
		a.updateSettings(func(s *settings.Settings) { s.Overlay = o })
		return
	}
	at := time.Now()
	f.saveAt = at
	time.AfterFunc(ovSaveDelay, func() {
		a.post(func() {
			if f.saveAt != at {
				return // a later change saves
			}
			f.saveAt = time.Time{}
			o := a.prefs.Overlay
			a.updateSettings(func(s *settings.Settings) { s.Overlay = o })
		})
	})
}

// overlaySettings is the Overlay style section of the settings dialog.
func (a *app) overlaySettings(c *ui.Context) {
	t := c.Theme()
	f := &a.settingsForm.ovStyle
	a.overlayPreview(c) // settings_overlay_preview.go
	slider := func(key, label string, value *float64, lo, hi, step float64, text string) {
		ui.Row(c).Gap(px(t, sp3)).AlignItems(ui.Center).Children(func() {
			if ui.Slider(c.Key(key), value, lo, hi).Step(step).Label(label).Width(px(t, 160)).Shrink(1).MinWidth(px(t, 100)).Changed() {
				a.overlayEdited()
			}
			ui.Text(c, text).FontSize(px(t, fsCaption)).TextColor(tok(t).Text2).Font("monospace").Width(px(t, 40)).Shrink(0).SingleLine()
		})
	}
	settingRow(c, "ov-position", i18n.T("Position"), i18n.T("On the display under the pointer, centred."), func() {
		if ui.Segmented(c.Key("ov-position-seg"), &f.position, i18n.T("Bottom centre"), i18n.T("Top centre")).Label(i18n.T("Position")).Changed() {
			a.overlayEdited()
		}
	})
	settingRow(c, "ov-margin", i18n.T("Distance from the edge"), i18n.T("From the bottom or the top of the screen, above the Dock and below the menu bar."), func() {
		slider("ov-margin-slider", i18n.T("Distance from the edge"), &f.margin, 0, ovMaxMargin, 4, fmt.Sprintf("%.0f", f.margin))
	})
	settingRow(c, "ov-opacity", i18n.T("Opacity"), i18n.T("Of the dark face over the blurred desktop: lower lets more of what is behind show through."), func() {
		slider("ov-opacity-slider", i18n.T("Opacity"), &f.opacity, 0.5, 1, 0.05, fmt.Sprintf("%.0f%%", f.opacity*100))
	})
	settingRow(c, "ov-radius", i18n.T("Corner radius"), i18n.T("The answer card takes the same corners."), func() {
		slider("ov-radius-slider", i18n.T("Corner radius"), &f.radius, 0, ovMaxRadius, 1, fmt.Sprintf("%.0f", f.radius))
	})
	settingRow(c, "ov-time", i18n.T("Show the recording time"), i18n.T("Off, the overlay shows only the feature's icon and the waveform."), func() {
		if ui.Switch(c.Key("ov-time-switch"), &f.showTime).Label(i18n.T("Show the recording time")).Changed() {
			a.overlayEdited()
		}
	})
	settingRow(c, "ov-live", i18n.T("Show live text"), i18n.T("What you say, as it is recognised, and the answer as it is written. The answer card is not affected."), func() {
		if ui.Switch(c.Key("ov-live-switch"), &f.liveText).Label(i18n.T("Show live text")).Changed() {
			a.overlayEdited()
		}
	})
}
