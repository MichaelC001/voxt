package agentsetup

// Skill installation into ~/.agents/skills/voxtmini/,
// ~/.claude/skills/voxtmini/, and ~/.workbuddy/skills/voxtmini/.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
)

// AppsDir is where macOS .app bundles are checked (overridden in tests).
var AppsDir = "/Applications"

// SkillTarget describes one client's skills folder.
type SkillTarget struct {
	ID        string
	Name      string
	Dir       string // .../skills/voxtmini
	Found     bool
	Installed bool
	Current   bool
}

var skillClients = []struct {
	id, name, dot string
	found, apps   []string
}{
	{
		"agents",
		"ChatGPT / Codex, Cursor, VS Code, Gemini",
		".agents",
		[]string{".agents", ".codex", ".cursor", ".gemini", ".copilot"},
		[]string{"ChatGPT.app", "Codex.app", "Cursor.app", "Visual Studio Code.app"},
	},
	{
		"claude",
		"Claude Code",
		".claude",
		[]string{".claude"},
		[]string{"Claude.app"},
	},
	{
		WorkBuddy,
		"WorkBuddy",
		".workbuddy",
		[]string{".workbuddy"},
		[]string{"WorkBuddy.app", "WorkBuddy AI.app"},
	},
}

// SkillTargets inspects home and AppsDir for skill directories and compares
// installed files against skill (when non-nil).
func SkillTargets(home string, skill fs.FS) []SkillTarget {
	out := make([]SkillTarget, 0, len(skillClients))
	for _, c := range skillClients {
		t := SkillTarget{
			ID:   c.id,
			Name: c.name,
			Dir:  filepath.Join(home, c.dot, "skills", ServerName),
		}
		for _, d := range c.found {
			if fi, err := os.Stat(filepath.Join(home, d)); err == nil && fi.IsDir() {
				t.Found = true
			}
		}
		for _, a := range c.apps {
			if _, err := os.Stat(filepath.Join(AppsDir, a)); err == nil {
				t.Found = true
			}
		}
		if _, err := os.Stat(filepath.Join(t.Dir, "SKILL.md")); err == nil {
			t.Installed = true
			t.Current = skill != nil && sameFiles(skill, t.Dir)
		}
		out = append(out, t)
	}
	return out
}

func sameFiles(skill fs.FS, dir string) bool {
	same := true
	_ = fs.WalkDir(skill, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || !same {
			return fs.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		want, err1 := fs.ReadFile(skill, p)
		have, err2 := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err1 != nil || err2 != nil || !bytes.Equal(want, have) {
			same = false
		}
		return nil
	})
	return same
}

// InstallSkill atomically replaces dir with the contents of skill.
func InstallSkill(skill fs.FS, dir string) error {
	tmp := dir + ".new"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	if err := os.CopyFS(tmp, skill); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	_ = os.RemoveAll(dir)
	return os.Rename(tmp, dir)
}
