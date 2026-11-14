package session

import (
	"context"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/hans"
	"github.com/hehehai/VoxtMini/internal/settings"
	"github.com/hehehai/VoxtMini/internal/store"
)

// traditional reports whether s has Traditional Chinese in it.
func traditional(s string) bool { return hans.Simplify(s) != s }

// noTraditional fails on a snapshot that shows Traditional Chinese, or a
// history entry or paste that holds it.
func noTraditional(t *testing.T, h *harness) {
	t.Helper()
	for _, s := range h.rec.all() {
		for _, x := range []string{s.Partial, s.Raw, s.Draft, s.Result, s.Title} {
			if traditional(x) {
				t.Errorf("snapshot (%s) shows %q", s.State, x)
			}
		}
	}
	for _, e := range h.hist.all() {
		if traditional(e.RawText) || traditional(e.Result) || traditional(e.PromptText) {
			t.Errorf("entry %q / %q / %q", e.RawText, e.PromptText, e.Result)
		}
	}
	for _, p := range h.text.pastes() {
		if traditional(p) {
			t.Errorf("pasted %q", p)
		}
	}
}

// TestSimplifiedOnly: what the models write in Traditional Chinese shows,
// is kept and is delivered in Simplified (behavior.md 5.30): the live
// text, the recognised text, the clean-up's draft and its answer, a
// translation, a rewrite's JSON answer, a dictionary term the user wrote
// in Traditional; Japanese is never changed.
func TestSimplifiedOnly(t *testing.T) {
	glossary := func(context.Context) ([]store.Term, error) {
		return []store.Term{{Term: "頭髮店", Aliases: []string{"hair shop"}, Enabled: true}}, nil
	}
	t.Run("transcribe: live text, raw, draft, result", func(t *testing.T) {
		prov := &fakeStreamer{partial: "後來我們", final: "後來我們在這裡看著頭髮"}
		model := &fakeLLM{text: "後來，我們在這裡看著頭髮乾淨了。", chunk: 1}
		h := newHarness(t, prov, model, func(c *Config, s *settings.Settings) {
			s.General.MainLanguage = "zh"
			s.Features.Transcribe.Enhance = true
			c.LLMTimeout = 5 * time.Second
		})
		h.speak(h.start(), 5)
		end := h.rec.wait(t)
		if end.State != Done || end.Raw != "后来我们在这里看着头发" || end.Result != "后来，我们在这里看着头发干净了。" {
			t.Errorf("end %s raw %q result %q", end.State, end.Raw, end.Result)
		}
		h.rec.waitFor(t, "the live text", func(s Snapshot) bool { return s.Partial == "后来我们" })
		h.rec.waitFor(t, "a draft", func(s Snapshot) bool { return s.State == Composing && s.Draft != "" })
		// The clean-up got the text in Simplified.
		if reqs := model.requests(); len(reqs) != 1 || !containsText(reqs[0], "后来我们在这里看着头发") {
			t.Errorf("requests %+v", reqs)
		}
		noTraditional(t, h)
	})
	t.Run("transcribe: a Traditional dictionary term, no language model", func(t *testing.T) {
		h := newHarness(t, &fakeFile{text: "去 hair shop 剪頭髮"}, nil, func(c *Config, s *settings.Settings) {
			s.General.MainLanguage = "zh"
			c.Glossary = glossary
		})
		h.speak(h.start(), 5)
		if end := h.rec.wait(t); end.Result != "去 头发店 剪头发" {
			t.Errorf("result %q", end.Result)
		}
		noTraditional(t, h)
	})
	t.Run("transcribe: Japanese is left alone", func(t *testing.T) {
		for _, tc := range []struct{ main, text string }{
			{"ja", "東京都議会議員選挙"},             // no kana: the main language says Japanese
			{"zh", "東京駅で電話を掛けました。"},         // kana
			{"en", "会議の後で、時間があれば連絡してください。"}, // kana
		} {
			h := newHarness(t, &fakeFile{text: tc.text}, &fakeLLM{text: tc.text}, func(c *Config, s *settings.Settings) {
				s.General.MainLanguage = tc.main
				s.Features.Transcribe.Enhance = true
			})
			h.speak(h.start(), 5)
			if end := h.rec.wait(t); end.Raw != tc.text || end.Result != tc.text {
				t.Errorf("%s: raw %q result %q, want %q", tc.main, end.Raw, end.Result, tc.text)
			}
		}
	})
	t.Run("translate into Chinese, and into Japanese", func(t *testing.T) {
		for _, tc := range []struct{ target, answer, want string }{
			{"zh-Hans", "我們在這裡開會。", "我们在这里开会。"},
			{"ja", "東京駅で会議", "東京駅で会議"},
			{"ja", "東京都議会", "東京都議会"}, // no kana: the target says Japanese
		} {
			h := newHarness(t, &fakeFile{text: "開會 meeting"}, &fakeLLM{text: tc.answer, chunk: 2}, func(c *Config, s *settings.Settings) {
				s.General.MainLanguage = "zh"
				c.Select = &fakeSelector{}
			})
			id, err := h.m.Start(StartRequest{Kind: store.KindTranslate, Target: slack, TargetLanguage: tc.target})
			if err != nil {
				t.Fatal(err)
			}
			h.speak(id, 5)
			end := h.rec.wait(t)
			if end.Raw != "开会 meeting" || end.Result != tc.want {
				t.Errorf("%s: raw %q result %q, want %q", tc.target, end.Raw, end.Result, tc.want)
			}
		}
	})
	t.Run("rewrite: a JSON answer", func(t *testing.T) {
		model := &fakeLLM{text: `{"title": "會議通知", "content": "明天下午三點開會，請準時參加。"}`, chunk: 3}
		h := newHarness(t, &fakeFile{text: "write a meeting notice"}, model, func(c *Config, s *settings.Settings) {
			c.Select = &fakeSelector{}
		})
		id, err := h.m.Start(StartRequest{Kind: store.KindRewrite, Target: slack})
		if err != nil {
			t.Fatal(err)
		}
		h.speak(id, 5)
		end := h.rec.wait(t)
		if end.Title != "会议通知" || end.Result != "明天下午三点开会，请准时参加。" {
			t.Errorf("title %q result %q", end.Title, end.Result)
		}
		noTraditional(t, h)
	})
	t.Run("rewrite a Traditional selection: the result is Simplified", func(t *testing.T) {
		h := newHarness(t, &fakeFile{text: "fix typos"}, &fakeLLM{text: "這個軟體很好用。"}, func(c *Config, s *settings.Settings) {
			c.Select = &fakeSelector{text: "這個軟體很好用"}
		})
		id, err := h.m.Start(StartRequest{Kind: store.KindRewrite, Target: slack})
		if err != nil {
			t.Fatal(err)
		}
		h.speak(id, 5)
		end := h.rec.wait(t)
		if end.Result != "这个软体很好用。" || end.Selection != "這個軟體很好用" {
			t.Errorf("result %q selection %q", end.Result, end.Selection)
		}
	})
	t.Run("retry", func(t *testing.T) {
		h, e := retryHarness(t, &fakeFile{text: "後來 hello aura link"}, &fakeLLM{text: "後來，VoxtMini 說好。"}, false)
		out, err := h.m.Retry(context.Background(), RetryRequest{Entry: e})
		if err != nil || out.RawText != "后来 hello aura link" || out.Result != "后来，VoxtMini 说好。" {
			t.Errorf("entry %q %q, %v", out.RawText, out.Result, err)
		}
		h, e = retryHarness(t, &fakeFile{text: "後來 hello aura link"}, nil, false)
		if out, err := h.m.Retry(context.Background(), RetryRequest{Entry: e}); err != nil || out.Result != "后来 hello VoxtMini" {
			t.Errorf("entry %q, %v", out.Result, err)
		}
	})
}
