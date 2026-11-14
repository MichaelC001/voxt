package datadir

// VoxtMini's folders under its old name. Until 0.1.0-beta.3 VoxtMini
// kept its data, caches and logs under MyGo's app name "VoxtNew"
// (~/Library/{Application Support,Caches,Logs}/VoxtNew), which Voxt 2,
// another app with that name, may use too. Since then they are
// …/VoxtMini. AdoptOldName moves VoxtMini's own entries over at the first
// start, before anything opens them; whatever else is in the old folders
// stays where it is.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hehehai/VoxtMini/internal/store/sqlitestore"
)

// OldName is the folders' name until 0.1.0-beta.3.
const OldName = "VoxtNew"

// Dirs are VoxtMini's three folders: the default data folder
// (mygo.PathUserData), the caches and the logs.
type Dirs struct{ Data, Cache, Logs string }

// Renamed is the same folders under another name (their last element).
func (d Dirs) Renamed(name string) Dirs {
	re := func(p string) string {
		if p == "" {
			return ""
		}
		return filepath.Join(filepath.Dir(p), name)
	}
	return Dirs{Data: re(d.Data), Cache: re(d.Cache), Logs: re(d.Logs)}
}

// The entries of each folder that are VoxtMini's: a name, or a prefix
// ending in "*" (the database's -wal, -shm and .bak-<v>, rotated logs).
var (
	ownData = []string{sqlitestore.FileName + "*", "audio", "models", "output-mute",
		LocationFile, MoveFile, ResetFile,
		// MyGo's: the updater's state, the window's frame, the language.
		"updater.json", "window-state.json", "locale", "recent-documents.json"}
	ownCache = []string{"icons", "output-mute"}
	ownLogs  = []string{"voxtmini.log*", "helper.log*", "llm.log*", "crash.log*", "stderr.log*"}
)

func own(name string, patterns []string) bool {
	return slices.ContainsFunc(patterns, func(p string) bool {
		if pre, ok := strings.CutSuffix(p, "*"); ok {
			return strings.HasPrefix(name, pre)
		}
		return name == p
	})
}

// Adopted is what AdoptOldName did.
type Adopted struct {
	Moved []string // the entries moved, as old paths
	Left  []string // entries of the old folders that are not VoxtMini's, left in place
	Paths int64    // the database's rows whose paths now name the new folder
}

// AdoptOldName moves VoxtMini's entries from the folders of its old
// name (old) to the new ones (now), when the old data folder is
// VoxtMini's (it holds its database or a location.json) and the new one
// holds neither yet: once, at the start, before the library opens. Each
// entry is renamed (same volume), else copied, checked and removed; one
// the new folder already has stays. The database's paths into the old
// data folder (recordings) then name the new one, as do the location
// files'. Old folders left empty are removed. Errors are joined: a
// failed entry stays in the old folder.
func AdoptOldName(ctx context.Context, old, now Dirs) (Adopted, error) {
	var out Adopted
	if old.Data == "" || old.Data == now.Data || !isData(old.Data) || isData(now.Data) {
		return out, nil
	}
	var errs []error
	for _, f := range []struct {
		from, to string
		own      []string
	}{{old.Data, now.Data, ownData}, {old.Cache, now.Cache, ownCache}, {old.Logs, now.Logs, ownLogs}} {
		if f.from == "" || f.to == "" || f.from == f.to {
			continue
		}
		moved, left, err := moveOwn(ctx, f.from, f.to, f.own)
		out.Moved, out.Left = append(out.Moved, moved...), append(out.Left, left...)
		if err != nil {
			errs = append(errs, err)
		}
	}
	n, err := repoint(ctx, old.Data, now.Data)
	out.Paths = n
	if err != nil {
		errs = append(errs, err)
	}
	return out, errors.Join(errs...)
}

