package models

// The folder a runtime loads a model with auxiliary repos from: the
// servers read one directory (whisper_server: the MLX checkpoint and the
// tokenizer files of openai/whisper-*), and the family's library keeps
// each repo in a snapshot of its own. VoxtMini never writes into $HUB
// (other apps read it): the folder is VoxtMini's, in its data folder
// (Options.Links), made of links to the snapshots' files, made again when
// they point elsewhere, and removed when the model is deleted.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	fammodels "github.com/hehehai/actnowkit/models"
)

// ErrNoLinks: a model with auxiliary repos, and no Options.Links to put
// its folder in.
var ErrNoLinks = errors.New("models: no folder for the models' links")

// RunDir is the folder a runtime loads model id from: its snapshot
// (Dir), or for a model with auxiliary repos the folder of links in
// Options.Links/<id>, checked and made again when needed. "" and no
// error when it is not installed. It touches the disk: off the main
// thread.
func (m *Manager) RunDir(id string) (string, error) {
	dir := m.Dir(id)
	mod, _ := m.Lookup(id)
	if dir == "" || len(mod.Aux) == 0 {
		return dir, nil
	}
	if m.opt.Links == "" {
		return "", fmt.Errorf("%w: %s", ErrNoLinks, id)
	}
	want := map[string]string{} // name → target
	for _, f := range mod.Files {
		want[f.Path] = filepath.Join(dir, filepath.FromSlash(f.Path))
	}
	for _, a := range mod.Aux {
		snap := fammodels.SnapshotDir(m.store.Hub, fammodels.Ref{Repo: a.Repo, Revision: a.Revision})
		for _, f := range a.Files {
			if _, dup := want[f.Path]; !dup { // the model's own file wins
				want[f.Path] = filepath.Join(snap, filepath.FromSlash(f.Path))
			}
		}
	}
	m.linksMu.Lock()
	defer m.linksMu.Unlock()
	out := filepath.Join(m.opt.Links, id)
	if linked(out, want) {
		return out, nil
	}
	if err := makeLinks(out, want); err != nil {
		return "", fmt.Errorf("models: the folder of %s: %w", id, err)
	}
	return out, nil
}

// linked reports whether dir holds exactly the links want.
func linked(dir string, want map[string]string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(want) {
		return false
	}
	for name, target := range want {
		if got, err := os.Readlink(filepath.Join(dir, filepath.FromSlash(name))); err != nil || got != target {
			return false
		}
	}
	return true
}

// makeLinks makes dir again with the links want: in a new folder beside
// it, which then takes its place (a runtime reading the old one keeps
// its open files).
func makeLinks(dir string, want map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), filepath.Base(dir)+".new-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for name, target := range want {
		p := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(target, p); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}

// dropLinks removes model id's folder of links (its files are gone):
// only VoxtMini's folder, never what the links point to.
func (m *Manager) dropLinks(id string) {
	mod, ok := m.Lookup(id)
	if !ok || len(mod.Aux) == 0 || m.opt.Links == "" {
		return
	}
	m.linksMu.Lock()
	defer m.linksMu.Unlock()
	_ = os.RemoveAll(filepath.Join(m.opt.Links, id))
}
