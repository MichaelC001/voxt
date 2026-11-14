package session

import (
	"strings"
	"testing"

	"github.com/hehehai/VoxtMini/internal/compose"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TestMergeRules: version 4's rules become the feature's task prompt,
// after the built-in one in the main language's set (the rules' language
// without one); a feature without rules keeps the built-in prompt ("").
func TestMergeRules(t *testing.T) {
	s, err := settings.Migrate([]byte(`{"version":4,"features":{"transcribe":{"prompt":"Write numbers as digits."},"translate":{"prompt":"术语保留英文。"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Features.Transcribe.Prompt != "" || s.Features.Transcribe.Rules != "Write numbers as digits." {
		t.Fatalf("migrated %+v", s.Features.Transcribe.Feature)
	}
	MergeRules(&s)
	f := s.Features
	if f.Transcribe.Prompt != compose.DefaultPrompt(compose.Transcribe, compose.English)+"\n\nWrite numbers as digits." || f.Transcribe.Rules != "" {
		t.Errorf("transcribe %q", f.Transcribe.Prompt)
	}
	if f.Translate.Prompt != compose.DefaultPrompt(compose.Translate, compose.Chinese)+"\n\n术语保留英文。" {
		t.Errorf("translate %q", f.Translate.Prompt)
	}
	if f.Rewrite.Prompt != "" || f.Rewrite.Rules != "" {
		t.Errorf("rewrite %+v", f.Rewrite.Feature)
	}
	// A version 5 prompt is the prompt, and a second run changes nothing.
	again, _ := settings.Migrate([]byte(`{"version":5,"general":{"mainLanguage":"zh"},"features":{"rewrite":{"prompt":"Be brief."}}}`))
	MergeRules(&again)
	if again.Features.Rewrite.Prompt != "Be brief." {
		t.Errorf("v5 %q", again.Features.Rewrite.Prompt)
	}
	// The main language picks the set, whatever the rules' language.
	zh, _ := settings.Migrate([]byte(`{"version":4,"general":{"mainLanguage":"zh"},"features":{"rewrite":{"prompt":"no emoji"}}}`))
	MergeRules(&zh)
	if !strings.HasPrefix(zh.Features.Rewrite.Prompt, compose.DefaultPrompt(compose.Rewrite, compose.Chinese)) {
		t.Errorf("zh %q", zh.Features.Rewrite.Prompt)
	}
}
