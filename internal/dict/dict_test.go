package dict

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/store"
)

var t0 = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

func TestCleanAndCheck(t *testing.T) {
	got := Clean(store.Term{Term: "  Vox\ntunew ", Aliases: []string{" vox tunew", "VOX TUNEW", "", "Vox tunew", "voxtnew", "vox  tuna "}, Note: " n "})
	want := store.Term{Term: "Vox tunew", Aliases: []string{"voxtnew", "vox tuna"}, Note: "n"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Clean = %+v", got)
	}
	many := store.Term{Term: "x"}
	for i := range 20 {
		many.Aliases = append(many.Aliases, strings.Repeat("a", i+1))
	}
	if n := len(Clean(many).Aliases); n != MaxAliases {
		t.Errorf("%d aliases", n)
	}
	for in, want := range map[string]error{"VoxtMini": nil, "": ErrEmpty, "  ": ErrEmpty, "a\nb": ErrNewline, strings.Repeat("长", 81): ErrLong, strings.Repeat("长", 80): nil} {
		if err := Check(store.Term{Term: in}); !errors.Is(err, want) && err != want {
			t.Errorf("Check(%q) = %v", in, err)
		}
	}
	if got := SplitAliases("vox tunew, 沃克斯牛，沃克斯 、 x; y\n z,,"); !reflect.DeepEqual(got, []string{"vox tunew", "沃克斯牛", "沃克斯", "x", "y", "z"}) {
		t.Errorf("SplitAliases = %q", got)
	}
}

func TestHints(t *testing.T) {
	terms := []store.Term{
		{Term: "VoxtMini", Enabled: true, UpdatedAt: t0},
		{Term: "Kubernetes", Enabled: true, UpdatedAt: t0.Add(2 * time.Hour)},
		{Term: "Off", Enabled: false, UpdatedAt: t0.Add(3 * time.Hour)},
		{Term: "voxtmini", Enabled: true, UpdatedAt: t0.Add(-time.Hour)},
		{Term: "  火山 引擎 ", Aliases: []string{"火山引擎"}, Enabled: true, UpdatedAt: t0.Add(time.Hour)},
		{Term: " ", Enabled: true},
	}
	if got := Hints(terms, ""); !reflect.DeepEqual(got, []string{"Kubernetes", "火山 引擎", "VoxtMini"}) {
		t.Errorf("Hints = %q", got)
	}
	if Hints(nil, "") != nil {
		t.Error("hints of nothing")
	}
	// The most used first, then the last used; the session's group's
	// terms before all.
	terms[0].Uses = 3
	terms[4].Uses, terms[4].LastUsed = 1, t0
	terms[3].Uses, terms[3].LastUsed = 1, t0.Add(time.Hour) // a duplicate spelling of VoxtMini
	if got := Hints(terms, ""); !reflect.DeepEqual(got, []string{"VoxtMini", "火山 引擎", "Kubernetes"}) {
		t.Errorf("by use = %q", got)
	}
	terms[1].Group = "g1"
	if got := Hints(terms, "g1"); !reflect.DeepEqual(got, []string{"Kubernetes", "VoxtMini", "火山 引擎"}) {
		t.Errorf("group first = %q", got)
	}
}

func TestScoped(t *testing.T) {
	terms := []store.Term{{ID: "all"}, {ID: "g1", Group: "g1"}, {ID: "g2", Group: "g2"}, {ID: "gone", Group: "g9"}}
	exists := func(id string) bool { return id == "g1" || id == "g2" }
	ids := func(ts []store.Term) string {
		var out []string
		for _, t := range ts {
			out = append(out, t.ID)
		}
		return strings.Join(out, ",")
	}
	for group, want := range map[string]string{"": "all,gone", "g1": "all,g1,gone", "g2": "all,g2,gone"} {
		if got := ids(Scoped(terms, group, exists)); got != want {
			t.Errorf("Scoped(%q) = %s, want %s", group, got, want)
		}
	}
	if got := ids(Scoped(terms, "", nil)); got != "all" {
		t.Errorf("without exists: %s", got)
	}
}

func TestCategorize(t *testing.T) {
	n := 0
	newID := func() string { n++; return fmt.Sprint("new", n) }
	got, added := Categorize([]store.Term{{Term: "a"}, {Term: "b", Category: "work"}, {Term: "c", Category: "Fun"}, {Term: "d", Category: "fun"}},
		[]store.Category{{ID: "w", Name: "Work"}}, newID)
	if got[0].Category != "" || got[1].Category != "w" || got[2].Category != "new1" || got[3].Category != "new1" ||
		!reflect.DeepEqual(added, []store.Category{{ID: "new1", Name: "Fun"}}) {
		t.Errorf("Categorize = %+v, %+v", got, added)
	}
}

