package uitext

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/asr"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/privnet"
)

// TestPlainNetErrors: a refused plain connection and a private address
// out of reach (macOS's Local Network permission) are explained, through
// the services' errors, in the test's line, its ⓘ and a session's error.
func TestPlainNetErrors(t *testing.T) {
	denied := fmt.Errorf("dial tcp 100.88.1.2:3425: %w", privnet.ErrLocalNetwork)
	public := fmt.Errorf("%w: evil.example is 203.0.113.9", privnet.ErrPublic)
	for _, err := range []error{
		&asr.Error{Kind: asr.KindNetwork, Provider: "openai", Err: denied},
		asr.NetError(nil, "doubao", denied),
		&llm.Error{Code: llm.CodeNetwork, Err: denied},
	} {
		if msg, _ := ProviderErrorText(err); !strings.Contains(msg, "Local Network") {
			t.Errorf("%v: %q", err, msg)
		}
		if s := ProviderErrorShort(err); !strings.Contains(s, "Local Network") {
			t.Errorf("%v: short %q", err, s)
		}
		if msg, _ := ModelErrorText(err); !strings.Contains(msg, "Local Network") {
			t.Errorf("%v: model %q", err, msg)
		}
	}
	err := &asr.Error{Kind: asr.KindNetwork, Provider: "openai", Err: public}
	if msg, _ := ProviderErrorText(err); !strings.Contains(msg, "public") {
		t.Errorf("public: %q", msg)
	}
	if s := ProviderErrorShort(err); !strings.Contains(s, "public") {
		t.Errorf("public: short %q", s)
	}
	if msg, _ := ProviderErrorText(&asr.Error{Kind: asr.KindNetwork, Err: errors.New("connection refused")}); strings.Contains(msg, "Local Network") {
		t.Error(msg)
	}
}
