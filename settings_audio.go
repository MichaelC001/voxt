package main

// Settings → Audio (/settings/audio): the microphones in the user's
// order of preference (settings.Audio.Devices: the first one connected
// records, else the system's default input), whether the silence of a
// recording is left out before the speech model gets it (Audio.VAD.Off,
// internal/vad) and whether its sound is cleaned up (Audio.Clean.Off,
// internal/dsp), and the sounds a recording makes when it starts and
// stops (their presets and previews: settings_sounds.go); the silence,
// the cleaning and the mute below wait in the Advanced fold. The helper
// lists the inputs
// (capture.devices) when the section opens, when the helper becomes
// ready while it shows, and with Refresh. The system's output is muted
// while recording (Audio.MuteOutput, mute_link.go), unless turned off.

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/actnowkit/sounds"
)

// audioState is the audio section's view state, and the player of the
// recording's cues.
type audioState struct {
	audio struct {
		devices []proto.CaptureDevice // as capture.devices (or capture.devices.changed) last said
		read    bool                  // devices were read at least once
		loading bool
		err     string       // why the last read failed ("" for none)
		list    ui.ListState // the list's place, its rows dragged (Reorder)
	}
	// playSound plays a cue and returns when it ended (playCueWait, on a
	// goroutine); tests record the cues instead.
	playSound func(sounds.Target)
}

func (a *app) openAudio() {
	a.openSounds() // settings_sounds.go: the family's sounds, the player
	a.playSound = a.playCueWait
}

// loadDevices asks the helper for the audio inputs.
func (a *app) loadDevices() {
	if !a.helperReady() || a.audio.loading {
		return
	}
	a.audio.loading = true
	helperCall(a, proto.MethodCaptureDevices, nil, 5*time.Second, func(r proto.DevicesResult, err error) {
		a.audio.loading = false
		if err != nil {
			a.audio.err = errText(err)
			return
		}
		a.devicesListed(r.Devices) // tray_mic.go
	})
}

// rememberDeviceNames keeps the names of the listed devices that are
// connected, to show them while they are not.
func (a *app) rememberDeviceNames(devs []proto.CaptureDevice) {
	names := map[string]string{}
	for _, d := range devs {
		if slices.Contains(a.prefs.Audio.Devices, d.ID) && d.Name != "" && a.prefs.Audio.DeviceNames[d.ID] != d.Name {
			names[d.ID] = d.Name
		}
	}
	if len(names) == 0 {
		return
	}
	a.updateSettings(func(s *settings.Settings) {
		m := maps.Clone(s.Audio.DeviceNames)
		if m == nil {
			m = map[string]string{}
		}
		maps.Copy(m, names)
		s.Audio.DeviceNames = m
	})
}

// deviceInUse is the input a recording would use now: the first of list
// that is connected, else the system's default input ("" when unknown).
func deviceInUse(list []string, devs []proto.CaptureDevice) string {
	for _, id := range list {
		if slices.ContainsFunc(devs, func(d proto.CaptureDevice) bool { return d.ID == id }) {
			return id
		}
	}
	for _, d := range devs {
		if d.Default {
			return d.ID
		}
	}
	return ""
}

// addDevice puts a connected input at the end of the list.
func (a *app) addDevice(d proto.CaptureDevice) {
	a.updateSettings(func(s *settings.Settings) {
		if slices.Contains(s.Audio.Devices, d.ID) {
			return
		}
		s.Audio.Devices = append(slices.Clone(s.Audio.Devices), d.ID)
		m := maps.Clone(s.Audio.DeviceNames)
		if m == nil {
			m = map[string]string{}
		}
		m[d.ID] = d.Name
		s.Audio.DeviceNames = m
	})
}

// moveDevices moves the list's rows (in order) before row to (the
// list's length for its end): a drag (ListState.Reorder).
func (a *app) moveDevices(rows []int, to int) {
	a.updateSettings(func(s *settings.Settings) { s.Audio.Devices = moveRows(s.Audio.Devices, rows, to) })
}

// moveRows returns list with its rows (indices, in order) moved before
// row to; indices out of range are left out of the move.
func moveRows[T any](list []T, rows []int, to int) []T {
	moving := make([]bool, len(list))
	var moved []T
	for _, r := range rows {
		if r >= 0 && r < len(list) && !moving[r] {
			moving[r] = true
			moved = append(moved, list[r])
		}
	}
	to = max(to, 0)
	out := make([]T, 0, len(list))
	for i, v := range list {
		if i == to {
			out = append(out, moved...)
		}
		if !moving[i] {
			out = append(out, v)
		}
	}
	if to >= len(list) {
		out = append(out, moved...)
	}
	return out
}