func TestExportImport(t *testing.T) {
	terms := []store.Term{
		{Term: "VoxtMini", Aliases: []string{"vox tunew", "沃克斯牛"}, Note: "our app, \"quoted\", with a comma", Enabled: true},
		{Term: "MyGo", Enabled: false},
		{Term: "Kubernetes", Enabled: true, Category: "c1"},
		{Term: "Lost", Enabled: true, Category: "gone"},
	}
	names := func(id string) string {
		if id == "c1" {
			return "Work"
		}
		return ""
	}
	csvData := Export(terms, CSV, names)
	if !strings.HasPrefix(string(csvData), "\ufeffterm,aliases,note,enabled,category\n") || !strings.Contains(string(csvData), "\nKubernetes,,,true,Work\n") {
		t.Errorf("csv %q", csvData)
	}
	back, skipped, err := Import(csvData, CSV)
	want := slices.Clone(terms)
	want[2].Category, want[3].Category = "Work", "" // names
	if err != nil || skipped != 0 || !reflect.DeepEqual(back, want) {
		t.Errorf("csv round trip %+v %d %v", back, skipped, err)
	}
	txt := Export(terms, Text, names)
	if !strings.Contains(string(txt), "\nVoxtMini = vox tunew, 沃克斯牛\n# MyGo\nLost\n\n[Work]\nKubernetes\n") {
		t.Errorf("txt %q", txt)
	}
	back, _, err = Import(txt, Text)
	if err != nil || len(back) != 3 || back[0].Term != "VoxtMini" || len(back[0].Aliases) != 2 || back[1].Category != "" || back[2].Term != "Kubernetes" || back[2].Category != "Work" {
		t.Errorf("txt round trip %+v %v", back, err)
	}
}

func TestImportVariants(t *testing.T) {
	for _, tc := range []struct {
		name, format, data string
		want               []store.Term
		skipped            int
	}{
		{"csv without a header", CSV, "VoxtMini,vox tunew;voxtuna,note\n,,\nMyGo\n", []store.Term{
			{Term: "VoxtMini", Aliases: []string{"vox tunew", "voxtuna"}, Note: "note", Enabled: true},
			{Term: "MyGo", Enabled: true},
		}, 1},
		{"csv header in another order, Chinese names", CSV, "启用,备注,词条,别名\n否,内部,豆包,\"dou bao, 逗包\"\n", []store.Term{
			{Term: "豆包", Aliases: []string{"dou bao", "逗包"}, Note: "内部", Enabled: false},
		}, 0},
		{"csv with an unknown column", CSV, "Term,Weight\nVoxtMini,5\n", []store.Term{{Term: "VoxtMini", Enabled: true}}, 0},
		{"text with tabs and comments", Text, "# list\n\nKubernetes\tk8s, kube\nDeepSeek = deep seek\n// skip\r\nGo\n", []store.Term{
			{Term: "Kubernetes", Aliases: []string{"k8s", "kube"}, Enabled: true},
			{Term: "DeepSeek", Aliases: []string{"deep seek"}, Enabled: true},
			{Term: "Go", Enabled: true},
		}, 0},
		{"text with a long line", Text, strings.Repeat("x", 81) + "\nok\n", []store.Term{{Term: "ok", Enabled: true}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, skipped, err := Import([]byte(tc.data), tc.format)
			if err != nil || skipped != tc.skipped || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Import = %+v, %d, %v", got, skipped, err)
			}
		})
	}
	if _, _, err := Import([]byte{0xff, 0xfe, 0x00}, CSV); !errors.Is(err, ErrFormat) {
		t.Errorf("binary: %v", err)
	}
	if FormatOf("terms.TXT") != Text || FormatOf("terms.csv") != CSV || FormatOf("x") != CSV {
		t.Error("FormatOf")
	}
}

func TestMerge(t *testing.T) {
	have := []store.Term{
		{ID: "1", Term: "VoxtMini", Aliases: []string{"vox tunew"}, Enabled: true, CreatedAt: t0},
		{ID: "2", Term: "MyGo", Note: "kept", Enabled: false},
		{ID: "3", Term: "Same", Aliases: []string{"x"}, Enabled: true},
	}
	have[0].Uses, have[0].Group = 4, "g1"
	imported := []store.Term{
		{Term: "voxtmini", Aliases: []string{"Vox Tunew", "沃克斯牛"}, Note: "new note", Enabled: true, Category: "c1"},
		{Term: "MyGo", Note: "ignored", Enabled: true},
		{Term: "Same", Aliases: []string{"X"}},
		{Term: "New", Enabled: true},
		{Term: "new", Aliases: []string{"neu"}},
	}
	put, added, updated := Merge(have, imported)
	if added != 1 || updated != 1 {
		t.Errorf("added %d updated %d", added, updated)
	}
	want := []store.Term{
		{ID: "1", Term: "VoxtMini", Aliases: []string{"vox tunew", "沃克斯牛"}, Note: "new note", Enabled: true, CreatedAt: t0, Category: "c1", Group: "g1", Uses: 4},
		{Term: "New", Aliases: []string{"neu"}, Enabled: true},
	}
	if !reflect.DeepEqual(put, want) {
		t.Errorf("put %+v", put)
	}
}
