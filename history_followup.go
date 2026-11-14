package main

// Follow-up questions about a history entry, in its details
// (history_detail.go; Voxt's TranscriptionDetailWindow follow-up): a
// short chat under the texts. Rewrite's language model answers (the one
// its page chose: local, or a remote service the user picked), streamed;
// each question and answer is kept with the entry (store.History's
// FollowUps) and goes when the entry goes. The request is
// compose.Ask's.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/VoxtMini/internal/compose"
	"github.com/hehehai/VoxtMini/internal/hans"
	"github.com/hehehai/VoxtMini/internal/hotkey"
	"github.com/hehehai/VoxtMini/internal/i18n"
	"github.com/hehehai/VoxtMini/internal/icons"
	"github.com/hehehai/VoxtMini/internal/lang"
	"github.com/hehehai/VoxtMini/internal/llm"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/store"
)

// A follow-up's deadlines: a local model may have to load first.
const (
	askTimeout    = 3 * time.Minute
	askFirstToken = 90 * time.Second
	askStall      = 30 * time.Second
	askDrawEvery  = 80 * time.Millisecond // the streamed answer redraws at most this often
)

// followUps is the follow-up chat of the entry in the details.
type followUps struct {
	id      string // the entry they are of ("" none read)
	loaded  bool
	list    []store.FollowUp
	input   string
	asking  string // the question on its way ("" none)
	partial string // its answer so far
	err     string // the last question's failure
	cancel  context.CancelFunc
	seq     int // numbers the reads and the questions: only the last one lands
}

// closeEntry ends what runs for the entry in the details: its recording,
// its question.
func (a *app) closeEntry() {
	a.stopRecording() // history_playback.go
	a.resetFollowUps()
}

// resetFollowUps forgets the chat shown, stopping its question.
func (a *app) resetFollowUps() {
	f := &a.hist.ask
	if f.cancel != nil {
		f.cancel()
	}
	*f = followUps{seq: f.seq + 1}
}

// askText is what a question is about: the result, else the recognised
// text.
func askText(e store.Entry) string {
	if s := strings.TrimSpace(e.Result); s != "" {
		return s
	}
	return strings.TrimSpace(e.RawText)
}

// loadFollowUps reads e's follow-ups the first time its details show.
func (a *app) loadFollowUps(e store.Entry) {
	f := &a.hist.ask
	if f.id == e.ID {
		return
	}
	a.resetFollowUps()
	f.id = e.ID
	seq, hs := f.seq, a.lib.store.History()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		list, err := hs.FollowUps(ctx, e.ID)
		a.post(func() {
			if f.seq != seq {
				return
			}
			f.loaded, f.list = true, list
			if err != nil {
				f.err = i18n.T("Could not read the follow-up questions: %v", err)
			}
		})
	})
}

// askModel is the language model that answers: Rewrite's.
func (a *app) askModel() (local.Model, bool) {
	return a.featureModel(local.LLM, hotkey.Rewrite) // models_page.go
}

// askFollowUp sends the question in the field about e.
func (a *app) askFollowUp(e store.Entry) {
	f := &a.hist.ask
	q := strings.TrimSpace(f.input)
	m, ok := a.askModel()
	if q == "" || f.asking != "" || !f.loaded || !ok {
		return
	}
	main := session.MainLanguage(a.prefs, i18n.Locale())
	in := compose.AskInput{MainLanguage: main, Kind: string(e.Kind), Created: i18n.DateTime(e.CreatedAt),
		Result: askText(e), Question: q}
	if e.Kind == store.KindRewrite {
		in.Source, in.Instruct = e.SourceText, e.PromptText
	}
	for _, t := range f.list {
		in.Turns = append(in.Turns, compose.AskTurn{Question: t.Question, Answer: t.Answer})
	}
	var msgs []llm.Message
	for _, msg := range compose.Ask(in) {
		msgs = append(msgs, llm.Message{Role: llm.Role(msg.Role), Content: msg.Content})
	}
	// The answer in Simplified characters, unless the user writes
	// Japanese (behavior.md 5.30).
	written := hans.Simplify
	if lang.Canonical(main) == lang.Japanese {
		written = func(s string) string { return s }
	}

	f.seq++
	seq, id, list := f.seq, e.ID, f.list
	f.asking, f.partial, f.err, f.input = q, "", "", ""
	ctx, cancel := context.WithTimeout(context.Background(), askTimeout)
	f.cancel = cancel
	svc, hs := a.local, a.lib.store.History()
	a.async(func() {
		defer cancel()
		var b strings.Builder
		var drawn time.Time
		req := llm.Request{Messages: msgs, Skill: llm.SkillRewrite, Timeout: askTimeout,
			FirstTokenTimeout: askFirstToken, StallTimeout: askStall,
			OnDelta: func(d string) {
				b.WriteString(d)
				if time.Since(drawn) < askDrawEvery {
					return
				}
				drawn = time.Now()
				so := written(b.String())
				a.post(func() {
					if f.seq == seq {
						f.partial = so
					}
				})
			}}
		answer, err := "", error(nil)
		chat, _, err := svc.LLM(ctx, m.ID)
		if err == nil {
			var resp llm.Response
			resp, err = chat.Chat(ctx, req)
			answer = written(strings.TrimSpace(resp.Text))
			if err == nil && answer == "" {
				err = errors.New(i18n.T("The model gave no answer."))
			}
		}
		var saveErr error
		if err == nil {
			list = append(append([]store.FollowUp(nil), list...), store.FollowUp{Question: q, Answer: answer, At: time.Now()})
			sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
			saveErr = hs.SetFollowUps(sctx, id, list)
			scancel()
		}
		a.post(func() {
			if f.seq != seq {
				return // the entry was left, or cleared: the answer is not wanted
			}
			f.asking, f.partial, f.cancel = "", "", nil
			switch {
			case err != nil && ctx.Err() == context.Canceled:
				f.input = q // stopped: the question back in the field
			case err != nil:
				f.err, f.input = i18n.T("No answer: %v", err), q
			default:
				f.list = list
				if saveErr != nil {
					f.err = i18n.T("The answer was not kept: %v", saveErr)
				}
			}
		})
	})
}

