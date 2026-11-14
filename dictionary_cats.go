package main

// The Dictionary page's categories (dictionary_page.go): a header per
// category that opens and closes it, New category…, Rename… and
// Delete… (its terms move to the default one). The default category is
// not stored and can be neither renamed nor deleted.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/store"
)

// catEdit is the category dialog's fields: a new category when id is "".
type catEdit struct {
	id, name string
	err      string
	saving   bool
	created  time.Time
}

// catHeader is a category's row: open or close it, its count, and for
// one of the user's, Rename and Delete.
func (a *app) catHeader(c *ui.Context, it dictItem) {
	t := c.Theme()
	k := tok(t)
	d := &a.dv
	open := !d.collapsed[it.cat.ID] || d.listFor != ""
	row := ui.Row(c.Key("cat-"+it.cat.ID)).Padding(px(t, 6), px(t, sp2)).Margin(px(t, sp2), px(t, sp4), 0).Radius(px(t, radiusControl)).
		Gap(px(t, sp2)).AlignItems(ui.Center).Label(i18n.T("Category %s", it.cat.Name))
	if row.Hovered() {
		row.Background(k.Hover)
	}
	if row.Clicked() && d.listFor == "" {
		if d.collapsed == nil {
			d.collapsed = map[string]bool{}
		}
		d.collapsed[it.cat.ID] = open
	}
	row.Children(func() {
		rot := float32(90)
		if !open {
			rot = 0
		}
		ui.Icon(c, icons.AltArrowRight.Linear).Size(px(t, 14), px(t, 14)).TextColor(k.Text2).Rotate(rot)
		ui.Text(c, it.cat.Name).FontWeight(600).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		caption(c, i18n.N(it.count, "%d term", "%d terms", it.count)).SingleLine()
		ui.Spacer(c)
		if it.cat.ID == "" {
			return
		}
		cat := it.cat
		moreMenu(c.Key("cat-more"), i18n.T("Actions of %s", cat.Name), func(m *ui.Menu) {
			if m.Item(i18n.T("Rename")).Chosen() {
				a.openCategory(cat)
			}
			if m.Item(i18n.T("Delete")).Chosen() {
				d.catDel, d.delCat = true, cat
			}
		})
	})
}

// openCategory opens the category dialog on cat (a new one without an
// id).
func (a *app) openCategory(cat store.Category) {
	d := &a.dv
	d.catOpen = true
	d.cat = catEdit{id: cat.ID, name: cat.Name, created: cat.CreatedAt}
}

// categoryDialogs are the category dialog and Delete's confirmation.
func (a *app) categoryDialogs(c *ui.Context) {
	d := &a.dv
	if d.catOpen {
		e := &d.cat
		title := i18n.T("New category")
		if e.id != "" {
			title = i18n.T("Rename category")
		}
		dialogFrame(c, "cat-dlg", &d.catOpen, title, "", 400, func() {
			in := ui.TextInput(c.Key("cat-name"), &e.name).Label(i18n.T("Category name")).Placeholder(i18n.T("Work")).AutoFocus()
			if in.Changed() {
				e.err = ""
			}
			if in.Submitted() {
				a.saveCategory()
			}
			if e.err != "" {
				warnLine(c, e.err)
			}
		}, func() {
			ui.Spacer(c)
			if ui.Button(c.Key("cat-cancel"), i18n.T("Cancel")).Clicked() {
				d.catOpen = false
			}
			if primaryButton(c.Key("cat-save"), i18n.T("Save")).Disabled(e.saving).Clicked() {
				a.saveCategory()
			}
		})
	}
	if d.catDel {
		cat := d.delCat
		n := 0
		for _, t := range d.list {
			if t.Category == cat.ID {
				n++
			}
		}
		dialogFrame(c, "cat-del", &d.catDel, i18n.T("Delete “%s”?", cat.Name), "", 400, func() {
			if n > 0 || d.listFor != "" {
				ui.Text(c, i18n.T("Its terms move to Default."))
			} else {
				ui.Text(c, i18n.T("It has no terms."))
			}
		}, func() {
			ui.Spacer(c)
			if ui.Button(c.Key("cat-del-cancel"), i18n.T("Cancel")).AutoFocus().Clicked() {
				d.catDel = false
			}
			if recordButton(c.Key("cat-del-ok"), i18n.T("Delete")).Clicked() {
				d.catDel = false
				a.deleteCategory(cat)
			}
		})
	}
}

// saveCategory checks and saves the dialog's category.
func (a *app) saveCategory() {
	e := &a.dv.cat
	name := strings.Join(strings.Fields(e.name), " ")
	switch {
	case name == "":
		e.err = i18n.T("Type a name.")
		return
	case strings.EqualFold(name, i18n.T("Default")):
		e.err = i18n.T("A category with this name exists already.")
		return
	}
	cat := store.Category{ID: e.id, Name: name, CreatedAt: e.created}
	if cat.ID == "" {
		cat.ID, cat.CreatedAt = core.NewID(time.Now()), time.Now()
	}
	e.saving = true
	repo := a.lib.store.Dictionary()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := repo.PutCategory(ctx, cat)
		a.post(func() {
			e := &a.dv.cat
			e.saving = false
			switch {
			case errors.Is(err, core.ErrExists):
				e.err = i18n.T("A category with this name exists already.")
				return
			case err != nil:
				e.err = i18n.T("Could not save the category: %v", err)
				return
			}
			a.dv.catOpen = false
			a.loadDict()
		})
	})
}

// deleteCategory deletes a category; its terms move to the default one.
func (a *app) deleteCategory(cat store.Category) {
	repo := a.lib.store.Dictionary()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := repo.DeleteCategory(ctx, cat.ID)
		a.post(func() {
			if err != nil && !errors.Is(err, core.ErrNotFound) {
				a.toast(i18n.T("Could not delete the category: %v", err))
				return
			}
			delete(a.dv.collapsed, cat.ID)
			a.toast(i18n.T("Deleted “%s”", cat.Name))
			a.loadDict()
		})
	})
}
