package datadir

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hehehai/VoxtMini/internal/store/sqlitestore"
)

// library is a temporary ~/Library: VoxtMini's folders under the new
// name, and the old ones.
func library(t *testing.T) (old, now Dirs) {
	lib := t.TempDir()
	now = Dirs{
		Data:  filepath.Join(lib, "Application Support", "VoxtMini"),
		Cache: filepath.Join(lib, "Caches", "VoxtMini"),
		Logs:  filepath.Join(lib, "Logs", "VoxtMini"),
	}
	for _, d := range []string{now.Data, now.Cache, now.Logs} {
		os.MkdirAll(d, 0o755) // MyGo makes them when asked for their paths
	}
	return now.Renamed(OldName), now
}

// putDoc writes a document holding path into the library in dir.
func putDoc(t *testing.T, dir, path string) {
	t.Helper()
	st, err := sqlitestore.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Documents().Put(context.Background(), "k", []byte(`{"audio":"`+path+`"}`)); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
}

func getDoc(t *testing.T, dir string) string {
	t.Helper()
	st, err := sqlitestore.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	doc, _, _ := st.Documents().Get(context.Background(), "k")
	return string(doc)
}

func TestAdoptOldName(t *testing.T) {
	old, now := library(t)
	putDoc(t, old.Data, filepath.Join(old.Data, "audio", "a.wav"))
	write(t, old.Data, map[string]string{
		"audio/a.wav": "RIFF", "models/whisper/x": "link", "updater.json": "{}", "window-state.json": "{}",
		sqlitestore.FileName + ".bak-3": "bak",
		"Voxt2.sqlite":                  "another app's", // not VoxtMini's: stays
	})
	write(t, old.Cache, map[string]string{"icons/com.apple.TextEdit-64.png": "png", ".DS_Store": ""})
	write(t, old.Logs, map[string]string{"voxtmini.log": "l", "voxtmini.log.1": "l1", "helper.log": "h", "crash.log": "c"})

	got, err := AdoptOldName(context.Background(), old, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(old.Data, "Voxt2.sqlite")}; !slices.Equal(got.Left, want) {
		t.Errorf("left %v, want %v", got.Left, want)
	}
	if got.Paths == 0 {
		t.Error("no path in the library was updated")
	}
	files := read(t, now.Data)
	for _, f := range []string{"audio/a.wav", "models/whisper/x", "updater.json", "window-state.json", sqlitestore.FileName, sqlitestore.FileName + ".bak-3"} {
		if _, ok := files[f]; !ok {
			t.Errorf("%s did not move: %v", f, files)
		}
	}
	if doc := getDoc(t, now.Data); doc != `{"audio":"`+filepath.Join(now.Data, "audio", "a.wav")+`"}` {
		t.Errorf("the library's path: %s", doc)
	}
	if data, _ := os.ReadFile(filepath.Join(old.Data, "Voxt2.sqlite")); string(data) != "another app's" {
		t.Error("another app's file was touched")
	}
	if logs := read(t, now.Logs); len(logs) != 4 || logs["voxtmini.log.1"] != "l1" {
		t.Errorf("logs: %v", logs)
	}
	for _, d := range []string{old.Cache, old.Logs} { // emptied: removed
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("%s is still there", d)
		}
	}
	if read(t, now.Cache)["icons/com.apple.TextEdit-64.png"] != "png" {
		t.Error("the icons did not move")
	}

	// Once: the new folder has the library now.
	write(t, old.Data, map[string]string{sqlitestore.FileName: "a stale copy"})
	if again, err := AdoptOldName(context.Background(), old, now); err != nil || len(again.Moved) != 0 {
		t.Errorf("a second start moved %v, %v", again.Moved, err)
	}
}

