package agentsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const ctlPath = "/Applications/VoxtMini.app/Contents/Helpers/voxtmini-ctl"

var testCmd = DefaultCommand(ctlPath)

const wantTOMLBody = "command = \"" + ctlPath + "\"\nargs = [\"mcp\"]\nstartup_timeout_sec = 60\ntool_timeout_sec = 180\n"

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteCodex(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".codex", "config.toml")

	// Preview on missing file.
	prev, err := Preview(home, Codex, testCmd)
	if err != nil || !prev.Changes || prev.Path != p || !strings.Contains(prev.Diff, "+ [mcp_servers.voxtmini]") {
		t.Fatalf("Preview missing: %+v %v", prev, err)
	}

	// Write new file.
	r, err := Write(home, Codex, testCmd)
	if err != nil || r.Path != p || r.Backup != "" || r.Unchanged {
		t.Fatalf("Write new: %+v %v", r, err)
	}
	if got := mustRead(t, p); got != "[mcp_servers.voxtmini]\n"+wantTOMLBody {
		t.Errorf("new file:\n%s", got)
	}

	// Write again -> unchanged.
	if r, err := Write(home, Codex, testCmd); err != nil || !r.Unchanged {
		t.Errorf("Write again: %+v %v", r, err)
	}

	// Replace existing entry, preserving sub-tables, other tables, and mode 0600.
	user := "model = \"gpt-6\"\n\n[mcp_servers.other]\ncommand = \"other\"\n\n[mcp_servers.voxtmini]\ncommand = \"/old\"\nstartup_timeout_sec = 10\n\n[mcp_servers.voxtmini.env]\nFOO = \"1\"\n\n[profiles.fast]\nmodel = \"o\"\n"
	_ = os.WriteFile(p, []byte(user), 0o600)
	_ = os.Chmod(p, 0o600)
	r, err = Write(home, Codex, testCmd)
	if err != nil || r.Unchanged || r.Backup == "" {
		t.Fatalf("replace: %+v %v", r, err)
	}
	if mustRead(t, r.Backup) != user {
		t.Error("backup does not match original")
	}
	want := strings.Replace(user, "command = \"/old\"\nstartup_timeout_sec = 10\n", wantTOMLBody, 1)
	if got := mustRead(t, p); got != want {
		t.Errorf("replaced:\n%s\nwant:\n%s", got, want)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}

	// Refuses unsafe TOML forms without modifying the file.
	for _, odd := range []string{
		"mcp_servers.voxtmini.command = \"x\"\n",
		"[mcp_servers]\nvoxtmini = { command = \"x\" }\n",
		"mcp_servers = { voxtmini = { command = \"x\" } }\n",
		"[mcp_servers.\"voxtmini\"]\ncommand = \"x\"\n",
		"[mcp_servers.voxtmini]\nnote = \"\"\"\n[x]\n\"\"\"\ncommand = \"x\"\n",
		"prompt = '''\n[mcp_servers.voxtmini]\n'''\n",
	} {
		_ = os.WriteFile(p, []byte(odd), 0o644)
		if _, err := Write(home, Codex, testCmd); err != ErrEditByHand {
			t.Errorf("%q: got err=%v, want ErrEditByHand", odd, err)
		}
		if mustRead(t, p) != odd {
			t.Errorf("%q was modified", odd)
		}
	}
}

