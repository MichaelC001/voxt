package models

// Models with auxiliary repos (Whisper's tokenizer): downloaded and
// claimed with them, installed only with them, loaded from a folder of
// links in VoxtMini's data folder, deleted safely for the family.

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hehehai/actnowkit/conformance"
	fammodels "github.com/hehehai/actnowkit/models"
)

var (
	repoW1  = conformance.TestRepo("mlx-community/test-whisper-1", "4444444444444444444444444444444444444444", 32<<10)
	repoW2  = conformance.TestRepo("mlx-community/test-whisper-2", "5555555555555555555555555555555555555555", 32<<10)
	repoTok = conformance.Repo{Ref: fammodels.Ref{Repo: "openai/test-whisper", Revision: "6666666666666666666666666666666666666666"},
		Files: []conformance.RepoFile{
			{Path: "vocab.json", Content: []byte(`{"a":0}`)},
			{Path: "merges.txt", Content: []byte("#version: 0.2\n")},
		}}
)

// auxModel is a model of r that needs repoTok.
func auxModel(id string, r conformance.Repo) Model {
	m := repoModel(id, ASR, r)
	m.Files = slices.DeleteFunc(m.Files, func(f File) bool { return f.Path == "tokenizer/vocab.json" })
	tok := repoModel("tok", ASR, repoTok)
	m.Aux = []Aux{{Repo: tok.Repo, Revision: tok.Revision, Files: tok.Files}}
	return m
}

func newAuxEnv(t *testing.T) (*testEnv, string) {
	fx := conformance.New(t)
	fx.Register(audiosmith)
	h := conformance.NewFakeHub(repoW1, repoW2, repoTok)
	t.Cleanup(h.Close)
	links := filepath.Join(t.TempDir(), "data", "models")
	m := Open(Options{Store: fx.Peer(voxtmini), Catalog: []Model{auxModel("w1", repoW1), auxModel("w2", repoW2)},
		Network: Network{Endpoints: []string{h.URL}}, ReserveBytes: -1, Links: links})
	t.Cleanup(m.Close)
	return &testEnv{fx: fx, hub: h, m: m}, links
}