// isData: dir holds VoxtMini's database or says where it moved.
func isData(dir string) bool {
	for _, name := range []string{sqlitestore.FileName, LocationFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// moveOwn moves from's entries that own names into to, and removes from
// when nothing (but a .DS_Store) is left.
func moveOwn(ctx context.Context, from, to string, patterns []string) (moved, left []string, err error) {
	ents, err := os.ReadDir(from)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return nil, nil, err
	}
	var errs []error
	for _, e := range ents {
		src, dst := filepath.Join(from, e.Name()), filepath.Join(to, e.Name())
		if !own(e.Name(), patterns) {
			if e.Name() != ".DS_Store" {
				left = append(left, src)
			}
			continue
		}
		if _, err := os.Lstat(dst); err == nil {
			left = append(left, src)
			errs = append(errs, fmt.Errorf("%s: %s exists already", src, dst))
			continue
		}
		if err := move(ctx, src, dst, e.IsDir()); err != nil {
			left = append(left, src)
			errs = append(errs, err)
			continue
		}
		moved = append(moved, src)
	}
	if len(left) == 0 {
		os.Remove(filepath.Join(from, ".DS_Store"))
		os.Remove(from) // only when empty
	}
	return moved, left, errors.Join(errs...)
}

// rename is os.Rename (tests make it fail, as across volumes).
var rename = os.Rename

// move renames src to dst, else copies it (checked) and removes src.
func move(ctx context.Context, src, dst string, dir bool) error {
	if err := rename(src, dst); err == nil {
		return nil
	}
	var err error
	if dir {
		err = CopyTree(ctx, src, dst, func(string) bool { return false }, nil)
	} else {
		_, err = copyVerified(src, dst)
	}
	if err != nil {
		os.RemoveAll(dst)
		return fmt.Errorf("moving %s: %w", src, err)
	}
	return os.RemoveAll(src)
}

// repoint makes what names the old data folder name the new one: the
// database's paths (in the folder in use: the new default one, or the
// one location.json names) and the location files'.
func repoint(ctx context.Context, old, now string) (int64, error) {
	var errs []error
	loc := ReadLocation(now)
	if under(loc.Previous, old) {
		loc.Previous = swap(loc.Previous, old, now)
		errs = append(errs, WriteJSON(filepath.Join(now, LocationFile), loc))
	}
	var mv PendingMove
	if readJSON(filepath.Join(now, MoveFile), &mv) && (under(mv.From, old) || under(mv.To, old)) {
		mv.From, mv.To = swap(mv.From, old, now), swap(mv.To, old, now)
		errs = append(errs, WriteJSON(filepath.Join(now, MoveFile), mv))
	}
	var rs PendingReset
	if readJSON(filepath.Join(now, ResetFile), &rs) && len(rs.Also) > 0 {
		// The caches and logs to erase: the old folders hold no more of
		// VoxtMini's, and maybe another app's.
		for i, d := range rs.Also {
			rs.Also[i] = swapName(d, filepath.Base(now))
		}
		errs = append(errs, WriteJSON(filepath.Join(now, ResetFile), rs))
	}
	db := filepath.Join(now, sqlitestore.FileName)
	if loc.Dir != "" {
		db = filepath.Join(loc.Dir, sqlitestore.FileName)
	}
	var n int64
	if _, err := os.Stat(db); err == nil {
		var rerr error
		n, rerr = sqlitestore.RewritePaths(ctx, db, old, now)
		if rerr != nil {
			errs = append(errs, fmt.Errorf("the library's paths: %w", rerr))
		}
	}
	return n, errors.Join(errs...)
}

// swapName is a folder of the old name (…/VoxtNew) under the new one.
func swapName(p, newName string) string {
	if filepath.Base(p) == OldName {
		return filepath.Join(filepath.Dir(p), newName)
	}
	return p
}

// under reports whether p is dir or inside it.
func under(p, dir string) bool {
	return p != "" && (p == dir || strings.HasPrefix(p, dir+string(filepath.Separator)))
}

// swap replaces the folder old at the start of p by now.
func swap(p, old, now string) string {
	if !under(p, old) {
		return p
	}
	return now + strings.TrimPrefix(p, old)
}