func TestWriteJSONClients(t *testing.T) {
	home := t.TempDir()
	for _, client := range []string{ClaudeCode, ClaudeDesktop, Cursor, WorkBuddy} {
		t.Run(client, func(t *testing.T) {
			r, err := Write(home, client, testCmd)
			if err != nil || r.Unchanged || r.Backup != "" {
				t.Fatalf("Write new: %+v %v", r, err)
			}
			var m struct {
				MCPServers map[string]struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcpServers"`
			}
			raw := mustRead(t, r.Path)
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				t.Fatalf("invalid json: %v", err)
			}
			e := m.MCPServers[ServerName]
			if e.Command != ctlPath || len(e.Args) != 1 || e.Args[0] != "mcp" {
				t.Errorf("unexpected entry: %+v", e)
			}
			if r2, err := Write(home, client, testCmd); err != nil || !r2.Unchanged {
				t.Errorf("Write again: %+v %v", r2, err)
			}
		})
	}

	// Preserves order of other keys and servers; backs up; rejects duplicate keys.
	p := ConfigFile(home, Cursor)
	user := `{"zeta": 1, "mcpServers": {"b": {"url": "https://x"}, "voxtmini": {"command": "/old"}, "a": {"command": "a"}}, "alpha": [1,2]}`
	_ = os.WriteFile(p, []byte(user), 0o644)
	r, err := Write(home, Cursor, testCmd)
	if err != nil || r.Backup == "" || mustRead(t, r.Backup) != user {
		t.Fatalf("replace: %+v %v", r, err)
	}
	got := mustRead(t, p)
	if strings.Index(got, `"zeta"`) > strings.Index(got, `"alpha"`) || strings.Index(got, `"b"`) > strings.Index(got, `"a"`) {
		t.Errorf("key order changed:\n%s", got)
	}
	if strings.Contains(got, "/old") {
		t.Errorf("old command remained:\n%s", got)
	}

	for _, dup := range []string{
		`{"mcpServers": {"voxtmini": {"command": "/a"}, "voxtmini": {"command": "/b"}}}`,
		`{"mcpServers": {}, "mcpServers": {"voxtmini": {"command": "/b"}}}`,
	} {
		_ = os.WriteFile(p, []byte(dup), 0o644)
		if _, err := Write(home, Cursor, testCmd); err != ErrEditByHand {
			t.Errorf("%s: got %v, want ErrEditByHand", dup, err)
		}
	}
}

func TestDetectClientsAndSkills(t *testing.T) {
	home := t.TempDir()
	apps := t.TempDir()
	oldApps := AppsDir
	AppsDir = apps
	t.Cleanup(func() { AppsDir = oldApps })

	// Create .codex dir and Claude.app.
	_ = os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	_ = os.MkdirAll(filepath.Join(apps, "Claude.app"), 0o755)

	detected := DetectClients(home)
	byID := map[string]ClientStatus{}
	for _, d := range detected {
		byID[d.ID] = d
	}
	if !byID[Codex].Installed || byID[Codex].Configured {
		t.Errorf("codex detection: %+v", byID[Codex])
	}
	if !byID[ClaudeDesktop].Installed {
		t.Errorf("claude-desktop should be detected via Claude.app: %+v", byID[ClaudeDesktop])
	}
	if byID[WorkBuddy].Installed {
		t.Errorf("workbuddy should not be detected: %+v", byID[WorkBuddy])
	}

	if _, err := Write(home, Codex, testCmd); err != nil {
		t.Fatal(err)
	}
	for _, d := range DetectClients(home) {
		if d.ID == Codex && !d.Configured {
			t.Errorf("codex should be marked Configured after Write")
		}
	}

	// Skills: install, verify current, update, verify updated.
	skV1 := fstest.MapFS{"SKILL.md": &fstest.MapFile{Data: []byte("v1"), Mode: 0o644}}
	skV2 := fstest.MapFS{"SKILL.md": &fstest.MapFile{Data: []byte("v2"), Mode: 0o644}}
	targets := SkillTargets(home, skV1)
	if len(targets) != 3 {
		t.Fatalf("expected 3 skill targets, got %d", len(targets))
	}
	for _, tg := range targets {
		if !strings.HasSuffix(tg.Dir, filepath.Join("skills", "voxtmini")) {
			t.Errorf("unexpected skill dir %q", tg.Dir)
		}
		if err := InstallSkill(skV1, tg.Dir); err != nil {
			t.Fatalf("InstallSkill(%s): %v", tg.ID, err)
		}
	}
	for _, tg := range SkillTargets(home, skV1) {
		if !tg.Installed || !tg.Current {
			t.Errorf("%s: expected Installed & Current, got %+v", tg.ID, tg)
		}
	}
	for _, tg := range SkillTargets(home, skV2) {
		if !tg.Installed || tg.Current {
			t.Errorf("%s: expected Installed & !Current for v2, got %+v", tg.ID, tg)
		}
	}
}
