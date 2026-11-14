package main

// The Dictionary page's term dialog, and saving and deleting a term
// (dictionary_page.go).

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/dict"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/store"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// termEdit is the term dialog's fields: a new term when id is "".
type termEdit struct {
	id              string
	term, aliases   string
	note            string
	enabled         bool
	err             string
	saving          bool
	createdAt       time.Time
	original, title string
	// category and group are the term's ids ("": the default category,
	// every app).
	category, group string
}

// openTerm opens the term dialog on term (a new one without an id).
func (a *app) openTerm(term store.Term) {
	d := &a.dv
	d.editOpen = true
	d.edit = termEdit{id: term.ID, term: term.Term, aliases: strings.Join(term.Aliases, ", "), note: term.Note,
		enabled: term.Enabled, createdAt: term.CreatedAt, original: term.Term, category: term.Category, group: term.Group}
}

// choice is a select's options and the ids behind them: labels made
// unique.
type choice struct {
	labels []string
	ids    []string
}

func (ch *choice) add(id, label string) {
	l := label
	for n := 2; slices.Contains(ch.labels, l); n++ {
		l = fmt.Sprintf("%s (%d)", label, n)
	}
	ch.labels, ch.ids = append(ch.labels, l), append(ch.ids, id)
}

// pick shows a select of ch bound to the id *id (an unknown id shows the
// first option).
func (ch *choice) pick(c *ui.Context, key, label string, id *string) {
	t := c.Theme()
	i := max(slices.Index(ch.ids, *id), 0)
	sel := ch.labels[i]
	if ui.Select(c.Key(key), &sel, ch.labels).Label(label).Width(px(t, 260)).Changed() {
		*id = ch.ids[max(slices.Index(ch.labels, sel), 0)]
	}
}

func (a *app) termDialog(c *ui.Context) {
	t := c.Theme()
	d := &a.dv
	if !d.editOpen {
		return
	}
	e := &d.edit
	title := i18n.T("Add term")
	if e.id != "" {
		title = i18n.T("Edit term")
	}
	dialogFrame(c, "term-dlg", &d.editOpen, title, "", 460, func() {
		fieldInfo(c, i18n.T("Term"), i18n.T("As it must be written: VoxtMini, Kubernetes, 豆包."), func() {
			in := ui.TextInput(c.Key("term-term"), &e.term).Label(i18n.T("Term as written")).Placeholder("VoxtMini").AutoFocus()
			if in.Changed() {
				e.err = ""
			}
			if in.Submitted() {
				a.submitTerm()
			}
		})
		fieldInfo(c, i18n.T("Aliases"), i18n.T("What speech recognition writes instead, separated by commas: Vox Tunew, 沃克斯牛. VoxtMini puts the term in their place, also without a language model."), func() {
			ui.TextInput(c.Key("term-aliases"), &e.aliases).Label(i18n.T("Aliases of the term")).Placeholder(i18n.T("Vox Tunew, 沃克斯牛"))
		})
		fieldInfo(c, i18n.T("Note"), i18n.T("For you only: it is sent nowhere."), func() {
			ui.TextArea(c.Key("term-note"), &e.note).Label(i18n.T("Note on the term")).MinHeight(px(t, 56))
		})
		if len(d.cats) > 0 {
			var ch choice
			ch.add("", i18n.T("Default"))
			for _, cat := range d.cats {
				ch.add(cat.ID, cat.Name)
			}
			fieldInfo(c, i18n.T("Category"), "", func() { ch.pick(c, "term-cat", i18n.T("Category of the term"), &e.category) })
		}
		if groups := a.aeRules.Groups; len(groups) > 0 {
			var ch choice
			ch.add("", i18n.T("All apps"))
			for _, g := range groups {
				ch.add(g.ID, g.Name)
			}
			fieldInfo(c, i18n.T("Used in"), i18n.T("A term for one App enhancement group is used only when that group matches the app you speak in."), func() {
				ch.pick(c, "term-group", i18n.T("Where the term is used"), &e.group)
			})
		}
		ui.Checkbox(c.Key("term-enabled"), &e.enabled, i18n.T("Use this term"))
		if e.err != "" {
			warnLine(c, e.err)
		}
	}, func() {
		if e.id != "" && ghostButton(c.Key("term-delete"), i18n.T("Delete")).Clicked() {
			d.editOpen = false
			a.deleteTerm(store.Term{ID: e.id, Term: e.original})
		}
		ui.Spacer(c)
		if ui.Button(c.Key("term-cancel"), i18n.T("Cancel")).Clicked() {
			d.editOpen = false
		}
		if primaryButton(c.Key("term-save"), i18n.T("Save")).Disabled(e.saving).Clicked() {
			a.submitTerm()
		}
	})
}

// submitTerm checks and saves the dialog's term.
func (a *app) submitTerm() {
	e := &a.dv.edit
	term := dict.Clean(store.Term{ID: e.id, Term: e.term, Aliases: dict.SplitAliases(e.aliases), Note: e.note, Enabled: e.enabled, CreatedAt: e.createdAt,
		Category: e.category, Group: e.group})
	if err := dict.Check(term); err != nil {
		e.err = uitext.TermErrorText(err)
		return
	}
	e.saving = true
	a.saveTerm(term, true)
}

// saveTerm writes a term (a new id for a new one); fromDialog closes the
// dialog when it worked, or shows why not.
func (a *app) saveTerm(term store.Term, fromDialog bool) {
	if term.ID == "" {
		term.ID = core.NewID(time.Now())
	}
	if term.CreatedAt.IsZero() {
		term.CreatedAt = time.Now()
	}
	term.UpdatedAt = time.Now()
	repo := a.lib.store.Dictionary()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := repo.Put(ctx, term)
		a.post(func() {
			if fromDialog {
				a.dv.edit.saving = false
				if err != nil {
					a.dv.edit.err = uitext.TermErrorText(err)
					return
				}
				a.dv.editOpen = false
			} else if err != nil {
				a.toast(uitext.TermErrorText(err))
			}
			a.loadDict()
		})
	})
}

// deleteTerm deletes a term.
func (a *app) deleteTerm(term store.Term) {
	repo := a.lib.store.Dictionary()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := repo.Delete(ctx, term.ID)
		a.post(func() {
			if err != nil && !errors.Is(err, core.ErrNotFound) {
				a.toast(i18n.T("Could not delete the term: %v", err))
				return
			}
			a.toast(i18n.T("Deleted “%s”", term.Term))
			a.loadDict()
		})
	})
}
