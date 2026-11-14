package hotkey

import "strings"

// Mouse buttons as shortcuts (Voxt's Input.mouseButton): the middle
// button and the two side buttons, alone or with modifiers. The helper
// takes their presses from an active tap of their own (the left and right
// buttons are never taken), so a bound button's click does not reach the
// app under the pointer.

// The mouse buttons' names in the grammar.
const (
	MouseMiddle  = "MouseMiddle"
	MouseBack    = "MouseBack"
	MouseForward = "MouseForward"
)

// mouseButtons are CGMouseButton numbers by name: 0 and 1 (left, right)
// are no shortcuts.
var mouseButtons = map[string]int{MouseMiddle: 2, MouseBack: 3, MouseForward: 4}

var mouseAliases = map[string]string{
	"mousemiddle": MouseMiddle, "middlemouse": MouseMiddle, "middle": MouseMiddle, "mouse3": MouseMiddle,
	"mouseback": MouseBack, "mouse4": MouseBack,
	"mouseforward": MouseForward, "mouse5": MouseForward,
}

// mousePart reads a mouse button part: its name and number.
func mousePart(p string) (name string, button int, ok bool) {
	name, ok = mouseAliases[strings.ToLower(strings.ReplaceAll(p, " ", ""))]
	return name, mouseButtons[name], ok
}

// Mouse reports whether the chord is a mouse button's.
func (c Chord) Mouse() bool { return c.Button > 0 }