// TestAdoptOldNameNotOurs: old folders without VoxtMini's library (Voxt 2's
// alone) are never touched.
func TestAdoptOldNameNotOurs(t *testing.T) {
	old, now := library(t)
	write(t, old.Data, map[string]string{"Voxt2.sqlite": "x", "updater.json": "{}"})
	write(t, old.Logs, map[string]string{"helper.log": "theirs"})
	got, err := AdoptOldName(context.Background(), old, now)
	if err != nil || len(got.Moved) != 0 {
		t.Fatalf("moved %v, %v", got.Moved, err)
	}
	if len(read(t, old.Data)) != 2 || len(read(t, old.Logs)) != 1 {
		t.Error("the old folders changed")
	}
}

// TestAdoptOldNameMovedData: the data lives elsewhere (Settings →
// Storage): location.json moves, its previous folder and the pending
// erase follow the new name, and that library's paths into the old
// default folder are updated.
func TestAdoptOldNameMovedData(t *testing.T) {
	old, now := library(t)
	ext := t.TempDir()
	putDoc(t, ext, filepath.Join(old.Data, "audio", "old.wav"))
	os.MkdirAll(old.Data, 0o755)
	WriteJSON(filepath.Join(old.Data, LocationFile), Location{Dir: ext, Previous: old.Data})
	WriteJSON(filepath.Join(old.Data, ResetFile), PendingReset{Everything: true, Also: []string{old.Logs}})
	if _, err := AdoptOldName(context.Background(), old, now); err != nil {
		t.Fatal(err)
	}
	if loc := ReadLocation(now.Data); loc.Dir != ext || loc.Previous != now.Data {
		t.Errorf("location.json = %+v", loc)
	}
	var rs PendingReset
	if !readJSON(filepath.Join(now.Data, ResetFile), &rs) || !slices.Equal(rs.Also, []string{now.Logs}) {
		t.Errorf("reset.json = %+v", rs)
	}
	if doc := getDoc(t, ext); doc != `{"audio":"`+filepath.Join(now.Data, "audio", "old.wav")+`"}` {
		t.Errorf("the library's path: %s", doc)
	}
	if _, err := os.Stat(old.Data); !os.IsNotExist(err) {
		t.Error("the emptied old data folder is still there")
	}
}

// TestAdoptOldNameCopies: another volume (rename fails): copied, checked,
// then removed.
func TestAdoptOldNameCopies(t *testing.T) {
	old, now := library(t)
	openDB(t, old.Data)
	write(t, old.Data, map[string]string{"audio/a.wav": "RIFF"})
	rename = func(string, string) error { return errors.New("cross-device link") }
	defer func() { rename = os.Rename }()
	got, err := AdoptOldName(context.Background(), old, now)
	if err != nil || len(got.Moved) != 2 {
		t.Fatalf("moved %v, %v", got.Moved, err)
	}
	if read(t, now.Data)["audio/a.wav"] != "RIFF" {
		t.Error("the recording was not copied")
	}
	if _, err := sqlitestore.CheckFile(context.Background(), filepath.Join(now.Data, sqlitestore.FileName)); err != nil {
		t.Errorf("the copied library: %v", err)
	}
	if _, err := os.Stat(old.Data); !os.IsNotExist(err) {
		t.Error("the copied folder is still there")
	}
}

// TestAdoptOldNameKeepsNew: an entry the new folder has already is left in
// the old one (an error), the others move.
func TestAdoptOldNameKeepsNew(t *testing.T) {
	old, now := library(t)
	openDB(t, old.Data)
	write(t, old.Logs, map[string]string{"voxtmini.log": "old", "helper.log": "h"})
	write(t, now.Logs, map[string]string{"voxtmini.log": "new"})
	got, err := AdoptOldName(context.Background(), old, now)
	if err == nil || !slices.Contains(got.Left, filepath.Join(old.Logs, "voxtmini.log")) {
		t.Fatalf("left %v, %v", got.Left, err)
	}
	if logs := read(t, now.Logs); logs["voxtmini.log"] != "new" || logs["helper.log"] != "h" {
		t.Errorf("logs: %v", logs)
	}
}
