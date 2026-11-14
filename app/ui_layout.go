package main

// The kit's layout (ui_kit.go): the page header, cards, collapsible
// sections, the Advanced fold, form rows, search fields, empty states.

import (
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
)

// ---- Layout ----

// pageHeader is a page's title bar, which drags the window: its title,
// then on the right what trailing builds (the page's one primary action,
// icon buttons, moreMenu).
// headerInset is the room page titles leave on their left for the window
// controls: set each frame by sidebarFrame, non-zero beside the icon rail.
var headerInset float32

func pageHeader(c *ui.Context, title string, trailing func()) { pageHeaderTip(c, title, "", trailing) }

// pageHeaderTip is pageHeader with a ⓘ after the title telling what the
// page is for (tip, "" for none): the place for a page's description,
// instead of a paragraph under the title.
func pageHeaderTip(c *ui.Context, title, tip string, trailing func()) {
	t := c.Theme()
	ui.Row(c).Height(titleBarHeight).Shrink(0).AlignItems(ui.Center).Padding(0, px(t, sp4), 0, px(t, sp6)+headerInset).Gap(px(t, sp2)).DragWindow().Children(func() {
		heading := func() ui.Element {
			return ui.Text(c, title).FontSize(px(t, fsTitle)).FontWeight(600).SingleLine().Ellipsis("…").MinWidth(0)
		}
		if tip == "" {
			heading().Grow(1)
		} else {
			ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Start).MinWidth(0).Children(func() {
				tm, im := tipMargins(t, title, px(t, fsTitle), 600)
				heading().Margin(tm, 0, 0, 0)
				infoTip(c.Key("page-tip"), tip).Shrink(0).Margin(im, 0, 0, 0)
			})
			ui.Box(c).Grow(1)
		}
		if trailing != nil {
			trailing()
		}
	})
}

// card is a flat card: the page's background inside a hairline border,
// radius 12. Give it children.
func card(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(px(t, sp4)).Gap(px(t, sp3)).Radius(px(t, radiusCard)).Border(1, tok(t).Line)
}

// fillCard is a flat card on the subtle grey fill, without a border.
func fillCard(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(px(t, sp4)).Gap(px(t, sp3)).Radius(px(t, radiusCard)).Background(tok(t).Subtle)
}

// collapsible is a section that folds: a header row (chevron, title, and
// what trailing adds on the right, as a summary) that opens and closes
// it, and fn below it while open.
func collapsible(c *ui.Context, title string, open *bool, trailing, fn func()) ui.Element {
	t := c.Theme()
	k := tok(t)
	root := ui.Column(c).Shrink(0)
	root.Children(func() {
		parts := ui.CollapsibleBase(c, open)
		tr := parts.Trigger.Gap(px(t, sp2)).AlignItems(ui.Center).Padding(px(t, 6), px(t, 4)).Radius(px(t, radiusControl)).Label(title)
		if tr.Hovered() {
			tr.Background(k.Hover)
		}
		tr.Children(func() {
			ui.Icon(c, icons.AltArrowRight.Linear).Size(px(t, 14), px(t, 14)).TextColor(k.Text2).Rotate(90 * parts.Progress())
			ui.Text(c, title).FontWeight(600).SingleLine().Ellipsis("…").MinWidth(0).Grow(1)
			if trailing != nil {
				trailing()
			}
		})
		// Apply a click now, in this build pass: callers often hand in a
		// local copied from a map and store it back after this returns, so a
		// click applied after the frame would land in a dead local and the
		// section would never open or close.
		tr.Changed()
		if p := parts.Panel(fn); p.Valid() {
			p.Gap(px(t, sp3)).Padding(px(t, sp2), px(t, 4), px(t, 4))
		}
	})
	return root
}

// advanced is the fold a page keeps its less used settings in (Inboxly's
// kit.Advanced): titled Advanced, closed until opened (*open, kept for the
// run only); while closed its header says in a quiet line what the
// settings in it are now (summary, "" for nothing), cut when short.
func advanced(c *ui.Context, open *bool, summary string, body func()) ui.Element {
	t := c.Theme()
	k := tok(t)
	title := i18n.T("Advanced")
	root := ui.Column(c).Shrink(0)
	root.Children(func() {
		parts := ui.CollapsibleBase(c, open)
		tr := parts.Trigger.Gap(px(t, sp2)).AlignItems(ui.Center).Padding(px(t, 6), px(t, 4)).Radius(px(t, radiusControl)).Label(title)
		if tr.Hovered() {
			tr.Background(k.Hover)
		}
		closed := !*open
		tr.Children(func() {
			ui.Icon(c, icons.AltArrowRight.Linear).Size(px(t, 14), px(t, 14)).TextColor(k.Text2).Rotate(90 * parts.Progress())
			// The title keeps its room; the summary is cut first.
			ui.Text(c, title).FontWeight(600).SingleLine().Shrink(0)
			if closed && summary != "" {
				truncText(c, summary).FontSize(px(t, fsCaption)).TextColor(k.Text3).Shrink(1).Grow(1)
			} else {
				ui.Box(c).Grow(1)
			}
		})
		tr.Changed() // as collapsible: the click applies in this pass
		if p := parts.Panel(body); p.Valid() {
			p.Gap(px(t, sp3)).Padding(px(t, sp2), px(t, 4), px(t, 4))
		}
	})
	return root
}

