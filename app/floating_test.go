package main

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo"
)

// TestFloatingWindowOptions: floating windows never activate VoxtMini
// (a panel that never takes the keyboard), and start hidden above other
// windows.
func TestFloatingWindowOptions(t *testing.T) {
	o := floatingWindowOptions("pill", 220, 48, nil)
	if !o.Panel || o.PanelKeyboard != mygo.PanelKeyboardNever || !o.AlwaysOnTop || !o.Hidden || !o.SkipTaskbar || !o.Frameless || o.Width != 220 || o.Height != 48 {
		t.Errorf("options %+v", o)
	}
	// Off macOS nothing can be activated.
	if runtime.GOOS != "darwin" {
		if ok, _ := activateTarget(1); ok {
			t.Error("activated off macOS")
		}
	}
}
