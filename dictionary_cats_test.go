package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/appctx"
	"github.com/hehehai/VoxtMini/internal/store"
)

// pickOption opens the select labelled label and moves down (up for a
// negative n) n options, then picks.
func pickOption(t *testing.T, tt *ui.Tester, label string, n int) {
	t.Helper()
	click(t, tt, label)
	key := ui.KeyDown
	if n < 0 {
		key, n = ui.KeyUp, -n
	}
	for range n {
		tt.Key(0, key)
	}
	tt.Key(0, ui.KeyEnter)
	frames(tt)
}

// termOrder is the terms in the list's order (headers left out).
func termOrder(a *app) []string {
	var out []string
	for _, it := range a.dictItems() {
		if it.term != nil {
			out = append(out, it.term.Term)
		}
	}
	return out
}

func termByName(t *testing.T, a *app, name string) store.Term {
	t.Helper()
	for _, tm := range terms(t, a) {
		if tm.Term == name {
			return tm
		}
	}
	t.Fatalf("no term %q", name)
	return store.Term{}
}

// TestDictionaryCategories: a new category shows the headers; a term
// moves there in its dialog; a header closes and opens; rename; a name
// taken is refused; deleting moves the terms to Default.
func TestDictionaryCategories(t *testing.T) {
	a, tt := dictTester(t)
	seedTerms(t, a)
	a.loadDict()
	if has(tt, "Default") {
		t.Error("a header without categories")
	}
	chooseMenu(t, tt, "More actions", "New category…")
	click(t, tt, "Save")
	if !has(tt, "Type a name.") {
		t.Errorf("empty name saved: %q", firstTexts(tt))
	}
	click(t, tt, "Category name")
	tt.Type("Work")
	click(t, tt, "Save")
	if a.dv.catOpen || len(a.dv.cats) != 1 || !has(tt, "Default") || !has(tt, "Work") {
		t.Fatalf("category %+v %q", a.dv.cats, firstTexts(tt))
	}
	// The same name (or Default) again is refused.
	for _, name := range []string{"work", "default"} {
		chooseMenu(t, tt, "More actions", "New category…")
		click(t, tt, "Category name")
		tt.Type(name)
		click(t, tt, "Save")
		if !has(tt, "A category with this name exists already.") {
			t.Errorf("%s: %q", name, firstTexts(tt))
		}
		click(t, tt, "Cancel")
	}
	// Kubernetes moves to Work.
	click(t, tt, "Edit Kubernetes")
	pickOption(t, tt, "Category of the term", 1)
	click(t, tt, "Save")
	work := a.dv.cats[0]
	if got := termByName(t, a, "Kubernetes"); got.Category != work.ID {
		t.Fatalf("category %+v", got)
	}
	if got := termOrder(a); got[len(got)-1] != "Kubernetes" {
		t.Errorf("order %q", got)
	}
	// Close Work, open it again.
	click(t, tt, "Category Work")
	if slices.Contains(termOrder(a), "Kubernetes") || !has(tt, "1 term") {
		t.Errorf("closed: %q", termOrder(a))
	}
	click(t, tt, "Category Work")
	if !slices.Contains(termOrder(a), "Kubernetes") {
		t.Error("not opened")
	}
	// Rename.
	chooseMenu(t, tt, "Actions of Work", "Rename…")
	click(t, tt, "Category name")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Cloud")
	click(t, tt, "Save")
	if len(a.dv.cats) != 1 || a.dv.cats[0].Name != "Cloud" || a.dv.cats[0].ID != work.ID {
		t.Fatalf("renamed %+v", a.dv.cats)
	}
	// Delete: its term goes to Default, the headers go.
	chooseMenu(t, tt, "Actions of Cloud", "Delete…")
	if !has(tt, "Its terms move to Default.") {
		t.Errorf("confirmation %q", firstTexts(tt))
	}
	click(t, tt, "Delete")
	if got := termByName(t, a, "Kubernetes"); got.Category != "" || len(a.dv.cats) != 0 || has(tt, "Default") {
		t.Errorf("after delete %+v %+v", got, a.dv.cats)
	}
}

