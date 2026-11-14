package session

import (
	"slices"
	"strings"
	"unicode"

	"github.com/hehehai/VoxtMini/internal/compose"
	"github.com/hehehai/VoxtMini/internal/lang"
	"github.com/hehehai/VoxtMini/internal/settings"
)

// TranslateTarget is the language Translate writes in: override when
// set, else Translate.Target, else English, or Chinese for a user whose
// main language is English (the main language comes from locale, the UI
// language, when the settings leave it to follow it). Translating into
// the language one speaks would change nothing.
func TranslateTarget(s settings.Settings, locale, override string) string {
	if t := strings.TrimSpace(override); t != "" {
		return t
	}
	if t := strings.TrimSpace(s.Features.Translate.Target); t != "" {
		return t
	}
	main := s.General.MainLanguage
	if main == "" {
		main = locale
	}
	if lang.Canonical(main) == "en" {
		return "zh-Hans"
	}
	return "en"
}

// MainLanguage is the user's main language: the settings', else the UI
// language's.
func MainLanguage(s settings.Settings, locale string) string {
	if m := s.General.MainLanguage; m != "" {
		return m
	}
	return lang.Canonical(locale)
}

// MergeRules makes each feature's task prompt of version 4's rules
// (settings.Feature.Rules, "Your rules"): the built-in prompt with the
// rules after it, so that nothing the user wrote is lost and the prompt
// still does what it did. The set is the main language's; without one,
// Chinese when the rules are written in Chinese. The app runs it once
// over the stored settings, as ScopeLanguages.
func MergeRules(s *settings.Settings) {
	for task, f := range map[compose.Task]*settings.Feature{
		compose.Transcribe: &s.Features.Transcribe.Feature,
		compose.Translate:  &s.Features.Translate.Feature,
		compose.Rewrite:    &s.Features.Rewrite.Feature,
	} {
		rules := strings.TrimSpace(f.Rules)
		f.Rules = ""
		if rules == "" {
			continue
		}
		main := s.General.MainLanguage
		if main == "" && strings.ContainsFunc(rules, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			main = lang.Chinese
		}
		base := strings.TrimSpace(f.Prompt)
		if base == "" {
			base = compose.DefaultPrompt(task, compose.TemplateSet(main))
		}
		f.Prompt = base + "\n\n" + rules
	}
}

// ScopeLanguages keeps the settings' languages to VoxtMini's three
// (internal/lang; user, 2026-11-13), in their codes: a language an older
// version or Voxt's settings left that is none of them goes (Traditional
// Chinese too, which is not Chinese (Simplified)). Then "" follows the
// app's language, Translate's automatic target and a speech service's main
// language. The app runs it once over the stored settings; the pages offer
// only the three.
func ScopeLanguages(s *settings.Settings) {
	g := &s.General
	g.MainLanguage = lang.Canonical(g.MainLanguage)
	g.OtherLanguages = slices.DeleteFunc(lang.Keep(g.OtherLanguages), func(c string) bool { return c == g.MainLanguage })
	t := &s.Features.Translate
	switch c := lang.Canonical(t.Target); c {
	case lang.Chinese:
		t.Target = "zh-Hans" // compose.TargetLanguages' spelling
	default:
		t.Target = c
	}
	asr := slices.Clone(s.Providers.ASR)
	for i := range asr {
		// "auto" (providers.LanguageAuto): no hint, the service detects.
		if l := strings.TrimSpace(asr[i].Language); l != "auto" {
			asr[i].Language = lang.Canonical(l)
		}
	}
	s.Providers.ASR = asr
}

// AllowCopy says whether VoxtMini may press ⌘C in an app to read its
// selection when Accessibility shows none (the helper's allowCopy).
// Not in editors that copy the whole line when nothing is selected (VS
// Code and its forks, Sublime Text, Zed, JetBrains IDEs…): the "selection"
// would be a line the user never chose. Obsidian is one of them.
func AllowCopy(bundleID string) bool {
	id := strings.TrimSpace(bundleID)
	if id == "" {
		return true
	}
	if copiesLine[id] {
		return false
	}
	for _, p := range copiesLinePrefixes {
		if strings.HasPrefix(id, p) {
			return false
		}
	}
	l := strings.ToLower(id)
	return !strings.Contains(l, "vscode") && !strings.Contains(l, "vscodium")
}

// copiesLine are apps whose ⌘C copies the current line on an empty
// selection (Voxt's list, SelectedTextSystemSelectionSupport).
var copiesLine = map[string]bool{
	"com.microsoft.VSCode": true, "com.microsoft.VSCodeInsiders": true, "com.microsoft.VSCodeExploration": true,
	"com.visualstudio.code.oss": true, "com.visualstudio.code": true, "com.vscodium": true,
	"com.google.antigravity": true, "com.sublimetext.3": true, "com.sublimetext.4": true,
	"dev.zed.Zed": true, "dev.zed.Zed-Preview": true, "com.trae.app": true, "com.trae.solo.app": true,
	"cn.trae.app": true, "com.qoder.app": true, "md.obsidian": true,
}

// copiesLinePrefixes are families of such apps: Cursor and other
// ToDesktop builds, JetBrains, Windsurf (Exafunction), Trae, Qoder.
var copiesLinePrefixes = []string{"com.todesktop.", "com.jetbrains.", "com.exafunction.", "com.trae.", "cn.trae.", "com.qoder."}