// removeDevice takes the list's input id off it.
func (a *app) removeDevice(id string) {
	a.updateSettings(func(s *settings.Settings) {
		s.Audio.Devices = slices.DeleteFunc(slices.Clone(s.Audio.Devices), func(x string) bool { return x == id })
	})
}

// devRowH is the height of a row of the microphones' list (a name and
// its notes): the list is as tall as its rows, inside the scrolling page.
const devRowH = 44

// audioSettings is the Audio section of the settings dialog.
func (a *app) audioSettings(c *ui.Context) {
	t := c.Theme()
	k := tok(t)
	devs := a.audio.devices
	list := a.prefs.Audio.Devices
	inUse := deviceInUse(list, devs)
	byID := func(id string) (proto.CaptureDevice, bool) {
		i := slices.IndexFunc(devs, func(d proto.CaptureDevice) bool { return d.ID == id })
		if i < 0 {
			return proto.CaptureDevice{}, false
		}
		return devs[i], true
	}

	ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Margin(px(t, sp2), 0, 0, 0).Children(func() {
		sectionTitleText(c, i18n.T("Microphones"))
		infoTip(c.Key("audio-mics-info"), i18n.T("VoxtMini records from the first microphone of the list that is connected. When none of them is, or the list is empty, it records from the system's default input."))
		ui.Spacer(c)
		if iconButton(c.Key("audio-refresh"), icons.Refresh, i18n.T("Refresh the microphones")).Disabled(!a.helperReady() || a.audio.loading).Clicked() {
			a.loadDevices()
		}
	})
	switch {
	case !a.helperReady():
		notice(c, "audio-no-helper", noticeWarn, i18n.T("The helper is not running: the microphones cannot be listed."), "", func() {
			if ui.Button(c.Key("audio-restart"), i18n.T("Restart the helper")).Clicked() {
				a.restartHelper()
			}
		}, nil)
	case a.audio.err != "":
		notice(c, "audio-err", noticeError, i18n.T("Could not list the microphones: %s", a.audio.err), "", nil, nil)
	}

	// The list, in order; a row is dragged to another place.
	card(c.Key("audio-list")).Gap(px(t, sp1)).Padding(px(t, sp2)).Children(func() {
		if len(list) == 0 {
			ui.Row(c).Padding(px(t, sp1), px(t, sp2)).Children(func() {
				caption(c, i18n.T("None chosen: the system's default input is used."))
			})
			return
		}
		ls := &a.audio.list
		ls.Key = func(i int) any { return list[i] }
		ls.Reorder = func(rows []int, to int) { a.moveDevices(rows, to) }
		n := len(list)
		h := float32(n)*px(t, devRowH) + float32(n-1)*px(t, sp1)
		ui.List(c.Key("audio-rows"), ls, n, func(i int) {
			id := list[i]
			d, present := byID(id)
			name := d.Name
			if !present {
				name = a.prefs.Audio.DeviceNames[id]
			}
			if name == "" {
				name = id
			}
			var notes []string
			switch {
			case !present && a.audio.read:
				notes = append(notes, i18n.T("Not connected"))
			case id == inUse:
				notes = append(notes, i18n.T("Records now"))
			}
			if d.Default {
				notes = append(notes, i18n.T("System default"))
			}
			ui.Row(c).Height(px(t, devRowH)).Gap(px(t, sp2)).AlignItems(ui.Center).Padding(0, px(t, sp1), 0, px(t, sp1)).
				Cursor(ui.CursorGrab).Children(func() {
				ui.Icon(c, icons.HamburgerMenu.Linear).Size(px(t, 14), px(t, 14)).TextColor(k.Text3).Shrink(0).Tooltip(i18n.T("Drag to change the order"))
				ui.Text(c, itoa(i+1)).FontSize(px(t, fsCaption)).TextColor(k.Text3).Width(px(t, 14)).Shrink(0)
				deviceName(c, name, strings.Join(notes, " · "), id == inUse, present || !a.audio.read)
				if iconButton(c.Key("remove"), icons.Close, i18n.T("Remove %s", name)).Clicked() {
					a.removeDevice(id)
				}
			})
		}).Height(h).Gap(px(t, sp1))
	})

	// The other inputs connected now.
	var others []proto.CaptureDevice
	for _, d := range devs {
		if !slices.Contains(list, d.ID) {
			others = append(others, d)
		}
	}
	if a.audio.read {
		sectionTitle(c, i18n.T("Other microphones"))
		card(c.Key("audio-others")).Gap(px(t, sp1)).Padding(px(t, sp2)).Children(func() {
			if len(others) == 0 {
				ui.Row(c).Padding(px(t, sp1), px(t, sp2)).Children(func() {
					caption(c, i18n.T("No other microphone is connected."))
				})
			}
			for _, d := range others {
				var notes []string
				if d.ID == inUse {
					notes = append(notes, i18n.T("Records now"))
				}
				if d.Default {
					notes = append(notes, i18n.T("System default"))
				}
				ui.Row(c.Key("audio-other-"+d.ID)).Gap(px(t, sp2)).AlignItems(ui.Center).Padding(px(t, sp1), px(t, sp1), px(t, sp1), px(t, sp2)).Children(func() {
					deviceName(c, d.Name, strings.Join(notes, " · "), d.ID == inUse, true)
					if ghostIconButton(c.Key("add"), icons.Add, i18n.T("Add")).Label(i18n.T("Add %s", d.Name)).Shrink(0).Clicked() {
						a.addDevice(d)
					}
				})
			}
		})
	}

	sectionTitle(c, i18n.T("Sounds"))
	a.soundsRow(c) // settings_sounds.go

	// Less used: what happens to the sound before recognition, and the
	// system's output while recording.
	f := &a.settingsForm
	sum := summary(onOff(i18n.T("Skip silence"), f.skipSilence), onOff(i18n.T("Optimize sound"), f.cleanSound), onOff(i18n.T("Mute while recording"), f.muteOutput))
	advanced(c.Key("audio-advanced"), &f.audioAdvanced, sum, func() {
		settingRow(c, "audio-silence", i18n.T("Skip silence"), i18n.T("Trims silence and long pauses before recognition; sends nothing when no one spoke. The history keeps the whole recording."), func() {
			if ui.Switch(c.Key("audio-silence-switch"), &f.skipSilence).Label(i18n.T("Skip silence")).Changed() {
				off := !f.skipSilence
				a.updateSettings(func(s *settings.Settings) { s.Audio.VAD.Off = off })
			}
		})
		settingRow(c, "audio-clean", i18n.T("Optimize sound"), i18n.T("Filters out low hum (air conditioning, fans) and evens out the volume before recognition. The history keeps the original recording."), func() {
			if ui.Switch(c.Key("audio-clean-switch"), &f.cleanSound).Label(i18n.T("Optimize sound")).Changed() {
				off := !f.cleanSound
				a.updateSettings(func(s *settings.Settings) { s.Audio.Clean.Off = off })
			}
		})
		settingRow(c, "audio-mute", i18n.T("Mute system audio while recording"), i18n.T("Mutes the output device while you speak, so videos, music or calls are not recorded; the sound comes back when recording stops. Some devices cannot be muted by software."), func() {
			if ui.Switch(c.Key("audio-mute-switch"), &f.muteOutput).Label(i18n.T("Mute system audio while recording")).Changed() {
				on := f.muteOutput
				a.updateSettings(func(s *settings.Settings) { s.Audio.MuteOutput = &on })
				a.muteSettingChanged(on) // mute_link.go: off during a recording gives the output back now
			}
		})
	})
}