// TestAuxDownload: a download fetches the model and its tokenizer, both
// claimed for VoxtMini (the tokenizer as auxiliary); RunDir is a folder
// of links to both snapshots in VoxtMini's folder, never in $HUB, made
// again when a link is wrong.
func TestAuxDownload(t *testing.T) {
	e, links := newAuxEnv(t)
	w1, _ := e.m.Lookup("w1")
	if w1.Size() != repoW1.Size()-int64(len(`{"a":0,"b":1}`))+repoTok.Size() {
		t.Errorf("size %d", w1.Size())
	}
	if err := e.m.Download("w1"); err != nil {
		t.Fatal(err)
	}
	if s := wait(t, e.m, "w1"); !s.Installed || !s.Mine || s.Err != nil {
		t.Fatalf("%+v", s)
	}
	cl := e.fx.Claims(voxtmini)
	if c, ok := cl.Find(repoTok.Ref); !ok || !c.Aux {
		t.Fatalf("tokenizer claim %+v %v", c, ok)
	}
	dir, err := e.m.RunDir("w1")
	if err != nil || dir != filepath.Join(links, "w1") {
		t.Fatalf("RunDir %q %v", dir, err)
	}
	for _, name := range []string{"config.json", "model.safetensors", "vocab.json", "merges.txt"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err != nil || len(b) == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	snap := fammodels.SnapshotDir(e.fx.Hub, repoTok.Ref)
	if got, _ := os.Readlink(filepath.Join(dir, "vocab.json")); got != filepath.Join(snap, "vocab.json") {
		t.Errorf("vocab.json links to %s", got)
	}
	// A wrong link is made again; a right folder is left as it is.
	os.Remove(filepath.Join(dir, "merges.txt"))
	if err := os.Symlink("/nowhere", filepath.Join(dir, "merges.txt")); err != nil {
		t.Fatal(err)
	}
	if d, err := e.m.RunDir("w1"); err != nil || d != dir {
		t.Fatal(d, err)
	}
	if got, _ := os.Readlink(filepath.Join(dir, "merges.txt")); got != filepath.Join(snap, "merges.txt") {
		t.Errorf("merges.txt links to %s", got)
	}
	// A model without auxiliary repos runs from its snapshot.
	if d, _ := e.m.RunDir("missing"); d != "" {
		t.Errorf("RunDir of nothing: %q", d)
	}
}

// TestAuxInstalled: the model without its tokenizer is not installed (a
// download completes it); a model on disk with it, claimed by nobody, is
// VoxtMini's when used, tokenizer included.
func TestAuxInstalled(t *testing.T) {
	e, _ := newAuxEnv(t)
	e.fx.Install(repoW1)
	e.m.Refresh()
	if s := e.m.Status("w1"); s.Installed || s.OnDisk {
		t.Fatalf("without the tokenizer: %+v", s)
	}
	e.fx.Install(repoTok)
	e.m.Refresh()
	if s := e.m.Status("w1"); !s.Installed {
		t.Fatalf("with it: %+v", s)
	}
	if err := e.m.Use(ctxT(t), "w1"); err != nil {
		t.Fatal(err)
	}
	if c, ok := e.fx.Claims(voxtmini).Find(repoTok.Ref); !ok || !c.Aux || c.Origin != fammodels.OriginReused {
		t.Errorf("tokenizer claim %+v %v", c, ok)
	}
}

// TestAuxDelete: two models share the tokenizer: deleting one keeps it
// (and the other model works), deleting the other removes it and both
// folders of links; another app's claim on the tokenizer keeps its files.
func TestAuxDelete(t *testing.T) {
	e, links := newAuxEnv(t)
	for _, id := range []string{"w1", "w2"} {
		if err := e.m.Download(id); err != nil {
			t.Fatal(err)
		}
		if s := wait(t, e.m, id); !s.Installed {
			t.Fatalf("%s: %+v", id, s)
		}
		if _, err := e.m.RunDir(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.fx.Peer(audiosmith).Claim(ctxT(t), fammodels.Claim{Repo: repoTok.Ref.Repo, Revision: repoTok.Ref.Revision, Aux: true, Origin: fammodels.OriginDownloaded}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Delete(ctxT(t), "w1", fammodels.ThisApp); err != nil {
		t.Fatal(err)
	}
	if e.fx.Installed(repoW1.Ref) || !tokThere(e) || !e.m.Installed("w2") {
		c, _ := e.fx.Claims(voxtmini).Find(repoTok.Ref)
		t.Fatalf("w1's delete: w1 %v tok %v w2 %+v; claim %+v", e.fx.Installed(repoW1.Ref), tokThere(e), e.m.Status("w2"), c)
	}
	if _, err := os.Stat(filepath.Join(links, "w1")); !os.IsNotExist(err) {
		t.Errorf("w1's links stay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(links, "w2", "vocab.json")); err != nil {
		t.Errorf("w2's links: %v", err)
	}
	if _, err := e.m.Delete(ctxT(t), "w2", fammodels.ThisApp); err != nil {
		t.Fatal(err)
	}
	if e.fx.Installed(repoW2.Ref) || !tokThere(e) {
		t.Fatal("AudioSmith's tokenizer was deleted, or w2 stayed")
	}
	if _, ok := e.fx.Claims(voxtmini).Find(repoTok.Ref); ok {
		t.Error("VoxtMini still claims the tokenizer")
	}
	if entries, _ := os.ReadDir(links); len(entries) != 0 {
		t.Errorf("links left: %v", entries)
	}
}

// TestAuxNoLinks: without Options.Links a model with auxiliary repos has
// no folder to run from.
func TestAuxNoLinks(t *testing.T) {
	fx := conformance.New(t)
	fx.Install(repoW1)
	fx.Install(repoTok)
	m := Open(Options{Store: fx.Peer(voxtmini), Catalog: []Model{auxModel("w1", repoW1)}, ReserveBytes: -1})
	t.Cleanup(m.Close)
	if _, err := m.RunDir("w1"); err == nil {
		t.Error("no error")
	}
}

// tokThere reports whether the tokenizer is complete in $HUB (an
// auxiliary repo has no weights file: fx.Installed would say no).
func tokThere(e *testEnv) bool {
	c := fammodels.Claim{Repo: repoTok.Ref.Repo, Revision: repoTok.Ref.Revision, Aux: true}
	return fammodels.Check(e.fx.Hub, c).State == fammodels.Installed
}
