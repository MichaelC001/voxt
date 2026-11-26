package mcp

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestSkill(t *testing.T) {
	sk := Skill("0.2.0")
	b, err := fs.ReadFile(sk, "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	lines := strings.Split(text, "\n")
	if len(lines) > 150 {
		t.Errorf("SKILL.md has %d lines, want <= 150", len(lines))
	}
	if len(b) > 12000 {
		t.Errorf("SKILL.md is %d bytes, want <= 12000", len(b))
	}

	front, body, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	if !strings.HasPrefix(text, "---\n") || !ok {
		t.Fatal("missing YAML frontmatter")
	}
	allowed := []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}
	for _, line := range strings.Split(front, "\n") {
		if line == "" || strings.HasPrefix(line, " ") {
			continue
		}
		key, _, _ := strings.Cut(line, ":")
		if !slices.Contains(allowed, key) {
			t.Errorf("frontmatter field %q is not one of the 6 standard fields", key)
		}
	}
	if !strings.Contains(front, "\nname: voxtmini\n") && !strings.HasPrefix(front, "name: voxtmini\n") {
		t.Error("frontmatter name must be voxtmini")
	}
	if m := regexp.MustCompile(`(?m)^description: (.*)$`).FindStringSubmatch(front); m == nil || len(m[1]) > 1024 || len(m[1]) < 100 {
		t.Errorf("invalid description: %v", m)
	}
	if !strings.Contains(front, `version: "0.2.0"`) {
		t.Error("version placeholder was not replaced")
	}

	names := map[string]bool{}
	for _, tl := range toolset() {
		names[tl.name] = true
	}
	nonToolTokens := map[string]bool{
		"user_declined": true,
		"not_confirmed": true,
		"scope_off":     true,
		"remote_off":    true,
		"rate_limited":  true,
		"invalid_path":  true,
		"too_large":     true,
		"model_id":      true,
	}
	for _, m := range regexp.MustCompile("`([a-z]+_[a-z_]+)`").FindAllStringSubmatch(body, -1) {
		if nonToolTokens[m[1]] {
			continue
		}
		if !names[m[1]] {
			t.Errorf("SKILL.md references %q, which is not a registered tool", m[1])
		}
	}
}
