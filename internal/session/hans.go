package session

import (
	"github.com/hehehai/VoxtMini/internal/hans"
	"github.com/hehehai/VoxtMini/internal/lang"
	"github.com/hehehai/VoxtMini/internal/store"
)

// VoxtMini's Chinese is Simplified Chinese only (user, 2026-11-13;
// behavior.md 5.30): every text a model makes is written in Simplified
// characters where it enters the session, before anything shows, keeps,
// matches or delivers it: the speech model's live text and final text
// (heard), the language model's draft and answer (written), and the
// result at its delivery (finish, Retry). The user's own text (a
// selection, the dictionary) is not changed where it is stored, only in
// the results made from it. No setting: it always holds.

// heard is recognised speech in Simplified characters, unless the user
// speaks Japanese (the main language): Japanese kanji are often
// Traditional characters. hans.Simplify leaves a text with kana alone.
func (r *run) heard(text string) string {
	if lang.Canonical(r.speechLang()) == lang.Japanese {
		return text
	}
	return hans.Simplify(text)
}

// written is a language model's answer in Simplified characters, unless
// it is in Japanese: Translate's target, else the main language.
func (r *run) written(text string) string {
	l := r.speechLang()
	if r.req.Kind == store.KindTranslate {
		l = r.target
	}
	if lang.Canonical(l) == lang.Japanese {
		return text
	}
	return hans.Simplify(text)
}
