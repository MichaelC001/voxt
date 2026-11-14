package models

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/hehehai/actnowkit/conformance"
	fammodels "github.com/hehehai/actnowkit/models"
	famruntime "github.com/hehehai/actnowkit/runtime"
)

// TestOrphans: the list holds the models on disk only uninstalled apps
// claimed (not one an installed app, VoxtMini or a running app's memory
// holds, nor one gone from disk), largest first, named by the catalogue;
// deleting some keeps the rest and the apps' files until nothing of
// theirs is left. Scratch family and $HUB (conformance.New).
func TestOrphans(t *testing.T) {
	fx := conformance.New(t)
	ctx := context.Background()
	repo := func(name string, size int) conformance.Repo {
		r := conformance.TestRepo("org/"+name, "0123456789abcdef0123456789abcdef01234567", size)
		fx.Install(r)
		return r
	}
	alone, both, shared, mine, loaded := repo("alone", 3000), repo("both", 2000), repo("shared", 1000), repo("mine", 1000), repo("loaded", 1000)
	missing := repo("missing", 10)
	claim := func(bid string, rs ...conformance.Repo) {
		t.Helper()
		var cs []fammodels.Claim
		for _, r := range rs {
			cs = append(cs, fammodels.Claim{Repo: r.Ref.Repo, Revision: r.Ref.Revision})
		}
		if err := fx.Peer(bid).Claim(ctx, cs...); err != nil {
			t.Fatal(err)
		}
	}
	claim(conformance.PeerGone, alone, both, shared, mine, loaded, missing)
	if err := os.RemoveAll(fammodels.RepoDir(fx.Hub, missing.Ref.Repo)); err != nil { // deleted by hand
		t.Fatal(err)
	}
	claim(conformance.PeerAuralink, both)
	claim(conformance.PeerSmith, shared)
	claim(conformance.AppUnderTest, mine)
	for _, bid := range []string{conformance.PeerGone, conformance.PeerAuralink} {
		st := fx.Apps[bid]
		st.Installed = false
		fx.Apps[bid] = st
	}
	// PeerSmith runs and has loaded "loaded" (it claims it no more).
	if err := famruntime.Write(fx.Family, famruntime.File{BundleID: conformance.PeerSmith, PID: os.Getpid(),
		Loaded: []famruntime.Loaded{{Repo: loaded.Ref.Repo, Revision: loaded.Ref.Revision}}}); err != nil {
		t.Fatal(err)
	}

	cat := []Model{{ID: "x", Kind: LLM, Runtime: MLX, Repo: both.Ref.Repo, Revision: both.Ref.Revision, Title: "Both 1B"}}
	m := Open(Options{Store: fx.Peer(conformance.AppUnderTest), Catalog: cat, ReserveBytes: -1})
	defer m.Close()
	list, err := m.Orphans()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, o := range list {
		names = append(names, o.Name)
	}
	if !slices.Equal(names, []string{"alone", "Both 1B"}) {
		t.Fatalf("orphans %+v", list)
	}
	if o := list[1]; !slices.Equal(o.BundleIDs, []string{conformance.PeerAuralink, conformance.PeerGone}) && !slices.Equal(o.BundleIDs, []string{conformance.PeerGone, conformance.PeerAuralink}) {
		t.Errorf("both's apps %+v", o)
	}
	if list[0].Bytes < 3000 || list[1].Bytes < 2000 {
		t.Errorf("sizes %+v", list)
	}

	// The largest only: the other stays, and so do both apps' files.
	r, err := m.DeleteOrphans(ctx, []fammodels.Ref{alone.Ref})
	if err != nil || r.Deleted != 1 || r.Freed < 3000 {
		t.Fatalf("delete alone: %+v %v", r, err)
	}
	if fx.Installed(alone.Ref) || !fx.Installed(both.Ref) {
		t.Fatal("deleted the wrong model")
	}
	if !fx.Claimed(conformance.PeerGone, both.Ref) || !fx.Claimed(conformance.PeerAuralink, both.Ref) {
		t.Error("an app's claims went while it had a model left")
	}

	// Then the rest: the apps' claims go; what others use stays.
	if r, err := m.DeleteOrphans(ctx, []fammodels.Ref{both.Ref}); err != nil || r.Deleted != 1 {
		t.Fatalf("delete both: %+v %v", r, err)
	}
	for _, ref := range []fammodels.Ref{shared.Ref, mine.Ref, loaded.Ref} {
		if !fx.Installed(ref) {
			t.Errorf("%s was deleted", ref.Repo)
		}
	}
	for _, bid := range []string{conformance.PeerGone, conformance.PeerAuralink} {
		if c := fx.Claims(bid); len(c.Claims) != 0 {
			t.Errorf("%s keeps claims %+v", bid, c.Claims)
		}
	}
	if list, _ := m.Orphans(); len(list) != 0 {
		t.Errorf("left %+v", list)
	}
}
