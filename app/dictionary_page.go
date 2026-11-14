package main

// The Dictionary page (/dictionary): the user's terms, kept by hand.
// Each term has the spelling it must have, aliases (what speech
// recognition tends to hear instead), a note, a switch, a category and,
// optionally, the App enhancement group it is for. Terms go to the
// speech model as hints (dict.Hints, the most used first) and to the
// language model as a glossary (compose.Glossary); the switches at the
// top say where. Search, sort (A to Z or most used), add, edit, delete,
// categories (dictionary_cats.go), and import or export CSV and plain
// text (dictionary_io.go, internal/dict).
//
// The page keeps the terms of its search and the categories in memory,
// read in the background when it shows and after each change; sessions
// read the store themselves, so a change applies to the next one.

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/dict"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

// dictionaryState is the Dictionary page's view state.
type dictionaryState struct {
	dv dictView
}

type dictView struct {
	query string
	// list holds the terms read for listFor (the search), cats the
	// categories; seq numbers the reads: only the last one lands.
	list    []store.Term
	cats    []store.Category
	listFor string
	read    bool
	seq     int
	loading bool
	err     string
	total   int // terms in the dictionary (the last read without a search)
	listUI  ui.ListState
	// byUse sorts each category's terms by use (the sort menu's "Most
	// used"); collapsed are the categories closed (by id, "" the
	// default one). Both for this run only.
	byUse     bool
	collapsed map[string]bool
	// The term dialog.
	editOpen bool
	edit     termEdit
	// The category dialogs (dictionary_cats.go).
	catOpen bool
	cat     catEdit
	catDel  bool
	delCat  store.Category
	// importing: an import runs.
	importing bool
}

// Test seams: the file dialogs of Import and Export.
var (
	chooseImportFile = func(a *app, then func(string)) {
		go func() {
			files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: i18n.T("Import terms"),
				Filters: []mygo.FileFilter{{Name: i18n.T("Dictionary (CSV or text)"), Extensions: []string{"csv", "txt"}}}})
			if err != nil || len(files) == 0 {
				return
			}
			a.post(func() { then(files[0]) })
		}()
	}
	chooseExportFile = func(a *app, name string, then func(string)) {
		go func() {
			dst, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: i18n.T("Export terms"), DefaultPath: name,
				Filters: []mygo.FileFilter{{Name: strings.ToUpper(filepath.Ext(name)[1:]), Extensions: []string{filepath.Ext(name)[1:]}}}})
			if err != nil || dst == "" {
				return
			}
			a.post(func() { then(dst) })
		}()
	}
)

// dictShown runs when the router goes to the page (pageShown).
func (a *app) dictShown() { a.loadDict() }

// loadDict reads the terms of the search and the categories in the
// background.
func (a *app) loadDict() {
	d := &a.dv
	d.seq++
	d.loading = true
	seq, q := d.seq, strings.TrimSpace(d.query)
	repo := a.lib.store.Dictionary()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		list, err := repo.List(ctx, q)
		var cats []store.Category
		if err == nil {
			cats, err = repo.Categories(ctx)
		}
		a.post(func() {
			d := &a.dv
			if seq != d.seq {
				return
			}
			d.loading, d.read, d.listFor = false, true, q
			if err != nil {
				d.err = err.Error()
				return
			}
			d.list, d.cats, d.err = list, cats, ""
			if q == "" {
				d.total = len(list)
			}
		})
	})
}

