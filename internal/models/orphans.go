package models

// The models in the family's library that only uninstalled ActNow apps
// claimed (family spec §7.6 step 4, §7.7): Settings → Storage lists them
// and deletes those the user picks. An uninstalled app's claim protects
// nothing, but what an installed app (VoxtMini too) claims, what a
// running app has loaded, and anything the family's files leave unsure
// stays: each deletion decides again holding the repo's lock
// (fammodels.Store.Delete, scope ThisApp).

import (
	"cmp"
	"context"
	"errors"
	"path"
	"slices"
	"strings"

	fammodels "github.com/hehehai/actnowkit/models"
)

// Orphan is a model on disk that only uninstalled apps claimed.
type Orphan struct {
	Ref fammodels.Ref
	// Name is the catalogue's title when VoxtMini knows the repo, else the
	// repo's name.
	Name string
	// Apps are the uninstalled apps that claimed it (their names), by
	// their bundle ids.
	Apps, BundleIDs []string
	// Bytes is what deleting it frees.
	Bytes int64
}

// Orphans lists the models only uninstalled apps claimed, largest first.
// It reads the family's files and the disk: off the main thread.
func (m *Manager) Orphans() ([]Orphan, error) {
	apps, err := m.store.Orphans()
	if err != nil {
		return nil, err
	}
	byRef := map[fammodels.Ref]*Orphan{}
	var refs []fammodels.Ref
	for _, app := range apps {
		for _, c := range app.Claims {
			ref := c.Ref()
			if o, ok := byRef[ref]; ok {
				if !slices.Contains(o.BundleIDs, app.BundleID) {
					o.Apps, o.BundleIDs = append(o.Apps, app.Name), append(o.BundleIDs, app.BundleID)
				}
				continue
			}
			byRef[ref] = &Orphan{Ref: ref, Name: m.repoName(ref.Repo), Apps: []string{app.Name}, BundleIDs: []string{app.BundleID}}
			refs = append(refs, ref)
		}
	}
	var out []Orphan
	for _, ref := range refs {
		d, err := m.store.Decide(ref)
		if err != nil || d.Mine || d.InUse() || d.Status.State == fammodels.Missing {
			continue
		}
		o := byRef[ref]
		o.Bytes = d.FreeBytes
		out = append(out, *o)
	}
	slices.SortStableFunc(out, func(a, b Orphan) int {
		return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), strings.Compare(a.Name, b.Name))
	})
	return out, nil
}

// repoName names a repo for the list.
func (m *Manager) repoName(repo string) string {
	for _, mod := range m.catalog {
		if mod.Repo == repo && mod.Title != "" {
			return mod.Title
		}
	}
	return path.Base(repo)
}

// CleanResult is what DeleteOrphans did.
type CleanResult struct {
	Deleted int   // models whose files went
	Kept    int   // models another app claimed or loaded meanwhile
	Freed   int64 // bytes
}

// DeleteOrphans deletes the models refs (from Orphans), each deciding
// again under its lock; then the uninstalled apps none of whose models
// is left to clean up lose their files in the family directory (claims,
// manifest, runtime: fammodels.Store.CleanOrphans). A model another app
// downloads fails with a *fammodels.BusyError (Busy) and the others go
// on. It blocks on the disk: off the main thread.
func (m *Manager) DeleteOrphans(ctx context.Context, refs []fammodels.Ref) (CleanResult, error) {
	var r CleanResult
	var errs []error
	for _, ref := range refs {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		res, err := m.store.Delete(ctx, ref, fammodels.ThisApp)
		r.Freed += res.Freed
		switch {
		case err != nil:
			errs = append(errs, err)
		case res.Deleted:
			r.Deleted++
		default:
			r.Kept++
		}
	}
	// An uninstalled app with nothing left to clean up: its files go.
	left, err := m.Orphans()
	if err == nil {
		apps, _ := m.store.Orphans()
		for _, app := range apps {
			if slices.ContainsFunc(left, func(o Orphan) bool { return slices.Contains(o.BundleIDs, app.BundleID) }) {
				continue
			}
			if _, err := m.store.CleanOrphans(ctx, app.BundleID); err != nil {
				errs = append(errs, err)
			}
		}
	}
	m.Refresh()
	return r, errors.Join(errs...)
}
