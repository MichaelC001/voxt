package i18n

import (
	"strings"
	"testing"
)

// statusEllipses are the messages that may end in "…": progress and
// state texts ("Downloading…"), never labels. A button, a menu item, a
// link, a select option or a settings row has no trailing ellipsis
// (user, 2026-11-14): add a status text here, not a label.
var statusEllipses = map[string]bool{
	"Checking…":                  true,
	"Cleaning up…":               true,
	"Downloading in %s…":         true,
	"Downloading…":               true,
	"Inserting…":                 true,
	"Listening…":                 true,
	"Loading the speech model…":  true,
	"Looking for apps…":          true,
	"Measuring…":                 true,
	"Reading the address…":       true,
	"Reading the selection…":     true,
	"Recognising…":               true,
	"Recognizing…":               true,
	"Rewriting the selection…":   true,
	"Starting the microphone…":   true,
	"Starting…":                  true,
	"Testing…":                   true,
	"Thinking…":                  true,
	"Translating the selection…": true,
	"Translating…":               true,
	"Writing…":                   true,
}

// TestNoTrailingEllipsis: no message ends in "…" or "..." in either
// language unless it is a status text.
func TestNoTrailingEllipsis(t *testing.T) {
	keys, _ := sourceKeys(t)
	zh := readLocale(t, "zh-Hans")
	for k := range keys {
		if _, ok := zh[k]; !ok {
			zh[k] = "" // the source's keys too, should the catalog lag
		}
	}
	for k, v := range zh {
		if statusEllipses[k] {
			continue
		}
		for _, s := range []string{k, v} {
			if strings.HasSuffix(s, "…") || strings.HasSuffix(s, "...") {
				t.Errorf("%q ends in an ellipsis: a label has none (a status text goes in statusEllipses)", s)
			}
		}
	}
}
