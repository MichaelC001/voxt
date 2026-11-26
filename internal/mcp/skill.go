package mcp

// Embedded agent skill (skill/voxtmini/SKILL.md) installed into client
// skill directories by internal/agentsetup.

import (
	"embed"
	"io/fs"
	"strings"
	"testing/fstest"
)

//go:embed skill/voxtmini
var skillFiles embed.FS

// Skill returns the voxtmini skill filesystem with {{VERSION}} filled in.
func Skill(version string) fs.FS {
	out := fstest.MapFS{}
	_ = fs.WalkDir(skillFiles, "skill/voxtmini", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := skillFiles.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(p, "SKILL.md") {
			b = []byte(strings.ReplaceAll(string(b), "{{VERSION}}", version))
		}
		out[strings.TrimPrefix(p, "skill/voxtmini/")] = &fstest.MapFile{Data: b, Mode: 0o644}
		return nil
	})
	return out
}