// TestDictionaryUsesAndScope: uses show on a row and sort the list; a
// term is scoped to an App enhancement group in its dialog and shows it.
func TestDictionaryUsesAndScope(t *testing.T) {
	a, tt := dictTester(t)
	seedTerms(t, a)
	d := a.lib.store.Dictionary()
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	d.RecordUses(context.Background(), []string{"t3", "t5"}, at)
	d.RecordUses(context.Background(), []string{"t3"}, at.Add(time.Hour))
	a.loadDict()
	if !has(tt, "2 uses") || !has(tt, "1 use") {
		t.Errorf("uses %q", firstTexts(tt))
	}
	if got := termOrder(a); !slices.Equal(got, []string{"DeepSeek", "Kubernetes", "MyGo", "VoxtMini", "豆包"}) {
		t.Errorf("A to Z %q", got)
	}
	pickOption(t, tt, "Sort terms", 1)
	if got := termOrder(a); !slices.Equal(got, []string{"豆包", "DeepSeek", "Kubernetes", "MyGo", "VoxtMini"}) {
		t.Errorf("most used %q", got)
	}
	// No groups: no scope in the dialog.
	click(t, tt, "Edit VoxtMini")
	if has(tt, "Used in") {
		t.Error("scope without groups")
	}
	click(t, tt, "Cancel")
	a.setRules(appctx.Rules{Groups: []appctx.Group{{ID: "g1", Name: "Chat", Enabled: true}}})
	click(t, tt, "Edit VoxtMini")
	pickOption(t, tt, "Where the term is used", 1)
	click(t, tt, "Save")
	got := termByName(t, a, "VoxtMini")
	if got.Group != "g1" || got.Uses != 0 || !has(tt, "Chat") {
		t.Errorf("scoped %+v %q", got, firstTexts(tt))
	}
	// Back to every app.
	click(t, tt, "Edit VoxtMini")
	pickOption(t, tt, "Where the term is used", -1)
	click(t, tt, "Save")
	if got := termByName(t, a, "VoxtMini"); got.Group != "" {
		t.Errorf("unscoped %+v", got)
	}
}

// TestDictionaryImportCategories: an import makes the categories it
// names and an export writes them.
func TestDictionaryImportCategories(t *testing.T) {
	a, tt := dictTester(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "terms.txt")
	os.WriteFile(in, []byte("VoxtMini = vox tunew\n[Work]\nKubernetes = k8s\n[Fun]\nPixcall\n"), 0o644)
	saved, savedOut := chooseImportFile, chooseExportFile
	t.Cleanup(func() { chooseImportFile, chooseExportFile = saved, savedOut })
	chooseImportFile = func(a *app, then func(string)) { then(in) }
	a.importTerms()
	frames(tt)
	if len(a.dv.cats) != 2 || a.dv.cats[0].Name != "Fun" || a.dv.cats[1].Name != "Work" {
		t.Fatalf("categories %+v", a.dv.cats)
	}
	if got := termByName(t, a, "Kubernetes"); got.Category != a.dv.cats[1].ID {
		t.Errorf("Kubernetes %+v", got)
	}
	out := filepath.Join(dir, "out.csv")
	chooseExportFile = func(a *app, name string, then func(string)) { then(out) }
	a.exportTerms("csv")
	data, _ := os.ReadFile(out)
	if !strings.Contains(string(data), "Kubernetes,k8s,,true,Work") || !strings.Contains(string(data), "VoxtMini,vox tunew,,true,\n") {
		t.Errorf("export %q", data)
	}
}

// TestHistoryCorrections: an entry's details mark what the dictionary
// changed and list it.
func TestHistoryCorrections(t *testing.T) {
	a, _, tt := newTestApp(t)
	tt.SetSize(windowWidth, 1200)
	e := dictionaryEntry()
	if err := a.lib.store.History().Put(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	a.navigate("/history")
	a.loadHistory()
	a.hist.sel = e.ID
	if !has(tt, "vox tunew → VoxtMini, 逗包 → 豆包") {
		t.Errorf("no corrections %q", firstTexts(tt))
	}
	spans := markSpans(e.Result, []string{"VoxtMini", "豆包"}, ui.Span{Weight: 600})
	var marked []string
	for _, s := range spans {
		if s.Weight == 600 {
			marked = append(marked, s.Text)
		}
	}
	if !slices.Equal(marked, []string{"VoxtMini", "豆包"}) || len(spans) != 5 {
		t.Errorf("spans %+v", spans)
	}
	if spans := markSpans("plain", []string{"x", ""}, ui.Span{}); len(spans) != 1 || spans[0].Text != "plain" {
		t.Errorf("unmarked %+v", spans)
	}
}

// dictionaryEntry is a transcription the dictionary corrected.
func dictionaryEntry() store.Entry {
	return store.Entry{
		ID: "01HIST0000000000000000DIC1", Kind: store.KindTranscribe, CreatedAt: time.Now().Add(-time.Minute), Status: store.StatusDone,
		AppBundle: "com.apple.Notes", AppName: "Notes",
		RawText:     "send the vox tunew build to the 逗包 team",
		Result:      "Send the VoxtMini build to the 豆包 team.",
		Delivery:    "insert",
		ASRProvider: "asr.fast", ASRModel: "asr.fast", LLMProvider: "llm.fast", LLMModel: "llm.fast",
		AudioMS: 3200, ASRMS: 410, LLMMS: 620, TotalMS: 4400,
		Corrections: []store.Correction{{From: "vox tunew", To: "VoxtMini"}, {From: "逗包", To: "豆包"}},
	}
}
