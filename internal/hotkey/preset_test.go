package hotkey

import (
	"testing"

	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestPresets: the defaults are the family's preset; each preset applied
// is that preset, without problems; a changed shortcut is custom, and
// Paste the last result does not count.
func TestPresets(t *testing.T) {
	f := settings.Defaults().Features
	if got := PresetOf(f); got != PresetFamily {
		t.Fatalf("defaults: %s", got)
	}
	for _, p := range Presets {
		g := settings.Defaults().Features
		g.PasteLast = settings.Hotkey{}
		if !ApplyPreset(&g, p.ID) || PresetOf(g) != p.ID {
			t.Errorf("%s: applied, PresetOf %s", p.ID, PresetOf(g))
		}
		if ps := Check(g); len(ps) != 0 {
			t.Errorf("%s: problems %+v", p.ID, ps)
		}
		if p.ID != PresetMouse && len(Build(g, Features).Bindings) != 3 {
			t.Errorf("%s: bindings %+v", p.ID, Build(g, Features).Bindings)
		}
	}
	// Spelling and an empty trigger do not matter; a second shortcut does.
	f.Transcribe.Hotkeys = []settings.Hotkey{{Keys: "Right Option"}}
	if got := PresetOf(f); got != PresetFamily {
		t.Errorf("spelled: %s", got)
	}
	f.Transcribe.Hotkeys = append(f.Transcribe.Hotkeys, settings.Hotkey{Keys: "F5"})
	if got := PresetOf(f); got != PresetCustom {
		t.Errorf("two shortcuts: %s", got)
	}
	f.Transcribe.Hotkeys = []settings.Hotkey{{Keys: "RightOption", Trigger: settings.TriggerLong}}
	if got := PresetOf(f); got != PresetCustom {
		t.Errorf("long press: %s", got)
	}
	if ApplyPreset(&f, PresetCustom) {
		t.Error("custom applied")
	}
}
