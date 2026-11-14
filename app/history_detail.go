package main

// A history entry's details (history_page.go): the result, the original,
// what made it (services, group, sound, times), and deleting it.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
	"github.com/hehehai/VoxtMini/internal/uitext"
)

// entryModel is the model an entry used, as the details show it: its
// name (the catalogue's words, also once it is deleted), else its id; ""
// for none.
func (a *app) entryModel(kind local.Kind, id, model string) string {
	if id == "" {
		return ""
	}
	name := uitext.ModelName(id)
	if pid, remote := settings.IsRemote(id); remote {
		name = pid // a remote service no longer set up: its id
	}
	for _, m := range a.installed(kind) {
		if m.ID == id {
			name = m.Name
		}
	}
	if model != "" && model != id {
		name += " · " + model
	}
	return name
}

// historyDetail is the details panel of an entry.
func (a *app) historyDetail(c *ui.Context, e store.Entry) {
	t := c.Theme()
	k := tok(t)
	h := &a.hist
	ui.Column(c.Key("hist-detail-"+e.ID)).Padding(px(t, sp3), px(t, sp5), px(t, sp5)).Gap(px(t, sp3)).Children(func() {
		// The panel's header: its title, then Show in Finder (with a
		// recording), Delete and Close.
		ui.Row(c).Gap(px(t, sp1)).AlignItems(ui.Center).Children(func() {
			sectionHeading(c, i18n.T("Entry details")).Grow(1)
			if e.Audio != "" && iconButton(c.Key("hist-reveal"), icons.FolderOpen, i18n.T("Show the recording in Finder")).Clicked() {
				revealInFinder(filepath.Join(a.lib.store.History().AudioDir(), e.Audio))
			}
			if iconButton(c.Key("hist-delete"), icons.TrashBinMinimalistic, i18n.T("Delete the entry")).Clicked() {
				a.askDeleteEntry(e.ID)
			}
			if iconButton(c.Key("panel-close"), icons.Close, i18n.T("Close")).Clicked() {
				h.sel = ""
			}
		})
		if e.Error != "" {
			kind, text := noticeError, i18n.T("Failed: %s", uitext.FirstLine(e.Error))
			if e.Status != store.StatusFailed {
				kind, text = noticeWarn, i18n.T("Done with a problem: %s", uitext.FirstLine(e.Error))
			}
			notice(c, "hist-detail-err", kind, text, e.Error, nil, nil)
		}
		a.entryRetryButton(c, e) // history_retry.go
		if e.Audio != "" {
			a.recordingRow(c, e) // history_playback.go
		}
		if s := recordingKeptText(e); s != "" {
			caption(c, s)
		}
		// The texts there are, each with Copy; the dictionary's changes
		// marked (what was heard in the recognised text, the terms in the
		// result).
		var heard, written []string
		for _, cr := range e.Corrections {
			heard, written = append(heard, cr.From), append(written, cr.To)
		}
		textBlock := func(key, title, copyLabel, text string, marks []string) {
			text = strings.TrimSpace(text)
			if text == "" {
				return
			}
			ui.Column(c.Key(key)).Gap(px(t, sp1)).Children(func() {
				ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).MinHeight(px(t, 28)).Children(func() {
					sectionTitleText(c, title).Grow(1).MinWidth(0)
					if iconButton(c.Key(key+"-copy"), icons.Copy, copyLabel).Clicked() {
						mygoClipboard(c, text)
						a.toast(i18n.T("Copied"))
					}
				})
				fillCard(c).Padding(px(t, sp2), px(t, sp3)).Children(func() {
					if spans := markSpans(text, marks, ui.Span{Color: k.AccentInk, Background: k.AccentSoft, Radius: px(t, radiusChip)}); len(spans) > 1 {
						ui.RichText(c, spans...).Selectable().Label(text)
						return
					}
					ui.Text(c, text).Selectable()
				})
			})
		}
		textBlock("hist-result", i18n.T("Result"), i18n.T("Copy the result"), e.Result, written)
		if e.PromptText != "" {
			textBlock("hist-prompt", i18n.T("Instruction"), i18n.T("Copy the instruction"), e.PromptText, heard)
		}
		if e.SourceText != "" {
			textBlock("hist-source", i18n.T("Selected text"), i18n.T("Copy the selected text"), e.SourceText, nil)
		}
		textBlock("hist-raw", i18n.T("Recognised text"), i18n.T("Copy the recognised text"), e.RawText, heard)
		a.followUpSection(c, e) // history_followup.go

		// The facts: labels as wide as the longest, values in the rest.
		app := e.AppName
		if e.URLHost != "" {
			app += " · " + e.URLHost
		}
		details := [][2]string{{i18n.T("Time"), i18n.DateTime(e.CreatedAt)}, {i18n.T("App"), app}}
		if e.Kind == store.KindTranslate {
			details = append(details, [2]string{i18n.T("Target language"), uitext.TargetLang(e)})
		}
		details = append(details,
			[2]string{i18n.T("Delivery"), uitext.DeliveryLabel(e)},
			[2]string{i18n.T("Speech recognition"), a.entryModel(local.ASR, e.ASRProvider, e.ASRModel)},
			[2]string{i18n.T("Sound optimization"), uitext.SoundText(e)},
			[2]string{i18n.T("Language model"), a.entryModel(local.LLM, e.LLMProvider, e.LLMModel)},
			[2]string{i18n.T("App rule"), uitext.RuleText(e)})
		if len(e.Corrections) > 0 {
			details = append(details, [2]string{i18n.T("Dictionary"), correctionsText(e.Corrections)})
		}
		sections := []struct {
			title string
			rows  [][2]string
		}{
			{i18n.T("Details"), details},
			{i18n.T("Timing"), [][2]string{
				{i18n.T("Recording"), uitext.RecordingTime(e)},
				{i18n.T("Recognition"), uitext.StageTime(e.ASRMS)},
				{i18n.T("Language model"), uitext.StageTime(e.LLMMS)},
				{i18n.T("Total"), uitext.StageTime(e.TotalMS)},
			}},
		}
		sectionGap := [2]float32{sp1, sp4} // above the first heading, above the others
		ui.Grid(c.Key("hist-facts")).ColumnTracks(ui.FitContent(), ui.Fr(1)).GapX(px(t, sp3)).GapY(px(t, sp1)).Children(func() {
			for i, sec := range sections {
				sectionTitleText(c, sec.title).ColumnSpan(2).Margin(px(t, sectionGap[min(i, 1)]), 0, px(t, 2), 0)
				for _, r := range sec.rows {
					ui.Text(c, r[0]).FontSize(px(t, fsCaption)).TextColor(k.Text2)
					v := ui.Text(c.Key(fmt.Sprintf("hist-fact-%d-%s", i, r[0])), uitext.OrDash(r[1])).FontSize(px(t, fsCaption)).Selectable().MinWidth(0)
					if r[1] == "" {
						v.TextColor(k.Text3)
					}
				}
			}
		})
	})
}