// summary joins an Advanced summary's parts with " · ", leaving out the
// empty ones.
func summary(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " · "
		}
		out += p
	}
	return out
}

// onOff is a switch in a summary: its name and its state.
func onOff(name string, on bool) string {
	v := i18n.T("Off")
	if on {
		v = i18n.T("On")
	}
	return i18n.T("%s: %s", name, v)
}

// formRow is a setting: its name, a ⓘ with what it does (info, "" for
// none), and its control on the right.
func formRow(c *ui.Context, key, title, info string, control func()) {
	t := c.Theme()
	// The control goes under the name when both do not fit on a line.
	ui.Row(c.Key(key)).Wrap().GapX(px(t, sp4)).GapY(px(t, 6)).AlignItems(ui.Center).MinHeight(px(t, 32)).Children(func() {
		ui.Row(c).Grow(1).Basis(px(t, 160)).MinWidth(px(t, 120)).Gap(px(t, 6)).AlignItems(ui.Start).Children(func() {
			labelTip(c, ui.Text(c, title).MinWidth(0), title, t.FontSize, 400, info)
		})
		ui.Row(c).Wrap().Gap(px(t, sp2)).AlignItems(ui.Center).Shrink(1).MinWidth(0).Children(control)
	})
}

// fieldInfo is a labelled control with a ⓘ beside its label.
func fieldInfo(c *ui.Context, label, info string, control func()) {
	t := c.Theme()
	ui.Column(c).Gap(px(t, 6)).Children(func() {
		ui.Row(c).Gap(px(t, 6)).AlignItems(ui.Start).Children(func() {
			labelTip(c, ui.Text(c, label).MinWidth(0), label, t.FontSize, 400, info)
		})
		control()
	})
}

// searchField is a text input with a magnifier before its text. It is
// as wide as its row allows, up to maxW (0: no limit); the input is
// returned for Changed, Label…
func searchField(c *ui.Context, key string, value *string, placeholder string, maxW float32) ui.Element {
	t := c.Theme()
	k := tok(t)
	var in ui.Element
	box := ui.Row(c.Key(key + "-box")).Grow(1).Shrink(1).Basis(0).MinWidth(px(t, 120)).AlignItems(ui.Stretch)
	if maxW > 0 {
		box.MaxWidth(maxW)
	}
	box.Children(func() {
		in = ui.TextInput(c.Key(key), value).Placeholder(placeholder).Label(placeholder).Grow(1).MinWidth(0).
			Padding(t.Space(1.5), t.Space(2.5), t.Space(1.5), px(t, 30))
		ui.Column(c).Absolute().Left(0).Top(0).Bottom(0).Width(px(t, 30)).Center().PassThrough().Children(func() {
			ui.Icon(c, icons.Magnifer.Linear).Size(px(t, 14), px(t, 14)).TextColor(k.Text3)
		})
	})
	return in
}

// emptyState is a page's or a list's empty state: a large icon, a title,
// a line of text and actions (one primary at most).
func emptyState(c *ui.Context, ic icons.Icon, title, body string, actions func()) ui.Element {
	t := c.Theme()
	k := tok(t)
	return ui.Column(c).Grow(1).Center().Gap(px(t, sp2)).Padding(px(t, sp6)).Children(func() {
		ui.Icon(c, ic.Linear).Size(px(t, 36), px(t, 36)).TextColor(k.Text3)
		ui.Text(c, title).FontSize(px(t, fsSection)).FontWeight(600).TextAlign(ui.Center)
		if body != "" {
			ui.Text(c, body).TextColor(k.Text2).TextAlign(ui.Center).MaxWidth(px(t, 420))
		}
		if actions != nil {
			ui.Row(c).Gap(px(t, sp2)).Wrap().Justify(ui.Center).Margin(px(t, sp2), 0, 0).Children(actions)
		}
	})
}