// stopFollowUp stops the question on its way.
func (a *app) stopFollowUp() {
	if f := &a.hist.ask; f.cancel != nil {
		f.cancel()
	}
}

// clearFollowUps deletes e's follow-ups.
func (a *app) clearFollowUps(e store.Entry) {
	f := &a.hist.ask
	f.seq++
	f.list, f.err = nil, ""
	hs := a.lib.store.History()
	a.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := hs.SetFollowUps(ctx, e.ID, nil); err != nil {
			a.post(func() { a.toast(i18n.T("Could not clear the follow-up questions: %v", err)) })
		}
	})
}

// followUpSection is the follow-up chat of e, under its texts.
func (a *app) followUpSection(c *ui.Context, e store.Entry) {
	if askText(e) == "" {
		return
	}
	a.loadFollowUps(e)
	t := c.Theme()
	k := tok(t)
	f := &a.hist.ask
	_, haveModel := a.askModel()
	ui.Column(c.Key("hist-ask")).Gap(px(t, sp2)).Children(func() {
		ui.Row(c).Gap(px(t, sp2)).AlignItems(ui.Center).MinHeight(px(t, 28)).Children(func() {
			sectionTitleText(c, i18n.T("Follow-up questions")).Grow(1).MinWidth(0)
			if len(f.list) > 0 && f.asking == "" && iconButton(c.Key("hist-ask-clear"), icons.TrashBinMinimalistic, i18n.T("Clear the follow-up questions")).Clicked() {
				a.clearFollowUps(e)
			}
		})
		turn := func(i int, q, ans string, live bool) {
			ui.Column(c.Key(fmt.Sprintf("hist-ask-turn-%d", i))).Gap(px(t, sp1)).Children(func() {
				ui.Text(c, q).FontWeight(500).Selectable()
				fillCard(c).Padding(px(t, sp2), px(t, sp3)).Children(func() {
					switch {
					case live && ans == "":
						ui.Text(c, i18n.T("Thinking…")).TextColor(k.Text3)
					default:
						ui.Text(c, ans).Selectable()
					}
				})
			})
		}
		for i, fu := range f.list {
			turn(i, fu.Question, fu.Answer, false)
		}
		if f.asking != "" {
			turn(len(f.list), f.asking, f.partial, true)
		}
		if f.err != "" {
			notice(c, "hist-ask-err", noticeError, f.err, "", nil, nil)
		}
		if !haveModel {
			caption(c, i18n.T("Rewrite has no language model to answer with."))
			return
		}
		ui.Row(c).Gap(px(t, sp1)).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c.Key("hist-ask-in"), &f.input).Label(i18n.T("Follow-up question")).
				Placeholder(i18n.T("Ask about this entry")).Disabled(f.asking != "" || !f.loaded).Grow(1).MinWidth(0)
			if f.asking != "" {
				if iconButton(c.Key("hist-ask-stop"), icons.Stop, i18n.T("Stop")).Clicked() {
					a.stopFollowUp()
				}
				return
			}
			send := iconButton(c.Key("hist-ask-send"), icons.AltArrowUp, i18n.T("Ask"))
			if send.Clicked() || in.Submitted() {
				a.askFollowUp(e)
			}
		})
	})
}
