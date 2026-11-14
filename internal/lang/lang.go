// Package lang is the languages VoxtMini speaks: English, Chinese
// (Simplified) and Japanese, and nothing else (user, 2026-11-13). Every
// language setting, hint and target is one of them; a code of another
// language (a setting written before, a service's answer) is unknown.
//
// Codes are BCP-47 primary language subtags ("en", "zh", "ja"). Canonical
// also accepts full tags ("zh-Hans-CN", "en_US"), ISO 639-3 and 639-2
// codes ("cmn", "zho", "jpn") and Cantonese, written in Chinese ("yue");
// Traditional Chinese tags ("zh-Hant", "zh-TW") are not Chinese
// (Simplified) and so unknown.
package lang

import (
	"slices"
	"strings"
)

// The languages' codes.
const (
	English  = "en"
	Chinese  = "zh"
	Japanese = "ja"
)

// Language is one of the three.
type Language struct {
	Code   string // "en", "zh", "ja"
	Name   string // English name: "Chinese"
	Native string // the name in the language itself: "中文"
	Script string // ISO 15924 code of its script: "Latn", "Hans", "Jpan"

	zh string // the Simplified Chinese name
}

// NameIn is the language's name for a UI in locale: the Simplified
// Chinese name for "zh…" locales, else the English name.
func (l Language) NameIn(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "zh") {
		return l.zh
	}
	return l.Name
}

var all = []Language{
	{Code: English, Name: "English", Native: "English", Script: "Latn", zh: "英语"},
	{Code: Chinese, Name: "Chinese", Native: "中文", Script: "Hans", zh: "中文"},
	{Code: Japanese, Name: "Japanese", Native: "日本語", Script: "Jpan", zh: "日语"},
}

// Codes are the three languages' codes, in the settings' order.
var Codes = []string{English, Chinese, Japanese}

// All returns the three languages. The slice is a copy.
func All() []Language { return slices.Clone(all) }

// aliases are the other spellings of the three's primary subtags.
var aliases = map[string]string{
	"eng": English,
	"cmn": Chinese, "zho": Chinese, "chi": Chinese, "yue": Chinese,
	"jpn": Japanese,
}

// Canonical returns the code of the language a tag names ("zh-Hans-CN" →
// "zh", "cmn" → "zh", "en_US" → "en"), or "" when it is none of the three
// (Traditional Chinese: "zh-Hant", "zh-TW", "zh-HK", "zh-MO").
func Canonical(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(tag, "_", "-")))
	base, rest, _ := strings.Cut(tag, "-")
	if a, ok := aliases[base]; ok {
		base = a
	}
	if !slices.Contains(Codes, base) {
		return ""
	}
	if base == Chinese && traditional(rest) {
		return ""
	}
	return base
}

// traditional reports whether a Chinese tag's subtags after the language
// name Traditional Chinese: its script, or a region that writes it.
func traditional(subtags string) bool {
	for _, s := range strings.Split(subtags, "-") {
		switch s {
		case "hant":
			return true
		case "hans":
			return false
		case "tw", "hk", "mo":
			return true
		}
	}
	return false
}

// Lookup finds a language by code, tag or alias (see Canonical).
func Lookup(tag string) (Language, bool) {
	c := Canonical(tag)
	for _, l := range all {
		if l.Code == c {
			return l, true
		}
	}
	return Language{}, false
}

// Keep is list's languages as codes (Canonical), in order, without the
// unknown ones and repeats.
func Keep(list []string) []string {
	var out []string
	for _, s := range list {
		if c := Canonical(s); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}