func (a *app) dictionaryPage(c *ui.Context) {
	t := c.Theme()
	d := &a.dv
	if !d.read && !d.loading {
		a.loadDict()
	}
	pageHeaderTip(c, i18n.T("Dictionary"), i18n.T("Recognition and clean-up write these as spelled here."), func() {
		if d.total > 0 {
			caption(c, i18n.N(d.total, "%d term", "%d terms", d.total)).SingleLine()
		}
		on := &a.prefs.Dictionary.Enabled
		state := i18n.T("Off")
		if *on {
			state = i18n.T("On")
		}
		caption(c, state).SingleLine()
		if ui.Switch(c.Key("dict-on"), on).Label(i18n.T("Use the dictionary")).Tooltip(i18n.T("Use the dictionary")).Changed() {
			v := *on
			a.updateSettings(func(s *settings.Settings) { s.Dictionary.Enabled = v })
		}
		moreMenu(c.Key("dict-more"), "", func(m *ui.Menu) {
			if m.Item(i18n.T("New category")).Chosen() {
				a.openCategory(store.Category{})
			}
			if m.Item(i18n.T("Import")).Chosen() {
				a.importTerms()
			}
			if m.Item(i18n.T("Export as CSV")).Chosen() {
				a.exportTerms(dict.CSV)
			}
			if m.Item(i18n.T("Export as text")).Chosen() {
				a.exportTerms(dict.Text)
			}
		})
		if primaryIconButton(c.Key("dict-add"), icons.Add, i18n.T("Add term")).Clicked() {
			a.openTerm(store.Term{Enabled: true})
		}
	})
	ui.Column(c).Padding(px(t, sp1), px(t, sp6), px(t, sp2)).Gap(px(t, sp2)).Shrink(0).Children(func() {
		ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Wrap().Children(func() {
			if searchField(c, "dict-search", &d.query, i18n.T("Search terms"), px(t, 320)).Label(i18n.T("Search the dictionary")).Changed() {
				a.loadDict()
			}
			a.dictSort(c)
			a.dictSwitches(c)
		})
		if !a.prefs.Dictionary.Enabled {
			notice(c, "dict-off", noticeInfo, i18n.T("The dictionary is off."), "", nil, nil)
		}
		if d.err != "" {
			notice(c, "dict-err", noticeError, i18n.T("Could not read the dictionary."), d.err, func() {
				if ghostButton(c.Key("dict-retry"), i18n.T("Try again")).Clicked() {
					a.loadDict()
				}
			}, nil)
		}
	})
	items := a.dictItems()
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		switch {
		case !d.read:
		case len(d.list) == 0 && len(d.cats) == 0:
			a.dictEmpty(c)
		default:
			ui.List(c.Key("dict-list"), &d.listUI, len(items)).Grow(1).MinHeight(0).Gap(px(t, 2)).Padding(0, 0, px(t, sp4)).
				ItemKey(func(i int) any { return items[i].key() }).Rows(func(r ui.ListRow) {
				it := items[r.Index]
				if it.term != nil {
					a.termRow(c, *it.term)
				} else {
					a.catHeader(c, it)
				}
			})
		}
	})
	a.termDialog(c)
	a.categoryDialogs(c) // dictionary_cats.go
}

// dictSort is the sort menu: A to Z, or the most used first.
func (a *app) dictSort(c *ui.Context) {
	t := c.Theme()
	d := &a.dv
	az, used := i18n.T("A to Z"), i18n.T("Most used")
	sel := az
	if d.byUse {
		sel = used
	}
	if ui.Select(c.Key("dict-sort"), &sel, []string{az, used}).Label(i18n.T("Sort terms")).Width(px(t, 140)).Changed() {
		d.byUse = sel == used
	}
}

// dictSwitches say where the terms go.
func (a *app) dictSwitches(c *ui.Context) {
	t := c.Theme()
	off := !a.prefs.Dictionary.Enabled
	ui.Row(c).Gap(px(t, sp3)).AlignItems(ui.Center).Wrap().Children(func() {
		hints := &a.prefs.Dictionary.ASRHints
		if ui.Checkbox(c.Key("dict-hints"), hints, i18n.T("Speech hints")).Disabled(off).
			Tooltip(i18n.T("Sent to the speech model as words to expect. It keeps a limited number: the most used terms go first.")).Changed() {
			v := *hints
			a.updateSettings(func(s *settings.Settings) { s.Dictionary.ASRHints = v })
		}
		gl := &a.prefs.Dictionary.Glossary
		if ui.Checkbox(c.Key("dict-glossary"), gl, i18n.T("Glossary for the language model")).Disabled(off).
			Tooltip(i18n.T("Clean-up writes the terms as spelled here, also when recognition heard them a little differently. A dictionary of up to 30 terms goes in whole; a larger one only with the terms found in what you said.")).Changed() {
			v := *gl
			a.updateSettings(func(s *settings.Settings) { s.Dictionary.Glossary = v })
		}
	})
}

// dictEmpty is the list's empty state.
func (a *app) dictEmpty(c *ui.Context) {
	d := &a.dv
	if q := strings.TrimSpace(d.listFor); q != "" {
		emptyState(c, icons.Magnifer, i18n.T("No term matches “%s”", q), i18n.T("The search looks in terms, aliases and notes."), func() {
			if ghostButton(c.Key("dict-clear-search"), i18n.T("Clear the search")).Clicked() {
				d.query = ""
				a.loadDict()
			}
		})
		return
	}
	emptyState(c, dictionaryIcon, i18n.T("Names and terms you use"),
		i18n.T("Names, products and jargon you say: recognised and written as you spell them here."), func() {
			if ghostIconButton(c.Key("dict-empty-add"), icons.Add, i18n.T("Add term")).Clicked() {
				a.openTerm(store.Term{Enabled: true})
			}
			if ghostIconButton(c.Key("dict-empty-import"), icons.Import, i18n.T("Import")).Label(i18n.T("Import terms")).Clicked() {
				a.importTerms()
			}
		})
}

