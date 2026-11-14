package main

// The Dictionary page's import and export (dictionary_page.go,
// internal/dict): CSV or plain text, with the terms' categories.

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/dict"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/store"
)

// importTerms reads a CSV or text file and merges its terms in.
func (a *app) importTerms() {
	if a.dv.importing {
		return
	}
	chooseImportFile(a, func(path string) { a.importFile(path) })
}

// errImportSize: the file is larger than dict.MaxImportBytes.
var errImportSize = errors.New("larger than the import limit")

// readImport reads a file to import, refusing one over
// dict.MaxImportBytes before reading it (a big file picked by mistake
// would sit in memory whole).
func readImport(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > dict.MaxImportBytes {
		return nil, errImportSize
	}
	data, err := io.ReadAll(io.LimitReader(f, dict.MaxImportBytes+1))
	if err == nil && len(data) > dict.MaxImportBytes {
		return nil, errImportSize
	}
	return data, err
}

// importFile merges the terms of the file at path into the dictionary.
func (a *app) importFile(path string) {
	a.dv.importing = true
	repo := a.lib.store.Dictionary()
	a.async(func() {
		msg := func() string {
			data, err := readImport(path)
			if errors.Is(err, errImportSize) {
				return i18n.T("Could not import %s: it is larger than 5 MB.", filepath.Base(path))
			}
			if err != nil {
				return i18n.T("Could not import: %v", err)
			}
			terms, skipped, err := dict.Import(data, dict.FormatOf(path))
			if err != nil {
				return i18n.T("Could not import %s: it is not a CSV or text list of terms.", filepath.Base(path))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			have, err := repo.List(ctx, "")
			if err != nil {
				return i18n.T("Could not import: %v", err)
			}
			cats, err := repo.Categories(ctx)
			if err != nil {
				return i18n.T("Could not import: %v", err)
			}
			now := time.Now()
			terms, newCats := dict.Categorize(terms, cats, func() string { return core.NewID(now) })
			for _, c := range newCats {
				c.CreatedAt = now
				if err := repo.PutCategory(ctx, c); err != nil {
					return i18n.T("Could not import: %v", err)
				}
			}
			put, added, updated := dict.Merge(have, terms)
			for i, tm := range put {
				if tm.ID == "" {
					tm.ID, tm.CreatedAt = core.NewID(now), now.Add(time.Duration(i))
				}
				tm.UpdatedAt = now
				if err := repo.Put(ctx, tm); err != nil {
					return i18n.T("Could not import: %v", err)
				}
			}
			if skipped > 0 {
				return i18n.T("Imported: %d new, %d updated, %d rows skipped.", added, updated, skipped)
			}
			return i18n.T("Imported: %d new, %d updated.", added, updated)
		}()
		a.post(func() {
			a.dv.importing = false
			a.toast(msg)
			a.loadDict()
		})
	})
}

// exportTerms writes every term to a file the user picks.
func (a *app) exportTerms(format string) {
	name := "voxtmini-dictionary." + format
	chooseExportFile(a, name, func(dst string) { a.exportFile(dst, format) })
}

func (a *app) exportFile(dst, format string) {
	repo := a.lib.store.Dictionary()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		terms, err := repo.List(ctx, "")
		var cats []store.Category
		if err == nil {
			cats, err = repo.Categories(ctx)
		}
		if err == nil {
			err = os.WriteFile(dst, dict.Export(terms, format, categoryNames(cats)), 0o644)
		}
		a.post(func() {
			if err != nil {
				a.toast(i18n.T("Could not export: %v", err))
				return
			}
			a.toast(i18n.N(len(terms), "Exported %d term to %s", "Exported %d terms to %s", len(terms), filepath.Base(dst)))
		})
	})
}

// categoryNames names the categories by id for an export.
func categoryNames(cats []store.Category) func(string) string {
	return func(id string) string {
		for _, c := range cats {
			if c.ID == id {
				return c.Name
			}
		}
		return ""
	}
}
