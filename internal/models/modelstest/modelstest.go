// Package modelstest installs models of the catalogue for tests, without
// their bytes: in a model library ($HUB) laid out as the Hugging Face
// cache, each file a sparse blob of its size linked from the snapshot,
// which is all the installed check reads (hashes are checked as files
// download).
package modelstest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hehehai/VoxtMini/internal/models"
)

// Seed installs the models ids at their pinned commits under hub.
func Seed(hub string, ids ...string) error {
	for _, id := range ids {
		m, ok := models.Lookup(id)
		if !ok {
			return fmt.Errorf("modelstest: unknown model %q", id)
		}
		if err := SeedModel(hub, m); err != nil {
			return err
		}
	}
	return nil
}

// SeedModel installs m (a borrowed model: one weights file of its size),
// and its auxiliary repos.
func SeedModel(hub string, m models.Model) error {
	files := m.Files
	if len(files) == 0 {
		files = []models.File{{Path: "model.safetensors", Size: m.Bytes, SHA256: fmt.Sprintf("%064x", len(m.ID))}}
	}
	if err := seedRepo(hub, m.Repo, m.Revision, files); err != nil {
		return err
	}
	for _, a := range m.Aux {
		if err := seedRepo(hub, a.Repo, a.Revision, a.Files); err != nil {
			return err
		}
	}
	return nil
}

// seedRepo lays out files of repo at revision under hub.
func seedRepo(hub, repoID, revision string, files []models.File) error {
	repo := filepath.Join(hub, "models--"+strings.ReplaceAll(repoID, "/", "--"))
	for _, f := range files {
		blob := filepath.Join(repo, "blobs", f.SHA256)
		if err := os.MkdirAll(filepath.Dir(blob), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(blob); err != nil {
			file, err := os.Create(blob)
			if err != nil {
				return err
			}
			err = file.Truncate(f.Size)
			file.Close()
			if err != nil {
				return err
			}
		}
		link := filepath.Join(repo, "snapshots", revision, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			return err
		}
		os.Remove(link)
		target := strings.Repeat("../", strings.Count(f.Path, "/")+2) + "blobs/" + f.SHA256
		if err := os.Symlink(target, link); err != nil {
			return err
		}
	}
	return nil
}
