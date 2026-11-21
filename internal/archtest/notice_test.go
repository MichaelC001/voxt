package archtest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// noticeElsewhere are the modules whose licence text is not a file of
// LICENSES/go named after their version: the Apache-2.0 ones share
// LICENSES/Apache-2.0.txt; MyGo comes from ActNow's fork (go.mod's
// replace), whose licence is the module's LICENSE (TestMyGoLicence).
var noticeElsewhere = map[string]string{
	"github.com/ebitengine/purego":            "LICENSES/Apache-2.0.txt",
	"github.com/hehehai/actnowkit":            "LICENSES/Apache-2.0.txt",
	"github.com/hehehai/actnowkit/feedbackui": "LICENSES/Apache-2.0.txt",
	"github.com/egoist/mygo":                  "LICENSES/go/github.com_egoist_mygo.LICENSE.txt",
}

// toolOnly are the modules go.mod requires for the mygo command only (its
// `tool` line: packaging for Linux, archives, its HTTP client), none of
// them compiled into VoxtMini (`GOOS=darwin go list -deps ./app` names
// none), so NOTICE does not list them. A module that VoxtMini's own code
// comes to import leaves this list and gets its notice.
var toolOnly = map[string]bool{
	"github.com/ProtonMail/go-crypto": true,
	"github.com/cloudflare/circl":     true,
	"github.com/klauspost/compress":   true,
	"github.com/ulikunitz/xz":         true,
	"go.digitalxero.dev/rpm":          true,
	"golang.org/x/crypto":             true,
	"golang.org/x/net":                true,
	"golang.org/x/sys":                true,
}

// requires returns the modules go.mod requires, path → version.
func requires(t *testing.T, gomod string) map[string]string {
	t.Helper()
	mods := map[string]string{}
	block := false
	for line := range strings.Lines(gomod) {
		line, _, _ = strings.Cut(strings.TrimSpace(line), "//")
		f := strings.Fields(line)
		switch {
		case len(f) == 2 && f[0] == "require" && f[1] == "(":
			block = true
		case block && len(f) == 1 && f[0] == ")":
			block = false
		case block && len(f) == 2:
			mods[f[0]] = f[1]
		case !block && len(f) == 3 && f[0] == "require":
			mods[f[1]] = f[2]
		}
	}
	if len(mods) == 0 {
		t.Fatal("go.mod requires no module: the parser is wrong")
	}
	return mods
}

// TestNotice: every module go.mod requires is named in NOTICE with its
// version, and its licence text is in the repository (AGENTS.md
// "Licence"), so that a new or updated dependency cannot ship without its
// notice.
func TestNotice(t *testing.T) {
	root, _ := moduleRoot(t)
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	notice := read("NOTICE")
	for path, version := range requires(t, read("go.mod")) {
		if toolOnly[path] {
			continue
		}
		licence, ok := noticeElsewhere[path]
		if ok {
			if !strings.Contains(notice, path) {
				t.Errorf("NOTICE does not name %s: add it to \"Third-party material\"", path)
			}
		} else {
			if !strings.Contains(notice, path+" "+version) {
				t.Errorf("NOTICE does not name %s %s: add or update its line under \"Go modules compiled into VoxtMini\"", path, version)
			}
			licence = "LICENSES/go/" + strings.ReplaceAll(path, "/", "_") + ".LICENSE.txt"
		}
		if _, err := os.Stat(filepath.Join(root, licence)); err != nil {
			t.Errorf("%s: no licence text at %s: copy the module's LICENSE file there, unchanged", path, licence)
		}
	}
}

// TestMyGoLicence: MyGo's licence in LICENSES/ (which VoxtMini.app
// carries, mygo.json "resources") is that of the module go.mod replaces it
// with, unchanged.
func TestMyGoLicence(t *testing.T) {
	root, _ := moduleRoot(t)
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	fork, ok := replacement(string(mod), "github.com/egoist/mygo")
	if !ok {
		t.Fatal("go.mod does not replace github.com/egoist/mygo with ActNow's fork")
	}
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Skipf("go env GOMODCACHE: %v", err)
	}
	upstream, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), escapePath(fork.Path)+"@"+fork.Version, "LICENSE"))
	if err != nil {
		t.Skipf("the fork's module is not downloaded: %v", err)
	}
	copied, err := os.ReadFile(filepath.Join(root, noticeElsewhere["github.com/egoist/mygo"]))
	if err != nil || !bytes.Equal(copied, upstream) {
		t.Errorf("%s is not %s@%s's LICENSE: copy it again (%v)", noticeElsewhere["github.com/egoist/mygo"], fork.Path, fork.Version, err)
	}
}

// escapePath is path as the module cache names its directory: an upper
// case letter is "!" and the letter in lower case.
func escapePath(path string) string {
	var b strings.Builder
	for _, r := range path {
		if 'A' <= r && r <= 'Z' {
			b.WriteByte('!')
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// replacement returns the module and version that go.mod's replace
// directive puts in place of path.
func replacement(gomod, path string) (m struct{ Path, Version string }, ok bool) {
	for line := range strings.Lines(gomod) {
		line, _, _ = strings.Cut(strings.TrimSpace(line), "//")
		f := strings.Fields(strings.TrimPrefix(line, "replace "))
		if len(f) == 4 && f[0] == path && f[1] == "=>" {
			m.Path, m.Version = f[2], f[3]
			return m, true
		}
	}
	return m, false
}