// correctionsText lists what the dictionary changed: "aura link →
// VoxtMini, 逗包 → 豆包".
func correctionsText(cs []store.Correction) string {
	var parts []string
	for _, c := range cs {
		parts = append(parts, c.From+" → "+c.To)
	}
	return strings.Join(parts, i18n.T(", "))
}

// markSpans splits text into spans, the occurrences of marks (the longest
// first where they overlap) in style; one span when none occurs.
func markSpans(text string, marks []string, style ui.Span) []ui.Span {
	var out []ui.Span
	plain := 0
	for i := 0; i < len(text); {
		n := 0
		for _, m := range marks {
			if len(m) > n && strings.HasPrefix(text[i:], m) {
				n = len(m)
			}
		}
		if n == 0 {
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size
			continue
		}
		if plain < i {
			out = append(out, ui.Span{Text: text[plain:i]})
		}
		sp := style
		sp.Text = text[i : i+n]
		out = append(out, sp)
		i += n
		plain = i
	}
	if plain < len(text) || len(out) == 0 {
		out = append(out, ui.Span{Text: text[plain:]})
	}
	return out
}

// historyDeleteDialog confirms Delete (askDeleteEntry), on History and
// Home.
func (a *app) historyDeleteDialog(c *ui.Context) {
	h := &a.hist
	if !h.delAsk {
		return
	}
	dialogFrame(c, "hist-del", &h.delAsk, i18n.T("Delete this entry?"), "", 420, func() {
		ui.Text(c, i18n.T("Its texts and its recording are deleted. This cannot be undone."))
	}, func() {
		ui.Spacer(c)
		if ui.Button(c.Key("hist-del-cancel"), i18n.T("Cancel")).AutoFocus().Clicked() {
			h.delAsk = false
		}
		if recordButton(c.Key("hist-del-ok"), i18n.T("Delete")).Clicked() {
			h.delAsk = false
			a.deleteHistoryEntry(h.delID)
		}
	})
}

// deleteHistoryEntry deletes an entry and its recording, in the
// background, then tells the history's observers (History, Home, the
// menu bar).
func (a *app) deleteHistoryEntry(id string) {
	if id == "" {
		return
	}
	if a.hist.play.id == id || a.hist.ask.id == id {
		a.closeEntry()
	}
	hs := a.lib.store.History()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := hs.Delete(ctx, id)
		a.post(func() {
			if err != nil && !errors.Is(err, core.ErrNotFound) {
				a.toast(i18n.T("Could not delete the entry: %v", err))
				return
			}
			if a.hist.sel == id {
				a.hist.sel = ""
			}
			a.toast(i18n.T("Entry deleted"))
			a.historyChanged()
		})
	})
}