// deviceName is an input's name and, under it, notes on its state; a
// check marks the one that records now, and a grey name one that is not
// connected.
func deviceName(c *ui.Context, name, notes string, inUse, present bool) {
	t := c.Theme()
	k := tok(t)
	ic, col := icons.MicrophoneLarge, k.Text3
	if inUse {
		ic, col = icons.CheckCircle, k.Accent
	}
	ui.Icon(c, ic.Linear).Size(px(t, 16), px(t, 16)).TextColor(col).Shrink(0)
	ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
		tx := truncText(c, name)
		if !present {
			tx.TextColor(k.Text2)
		}
		if notes != "" {
			ui.Text(c, notes).FontSize(px(t, fsCaption)).TextColor(k.Text2).SingleLine().Ellipsis("…").MinWidth(0)
		}
	})
}

// cue is a sound of a session: its recording starts, it stops, the
// session failed.
type cue int

const (
	noCue cue = iota
	startCue
	stopCue
	failCue
)

// recordingCue is the cue a change of the session makes: one when it
// starts recording, another when the recording stops and the text is on
// its way, a third when the session ends without a result (failed, or
// nothing was recognised). A cancelled recording (Esc, too short) stops
// silently, and so does a failed one until it ends: its stop cue is the
// failure's.
func recordingCue(was, s session.Snapshot) cue {
	switch {
	case s.State == session.Recording && (was.State != session.Recording || was.ID != s.ID):
		return startCue
	case sessionFailed(s) && (was.ID != s.ID || !was.State.Ended()):
		return failCue
	case was.State == session.Recording && was.ID == s.ID && s.State != session.Recording &&
		s.State != session.Cancelled && s.State != session.Failed:
		return stopCue
	}
	return noCue
}

// sessionFailed: s ended without a result — failed, or done with nothing
// recognised (a degraded result is a result).
func sessionFailed(s session.Snapshot) bool {
	return s.State == session.Failed || (s.State == session.Done && s.Problem != nil && s.Problem.Code == session.ProblemNoSpeech)
}
