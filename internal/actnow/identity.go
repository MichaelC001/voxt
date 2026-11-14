package actnow

// VoxtMini's identity in the family. Until 0.1.0-beta.3 VoxtMini ran as
// dev.hehehai.voxtnew, the bundle id Voxt 2 (sglang-omni's Swift app)
// ships with; since then it is dev.hehehai.voxtmini (docs/behavior.md
// 9.21). actnowkit's tables (v0.18.0) do not list it yet: VoxtMini knows
// its own bundle ids here, and reads the family's shortcut defaults and
// pins under KitAppID until actnowkit has a voxtmini row.
//
// AdoptOld carries what the old id left in the family directory over to
// the new one at a start, when it was VoxtMini's: its model claims (the
// same repos, origins and times; no model is touched), its place among
// the users of the family's shared keys (the keys stay as they are), and
// removes its manifest and stale runtime file.

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/hehehai/actnowkit/credentials"
	"github.com/hehehai/actnowkit/family"
	fammodels "github.com/hehehai/actnowkit/models"
	"github.com/hehehai/actnowkit/registry"
	famruntime "github.com/hehehai/actnowkit/runtime"
)

// BundleID is VoxtMini's release bundle id (mygo.json).
const BundleID = "dev.hehehai.voxtmini"

// OldBundleID is the bundle id VoxtMini had until 0.1.0-beta.3.
const OldBundleID = "dev.hehehai.voxtnew"

// KitAppID is the row of actnowkit's tables (shortcuts.DefaultsOf,
// models.PinsOfApp) that holds VoxtMini's defaults and pins: still the
// old id's, as actnowkit v0.18.0 has no voxtmini row.
const KitAppID = "voxtnew"

// ownVariant reports whether bundleID is one of VoxtMini's apps (not its
// helper or language model process) and which build: the release, .dev,
// or another suffix (a test build: .updtest, .betatest…).
func ownVariant(bundleID string) (family.Variant, bool) {
	lower := strings.ToLower(bundleID)
	if lower == BundleID {
		return family.Release, true
	}
	rest, ok := strings.CutPrefix(lower, BundleID+".")
	switch {
	case !ok || rest == "":
		return "", false
	case rest == "dev":
		return family.Dev, true
	case strings.HasSuffix("."+rest, ".helper"), strings.HasSuffix("."+rest, ".llm"), strings.HasSuffix("."+rest, ".engine"):
		return "", false
	}
	return family.Test, true
}

// IsSelf reports whether a found app is a build of VoxtMini (this one or
// another: a dev build), by its manifest's app id or its bundle id; or
// whatever LaunchServices still knows at this bundle's path (the old id,
// after an update replaced the bundle).
func IsSelf(f registry.Found) bool {
	if f.Manifest != nil && f.Manifest.App == AppID {
		return true
	}
	if f.Path != "" && f.Path == registry.BundlePath() {
		return true
	}
	_, ok := ownVariant(f.BundleID)
	return ok || f.App.ID == AppID
}

// Adopted is what AdoptOld carried over.
type Adopted struct {
	Claims   int  // claims made again under the new id
	Keys     int  // shared keys whose users now name the new id
	Manifest bool // the old manifest was removed
}

// Any reports whether something changed.
func (a Adopted) Any() bool { return a.Claims > 0 || a.Keys > 0 || a.Manifest }

// oldIsOurs decides whether what the old id left is VoxtMini's: its
// manifest names VoxtMini; without a manifest, its claims name only
// models of VoxtMini's catalogue (known). Voxt 2 may run under the same
// id: anything else is left alone.
func (f *Family) oldIsOurs(known func(id string) bool) (ours, manifest bool) {
	m, err := registry.Read(f.dir, OldBundleID)
	if err == nil {
		return m.Name == "VoxtMini", true
	}
	c, err := fammodels.ReadClaims(f.dir, OldBundleID)
	if err != nil || len(c.Claims) == 0 {
		return false, false
	}
	for _, x := range c.Claims {
		users := x.Users()
		if len(users) == 0 {
			return false, false
		}
		for _, u := range users {
			if !known(u) {
				return false, false
			}
		}
	}
	return true, false
}

// AdoptOld moves the old id's family state to VoxtMini's id when it was
// VoxtMini's (see oldIsOurs); known says whether a model id is in
// VoxtMini's catalogue. It does nothing when VoxtMini runs as the old id,
// when the directory is read only, or once the old files are gone, so it
// runs at every start (a few stats). Errors are joined; what succeeded
// stays done.
func (f *Family) AdoptOld(ctx context.Context, known func(id string) bool) (Adopted, error) {
	var out Adopted
	if f.readOnly || strings.EqualFold(f.bundleID, OldBundleID) {
		return out, nil
	}
	ours, hasManifest := f.oldIsOurs(known)
	if !ours {
		return out, nil
	}
	var errs []error
	old := &fammodels.Store{Family: f.dir, Hub: f.store.Hub, BundleID: OldBundleID, AppID: KitAppID}
	if c, err := old.Claims(); err == nil {
		for _, x := range c.Claims {
			err := f.store.Claim(ctx, x) // keeps origin, since, models, files
			switch {
			case err == nil:
				out.Claims++
			case errors.Is(err, fammodels.ErrNotInstalled):
			default:
				errs = append(errs, err)
			}
		}
		if len(errs) == 0 {
			if err := old.Remove(); err != nil {
				errs = append(errs, err)
			}
		}
	} else if !family.IsNotExist(err) {
		errs = append(errs, err)
	}
	if hasManifest {
		n, err := f.adoptKeys(ctx)
		out.Keys = n
		if err != nil {
			errs = append(errs, err)
		}
		if err := registry.Remove(f.dir, OldBundleID); err != nil {
			errs = append(errs, err)
		} else {
			out.Manifest = true
		}
	}
	if rt, err := famruntime.Read(f.dir, OldBundleID); err == nil && !family.ProcessAlive(rt.PID) {
		if err := famruntime.Remove(f.dir, OldBundleID); err != nil {
			errs = append(errs, err)
		}
	}
	return out, errors.Join(errs...)
}

// adoptKeys renames the old id to the new one among the users of the
// family's shared keys, and as the one who added them; the Keychain items
// are not touched.
func (f *Family) adoptKeys(ctx context.Context) (int, error) {
	if _, readOnly, err := credentials.Read(f.dir); err != nil || readOnly {
		return 0, nil // no file, or one this VoxtMini cannot write
	}
	n := 0
	var file credentials.File
	err := f.dir.Update(ctx, family.LockCredentials, f.dir.CredentialsFile(), credentials.Kind, 1, &file, func(bool) error {
		n = 0
		for v, e := range file.Vendors {
			changed := false
			if i := slices.Index(e.Users, OldBundleID); i >= 0 {
				e.Users = slices.Delete(e.Users, i, i+1)
				if !slices.Contains(e.Users, f.bundleID) {
					e.Users = append(e.Users, f.bundleID)
				}
				changed = true
			}
			if e.AddedBy == OldBundleID {
				e.AddedBy, changed = f.bundleID, true
			}
			if changed {
				file.Vendors[v] = e
				n++
			}
		}
		if n == 0 {
			return family.ErrNoChange
		}
		file.Rev++
		return nil
	})
	if err == nil && n > 0 {
		_ = f.dir.Post(family.TopicCredentials)
	}
	return n, err
}
