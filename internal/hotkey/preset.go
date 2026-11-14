package hotkey

import "github.com/hehehai/VoxtMini/internal/settings"

// Shortcut presets (Voxt's HotkeyPreference.Preset): one choice sets the
// three features' shortcuts at once. Which preset the settings are is
// worked out from the shortcuts themselves, never stored: changing a
// shortcut by hand makes them "custom". Paste the last result is no part
// of a preset.

// Preset ids.
const (
	// PresetFamily is the ActNow family's assignment, the defaults:
	// right ⌥, right ⌥ ⇧, ⌃ right ⌥.
	PresetFamily = "family"
	// PresetFn is Voxt's (and Auralink's) fn combos: fn, fn ⇧, fn ⌃.
	PresetFn = "fn"
	// PresetCommand is Voxt's ⌘ combos: right ⌘, right ⌘ right ⇧, right ⌘
	// right ⌥.
	PresetCommand = "command"
	// PresetMouse is Voxt's middle button preset: a click transcribes, a
	// double click rewrites, fn ⇧ translates (as Voxt's, though Auralink
	// uses fn too: users rarely run both).
	PresetMouse = "mouse"
	// PresetCustom: the shortcuts are none of the presets.
	PresetCustom = "custom"
)

// Preset is a set of shortcuts for Transcribe, Translate and Rewrite.
type Preset struct {
	ID                             string
	Transcribe, Translate, Rewrite settings.Hotkey
}

func tap(keys string) settings.Hotkey {
	return settings.Hotkey{Keys: keys, Trigger: settings.TriggerTap}
}

// Presets are the presets in the order the select offers them.
var Presets = []Preset{
	{PresetFamily, tap(settings.DefaultTranscribe), tap(settings.DefaultTranslate), tap(settings.DefaultRewrite)},
	{PresetFn, tap("Fn"), tap("Fn+Shift"), tap("Fn+Control")},
	{PresetCommand, tap("RightCommand"), tap("RightShift+RightCommand"), tap("RightOption+RightCommand")},
	{PresetMouse, tap(MouseMiddle), tap("Fn+Shift"), settings.Hotkey{Keys: MouseMiddle, Trigger: settings.TriggerDouble}},
}

// PresetOf is the preset f's shortcuts are (each feature exactly the
// preset's one shortcut), PresetCustom for none.
func PresetOf(f settings.Features) string {
	for _, p := range Presets {
		if sameHotkeys(f.Transcribe.Hotkeys, p.Transcribe) && sameHotkeys(f.Translate.Hotkeys, p.Translate) &&
			sameHotkeys(f.Rewrite.Hotkeys, p.Rewrite) {
			return p.ID
		}
	}
	return PresetCustom
}

// sameHotkeys reports whether hs is exactly h: the same keys (in any
// spelling) and trigger.
func sameHotkeys(hs []settings.Hotkey, h settings.Hotkey) bool {
	if len(hs) != 1 || trigger(hs[0].Trigger) != trigger(h.Trigger) {
		return false
	}
	a, err1 := Parse(hs[0].Keys)
	b, err2 := Parse(h.Keys)
	return err1 == nil && err2 == nil && a.Equal(b)
}

func trigger(t string) string {
	if t == "" {
		return settings.TriggerTap
	}
	return t
}

// ApplyPreset sets f's three features' shortcuts to the preset id's
// (replacing every shortcut they had); false for an unknown id.
func ApplyPreset(f *settings.Features, id string) bool {
	for _, p := range Presets {
		if p.ID == id {
			f.Transcribe.Hotkeys = []settings.Hotkey{p.Transcribe}
			f.Translate.Hotkeys = []settings.Hotkey{p.Translate}
			f.Rewrite.Hotkeys = []settings.Hotkey{p.Rewrite}
			return true
		}
	}
	return false
}
