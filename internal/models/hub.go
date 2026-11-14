package models

// The catalogue in the family's terms: a model as hfhub's download entry
// and as a claim (family spec §7.3), and where it is on disk.

import (
	"github.com/hehehai/actnowkit/hfhub"
	fammodels "github.com/hehehai/actnowkit/models"
)

// Role is the family's word for what a kind of model does (a claim's
// role).
func (k Kind) Role() string {
	switch k {
	case ASR:
		return fammodels.RoleASR
	case LLM:
		return fammodels.RoleLLM
	case VAD:
		return fammodels.RoleVAD
	}
	return fammodels.RoleOther
}

// Entry is m as hfhub downloads it, at revision rev ("": the pinned one):
// only the files the catalogue lists, the LFS ones with their SHA-256 (the
// Hub names the others' blobs by their git ids, which hfhub checks).
func (m Model) Entry(rev string) hfhub.Entry {
	if rev == "" {
		rev = m.Revision
	}
	e := hfhub.Entry{ID: m.ID, Repo: m.Repo, Revision: rev, Family: m.Family, Role: m.Kind.Role()}
	for _, f := range m.Files {
		hf := hfhub.File{Path: f.Path, Size: f.Size}
		if f.LFS {
			hf.SHA256 = f.SHA256
		}
		e.Files = append(e.Files, hf)
	}
	for _, a := range m.Aux {
		ha := hfhub.Aux{Repo: a.Repo, Revision: a.Revision}
		for _, f := range a.Files {
			hf := hfhub.File{Path: f.Path, Size: f.Size}
			if f.LFS {
				hf.SHA256 = f.SHA256
			}
			ha.Files = append(ha.Files, hf)
		}
		e.Aux = append(e.Aux, ha)
	}
	return e
}

// AuxClaims are VoxtMini's claims on m's auxiliary repos, made with
// origin (as hfhub makes them on a download: Aux, the model's id).
func (m Model) AuxClaims(origin string) []fammodels.Claim {
	var out []fammodels.Claim
	for _, a := range m.Aux {
		c := fammodels.Claim{Repo: a.Repo, Revision: a.Revision, Model: m.ID, Family: m.Family, Role: m.Kind.Role(), Aux: true, Origin: origin}
		for _, f := range a.Files {
			c.Files = append(c.Files, f.Path)
			if c.Sizes == nil {
				c.Sizes = map[string]int64{}
			}
			c.Sizes[f.Path] = f.Size
		}
		out = append(out, c)
	}
	return out
}

// auxInstalled reports whether every auxiliary repo of m is complete
// under hub.
func auxInstalled(hub string, m Model) bool {
	for _, c := range m.AuxClaims("") {
		if fammodels.Check(hub, c).State != fammodels.Installed {
			return false
		}
	}
	return true
}

// Claim is VoxtMini's claim on m at rev ("": the pinned one), made with
// origin (fammodels.OriginReused…).
func (m Model) Claim(rev, origin string) fammodels.Claim {
	if rev == "" {
		rev = m.Revision
	}
	c := fammodels.Claim{Repo: m.Repo, Revision: rev, Model: m.ID, Family: m.Family, Role: m.Kind.Role(), Origin: origin}
	for _, f := range m.Files {
		c.Files = append(c.Files, f.Path)
	}
	return c
}

// Ref is m's identity at rev ("": the pinned one).
func (m Model) Ref(rev string) fammodels.Ref {
	if rev == "" {
		rev = m.Revision
	}
	return fammodels.Ref{Repo: m.Repo, Revision: rev}
}

// find is the commit of m on disk under hub: the pinned one, else the
// first of Accept that is installed (fammodels.FindInstalled); "" when
// none is, or when an auxiliary repo is missing (the download completes
// it).
func find(hub string, m Model) (rev string, st fammodels.Status) {
	rev, st, ok := fammodels.FindInstalled(hub, m.Claim("", ""), m.Accept)
	if !ok || !auxInstalled(hub, m) {
		return "", st
	}
	return rev, st
}

// SnapshotDir is the folder a runtime loads m from when its commit rev is
// on disk under hub: $HUB/models--<owner>--<name>/snapshots/<rev>.
func SnapshotDir(hub string, m Model, rev string) string {
	return fammodels.SnapshotDir(hub, m.Ref(rev))
}

// Installed reports where m is installed under hub: its snapshot folder,
// "" when no commit it accepts is complete. Disk only; for tools and
// tests (the app asks its Manager, which keeps it in memory).
func Installed(hub string, m Model) string {
	rev, _ := find(hub, m)
	if rev == "" {
		return ""
	}
	return SnapshotDir(hub, m, rev)
}

// Hub is the family's default model library
// (~/Library/Application Support/ActNow/models/hub, spec v1.0.9), for
// tools and tests; the app resolves it with the family's settings first
// (internal/actnow).
func Hub() string { return fammodels.DefaultHub() }