// dictItem is a row of the list: a category's header (term nil) or a
// term.
type dictItem struct {
	term  *store.Term
	cat   store.Category // the header's; ID "" for the default one
	count int            // the header's terms (of the search)
}

func (it dictItem) key() string {
	if it.term != nil {
		return it.term.ID
	}
	return "cat:" + it.cat.ID
}

// dictItems are the list's rows: the terms in their categories (the
// default one first, then A to Z), each category under its header unless
// it is the only one; a closed category shows its header only (not while
// searching), and a search shows the categories it found terms in.
func (a *app) dictItems() []dictItem {
	d := &a.dv
	known := map[string]bool{}
	for _, c := range d.cats {
		known[c.ID] = true
	}
	byCat := map[string][]store.Term{}
	for _, t := range d.list {
		id := t.Category
		if !known[id] {
			id = ""
		}
		byCat[id] = append(byCat[id], t)
	}
	cats := append([]store.Category{{Name: i18n.T("Default")}}, d.cats...)
	searching := d.listFor != ""
	var out []dictItem
	for _, cat := range cats {
		terms := byCat[cat.ID]
		if d.byUse {
			slices.SortStableFunc(terms, byUseOrder)
		}
		if len(d.cats) > 0 {
			if searching && len(terms) == 0 {
				continue
			}
			out = append(out, dictItem{cat: cat, count: len(terms)})
			if d.collapsed[cat.ID] && !searching {
				continue
			}
		}
		for i := range terms {
			out = append(out, dictItem{term: &terms[i]})
		}
	}
	return out
}

// byUseOrder: the most used first, then the last used, then A to Z.
func byUseOrder(a, b store.Term) int {
	switch {
	case a.Uses != b.Uses:
		return b.Uses - a.Uses
	case !a.LastUsed.Equal(b.LastUsed):
		return b.LastUsed.Compare(a.LastUsed)
	}
	return strings.Compare(strings.ToLower(a.Term), strings.ToLower(b.Term))
}

// groupName is the name of an App enhancement group, "" when there is
// none with that id.
func (a *app) groupName(id string) string {
	for _, g := range a.aeRules.Groups {
		if g.ID == id && id != "" {
			return g.Name
		}
	}
	return ""
}

// termRow is a term in the list: its switch, the term, its aliases and
// note, its group and uses, Edit and Delete.
func (a *app) termRow(c *ui.Context, term store.Term) {
	t := c.Theme()
	k := tok(t)
	row := ui.Row(c.Key("term-"+term.ID)).Padding(px(t, 6), px(t, sp2)).Margin(0, px(t, sp4)).Radius(px(t, radiusControl)).
		Gap(px(t, sp2)).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(k.Hover)
	}
	row.Children(func() {
		on := term.Enabled // derived: committed in OnChange
		ui.Switch(c.Key("on"), &on).Label(i18n.T("Use %s", term.Term)).OnChange(func() {
			tm := term
			tm.Enabled = on
			a.saveTerm(tm, false)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(px(t, 2)).Children(func() {
			ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).Children(func() {
				tx := ui.Text(c, term.Term).FontWeight(600).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				if !term.Enabled {
					tx.TextColor(k.Text3)
				}
				if g := a.groupName(term.Group); g != "" {
					tagChip(c.Key("group"), g).Tooltip(i18n.T("Only in the App enhancement group “%s”", g))
				}
				if len(term.Aliases) > 0 {
					truncText(c, i18n.T("heard as %s", strings.Join(term.Aliases, i18n.T(", ")))).FontSize(px(t, fsCaption)).TextColor(k.Text2).Grow(1)
				}
			})
			if term.Note != "" {
				truncText(c, term.Note).FontSize(px(t, fsCaption)).TextColor(k.Text3)
			}
		})
		if term.Uses > 0 {
			caption(c.Key("uses"), i18n.N(term.Uses, "%d use", "%d uses", term.Uses)).SingleLine().Shrink(0).
				Tooltip(i18n.T("Last used %s", i18n.DateTime(term.LastUsed)))
		}
		if iconButton(c.Key("edit"), icons.Pen2, i18n.T("Edit %s", term.Term)).Clicked() {
			a.openTerm(term)
		}
		if iconButton(c.Key("delete"), icons.TrashBinMinimalistic, i18n.T("Delete %s", term.Term)).Clicked() {
			a.deleteTerm(term)
		}
	})
}
