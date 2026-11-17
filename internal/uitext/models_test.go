package uitext

import (
	"errors"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/llm"
)

// TestModelErrorOSTooOld: a runtime that this macOS cannot load (issue
// #162: MLX's Metal 4 library on macOS 15) says so instead of "stopped or
// did not answer", and keeps the runtime's words for the ⓘ.
func TestModelErrorOSTooOld(t *testing.T) {
	metal := "omni: asr.accurate did not start: model load failed: Failed to load the default metallib. This library is using language version 4.0 which is not supported on this OS. library not found"
	for _, err := range []error{
		&asr.Error{Kind: asr.KindServer, Provider: "omni", Detail: "the local speech model did not start: " + metal, Err: errors.New(metal)},
		&llm.Error{Code: llm.CodeServer, Err: errors.New("dyld: Library not loaded: libmlx.dylib (built for macOS 26.2 which is newer than running OS)")},
	} {
		msg, more := ModelErrorText(err)
		if !strings.Contains(msg, "macOS version") || more != err.Error() {
			t.Errorf("%v: %q (more %q)", err, msg, more)
		}
	}
	if msg, _ := ModelErrorText(&asr.Error{Kind: asr.KindServer, Provider: "omni", Detail: "exited"}); strings.Contains(msg, "macOS") {
		t.Errorf("another failure: %q", msg)
	}
}
