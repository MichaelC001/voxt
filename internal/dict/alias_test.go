package dict

import (
	"slices"
	"testing"

	"github.com/hehehai/VoxtMini/internal/store"
)

// TestReplace: aliases become their term: Latin ones as whole words
// without case, Chinese ones anywhere, the longest first; disabled
// terms, words that only contain an alias and the terms themselves stay.
func TestReplace(t *testing.T) {
	terms := []store.Term{
		{Term: "Auralink", Aliases: []string{"Or Link", "aura link", "奥拉林克", "奥拉"}, Enabled: true},
		{Term: "VoxtMini", Aliases: []string{"Vox Tunew", "Vox New", "Vox"}, Enabled: true},
		{Term: "SGLang", Aliases: []string{"S G Long"}, Enabled: true},
		{Term: "Kubernetes", Aliases: []string{"库伯"}, Enabled: false},
		{Term: "Pixcall", Aliases: []string{"pixcall", ""}, Enabled: true},
		{Term: "ActNow", Aliases: []string{"Or Link"}, Enabled: true},
	}
	for _, c := range []struct{ in, want string }{
		{"我们把 Or Link 的发布计划发给 team。", "我们把 Auralink 的发布计划发给 team。"},
		{"discuss the or  link launch", "discuss the Auralink launch"},
		{"OR LINK, then Vox Tunew.", "Auralink, then VoxtMini."},
		{"Vox New和Vox都是", "VoxtMini和VoxtMini都是"},
		{"Voxel and Voxt stay, Vox goes", "Voxel and Voxt stay, VoxtMini goes"},
		{"Or Linked and Door Link stay", "Or Linked and Door Link stay"},
		{"奥拉林克和奥拉的版本", "Auralink和Auralink的版本"},
		{"用 S G Long 部署，库伯也是", "用 SGLang 部署，库伯也是"},
		{"Auralink and VoxtMini and voxtmini", "Auralink and VoxtMini and voxtmini"},
		{"pixcall", "pixcall"},
		{"", ""},
	} {
		if got := replace(c.in, terms); got != c.want {
			t.Errorf("%q: %q, want %q", c.in, got, c.want)
		}
		if again := replace(replace(c.in, terms), terms); again != c.want {
			t.Errorf("%q twice: %q", c.in, again)
		}
	}
	if got := replace("Or Link", nil); got != "Or Link" {
		t.Errorf("no terms: %q", got)
	}
}

// TestReplaceChanges: what Replace changed, each pair once, in order.
func TestReplaceChanges(t *testing.T) {
	terms := []store.Term{
		{ID: "a", Term: "Auralink", Aliases: []string{"Or Link", "奥拉林克"}, Enabled: true},
		{ID: "v", Term: "VoxtMini", Aliases: []string{"Vox Tunew"}, Enabled: true},
	}
	got, changes := ReplaceChanges("奥拉林克 and or link, Or Link and vox tunew, Auralink", terms)
	want := []Change{{"a", "奥拉林克", "Auralink"}, {"a", "or link", "Auralink"}, {"a", "Or Link", "Auralink"}, {"v", "vox tunew", "VoxtMini"}}
	if got != "Auralink and Auralink, Auralink and VoxtMini, Auralink" || !slices.Equal(changes, want) {
		t.Errorf("%q %+v", got, changes)
	}
	if _, changes := ReplaceChanges("Auralink", terms); changes != nil {
		t.Errorf("a term as itself: %+v", changes)
	}
}

// TestUsed: the terms a text has, as ReplaceChanges matches them.
func TestUsed(t *testing.T) {
	terms := []store.Term{
		{ID: "a", Term: "Auralink", Enabled: true},
		{ID: "k", Term: "Kube Ctl", Enabled: true},
		{ID: "d", Term: "豆包", Enabled: true},
		{ID: "off", Term: "Off", Enabled: false},
		{ID: "go", Term: "Go", Enabled: true},
	}
	got := Used("Ask auralink about kube  ctl and 用豆包, Off, Gopher", terms)
	if !slices.Equal(got, []string{"a", "k", "d"}) {
		t.Errorf("Used = %q", got)
	}
}

// replace is ReplaceChanges' text alone.
func replace(text string, terms []store.Term) string {
	out, _ := ReplaceChanges(text, terms)
	return out
}
