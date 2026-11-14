package sqlitestore

// The store's disk hygiene (audit #37, WP-F): old backups go after a
// migration, a large delete gives its space back.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/store"
)

// TestOldBackupsDropped: a migration keeps its own backup only; older
// versions' go, and files that are not backups stay.
func TestOldBackupsDropped(t *testing.T) {
	dir := t.TempDir()
	saved := migrations
	t.Cleanup(func() { migrations = saved })
	openTest(t, dir).Close()
	cur := len(saved)
	path := filepath.Join(dir, FileName)
	for _, name := range []string{".bak-1", ".bak-" + strconv.Itoa(cur-1), ".bak-x"} {
		os.WriteFile(path+name, []byte("old"), 0o644)
	}
	migrations = append(slices.Clone(saved), migration{version: cur + 1, statements: []string{"CREATE TABLE extra (x)"}})
	openTest(t, dir).Close()
	baks, _ := filepath.Glob(path + ".bak-*")
	var got []string
	for _, b := range baks {
		got = append(got, strings.TrimPrefix(b, path))
	}
	slices.Sort(got)
	if want := []string{".bak-" + strconv.Itoa(cur), ".bak-x"}; !slices.Equal(got, want) {
		t.Errorf("backups %v, want %v", got, want)
	}
}

// TestPruneVacuums: a clean-up of more than vacuumAfter entries shrinks
// the file; a small one leaves it alone.
func TestPruneVacuums(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	h := s.History()
	t0 := time.Now()
	text := strings.Repeat("words to keep the pages busy ", 40)
	for i := range vacuumAfter + 200 {
		e := entry(fmt.Sprintf("e-%d", i), store.KindTranscribe, t0.Add(-time.Duration(i)*time.Minute), text, text)
		if err := h.Put(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	size := func() int64 {
		var n int64
		for _, f := range []string{"", "-wal"} {
			if st, err := os.Stat(filepath.Join(dir, FileName+f)); err == nil {
				n += st.Size()
			}
		}
		return n
	}
	s.db.Query(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	full := size()
	// A small clean-up: the pages stay free in the file.
	if n, err := h.Prune(ctx, store.Retention{PerKind: vacuumAfter + 100}); err != nil || n != 100 {
		t.Fatalf("pruned %d: %v", n, err)
	}
	s.db.Query(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	if after := size(); after < full*9/10 {
		t.Errorf("a small clean-up vacuumed: %d → %d", full, after)
	}
	if n, err := h.Prune(ctx, store.Retention{PerKind: 100}); err != nil || n != vacuumAfter {
		t.Fatalf("pruned %d: %v", n, err)
	}
	after := size()
	t.Logf("%d entries: %d bytes; 100 left after the clean-up: %d", vacuumAfter+200, full, after)
	if after > full/2 {
		t.Errorf("not smaller: %d → %d", full, after)
	}
	if ids := historyIDs(t, h); len(ids) != 100 {
		t.Errorf("%d entries left", len(ids))
	}
}
