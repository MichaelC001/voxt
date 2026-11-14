package lang

import (
	"slices"
	"testing"
)

func TestCanonical(t *testing.T) {
	for tag, want := range map[string]string{
		"en": "en", "en_US": "en", "ENG": "en", "en-GB": "en",
		"zh": "zh", "zh-Hans": "zh", "zh-Hans-CN": "zh", "zh-CN": "zh", "zh-SG": "zh", "cmn": "zh", "zho": "zh", "yue": "zh",
		"ja": "ja", "ja-JP": "ja", "jpn": "ja",
		"zh-Hant": "", "zh-TW": "", "zh-Hant-TW": "", "zh-HK": "", "zh-MO": "",
		"fr": "", "ko": "", "": "", "auto": "", "-": "", "xx": "",
	} {
		if got := Canonical(tag); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestLookup(t *testing.T) {
	cases := []struct{ tag, code, name, native, zh, script string }{
		{"en-US", "en", "English", "English", "英语", "Latn"},
		{"zh-Hans", "zh", "Chinese", "中文", "中文", "Hans"},
		{"ja", "ja", "Japanese", "日本語", "日语", "Jpan"},
	}
	for _, c := range cases {
		l, ok := Lookup(c.tag)
		if !ok || l.Code != c.code || l.Name != c.name || l.Native != c.native || l.NameIn("zh-Hans") != c.zh || l.NameIn("en") != c.name || l.Script != c.script {
			t.Errorf("Lookup(%q) = %+v %v", c.tag, l, ok)
		}
	}
	if _, ok := Lookup("fr"); ok {
		t.Error("French is not one of VoxtMini's languages")
	}
	if len(all) != 3 || !slices.Equal(Codes, []string{"en", "zh", "ja"}) {
		t.Errorf("all %v, Codes %v", all, Codes)
	}
}

func TestKeep(t *testing.T) {
	got := Keep([]string{"fr", "zh-Hans", "zh", "zh-Hant", "ja-JP", " ", "yue", "en"})
	if !slices.Equal(got, []string{"zh", "ja", "en"}) {
		t.Errorf("Keep = %v", got)
	}
}
