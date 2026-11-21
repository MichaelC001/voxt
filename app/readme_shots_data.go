//go:build readmeshots

package main

// The README shots' made-up data (readme_shots.go): a history, a
// dictionary, the overlay's texts and the editor window's, in English and
// in Chinese. Nothing here is anyone's.

import (
	"context"
	"time"

	"github.com/hehehai/VoxtMini/internal/store"
)

// readmeText is what one language's shots show.
type readmeText struct {
	// The editor window behind the overlay shots (readmeEditor).
	editorTitle string
	editor      []string
	// Transcribe: the live text while recording.
	live string
	// Translate: the selection, its translation, into which language.
	trSource, trResult, trLang string
	// Rewrite: the instruction, its answer; a follow-up and its answer.
	rwAsk, rwTitle, rwAnswer, rwFollow, rwFollowTitle, rwFollowAnswer string
	// History: the entry shown, its follow-up question.
	followQ, followA string
	history          []store.Entry
	terms            []store.Term
	categories       []store.Category
}

// The entries' ids: the first one (with a recording) is History's shot.
const (
	readmeHistMain = "01README000000000000000001"
)

func readmeTexts(lang string) readmeText {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	if lang == "zh-Hans" {
		return readmeText{
			editorTitle: "发布计划",
			editor: []string{
				"周四发布 · 待办",
				"1. 引导页：文案定稿，截图换成新版",
				"2. 历史页：录音回放和追问",
				"3. 词典：分类和使用次数",
				"",
				"会议纪要",
			},
			live:     "明天下午三点和设计组过一遍引导页，然后把发布说明发给大家",
			trSource: "明天下午三点和设计组过一遍引导页，然后把发布说明发给大家。",
			trResult: "Tomorrow at 3 pm we'll go through the onboarding with the design team, then send everyone the release notes.",
			trLang:   "en",
			rwAsk:    "写一条消息，告诉团队发布推迟到周四", rwTitle: "发布推迟",
			rwAnswer: "大家好，发布改到周四：我们想先把引导页做完，再跑一轮完整的测试。",
			rwFollow: "再短一点，语气轻松些", rwFollowTitle: "发布推迟",
			rwFollowAnswer: "周四发布，先把引导页磨好。",
			followQ:        "评审改到什么时候？", followA: "周四下午三点；新的议程今晚发出。",
			history: []store.Entry{
				{ID: readmeHistMain, Kind: store.KindTranscribe, CreatedAt: ago(4 * time.Minute), Status: store.StatusDone,
					AppBundle: "com.apple.Notes", AppName: "Notes",
					RawText: "嗯 那个设计评审改到周四下午三点 新的议程我今晚发出来",
					Result:  "设计评审改到周四下午三点，新的议程我今晚发出来。", Delivery: "insert",
					ASRProvider: "asr.accurate", ASRModel: "asr.accurate", LLMProvider: "llm.balanced", LLMModel: "llm.balanced",
					AudioMS: 6400, ASRMS: 540, LLMMS: 980, TotalMS: 8100, Audio: readmeHistMain + ".wav"},
				{ID: "01README000000000000000002", Kind: store.KindTranslate, CreatedAt: ago(11 * time.Minute), Status: store.StatusDone,
					AppBundle: "com.tinyspeck.slackmacgap", AppName: "Slack",
					RawText: "帮我跟他们说周五上午的同步取消", Result: "Heads-up: Friday morning's sync is cancelled.",
					TargetLang: "en", Delivery: "insert",
					ASRProvider: "asr.accurate", ASRModel: "asr.accurate", LLMProvider: "llm.balanced", LLMModel: "llm.balanced",
					AudioMS: 3100, ASRMS: 410, LLMMS: 1200, TotalMS: 5000, Audio: "01README000000000000000002.wav"},
				{ID: "01README000000000000000003", Kind: store.KindRewrite, CreatedAt: ago(26 * time.Minute), Status: store.StatusDone,
					AppBundle: "com.apple.mail", AppName: "Mail",
					PromptText: "改得正式一点", SourceText: "测试还在跑，发布可能要拖到周四，不好意思哈",
					Result: "测试仍在进行中，发布将顺延至周四，敬请谅解。", Delivery: "insert",
					ASRProvider: "asr.accurate", ASRModel: "asr.accurate", LLMProvider: "llm.balanced", LLMModel: "llm.balanced",
					AudioMS: 1900, ASRMS: 300, LLMMS: 1500, TotalMS: 3900},
				{ID: "01README000000000000000004", Kind: store.KindTranscribe, CreatedAt: ago(48 * time.Minute), Status: store.StatusDone,
					AppBundle: "com.tinyspeck.slackmacgap", AppName: "Slack",
					RawText: "我十分钟后到 你们先开始", Result: "我十分钟后到，你们先开始。", Delivery: "insert",
					ASRProvider: "asr.fast", ASRModel: "asr.fast", AudioMS: 2200, ASRMS: 260, TotalMS: 2600, Audio: "01README000000000000000004.wav"},
				{ID: "01README000000000000000005", Kind: store.KindTranslate, CreatedAt: ago(2 * time.Hour), Status: store.StatusDone,
					AppBundle: "com.google.Chrome", AppName: "Google Chrome", URLHost: "go.dev",
					SourceText: "A goroutine is a lightweight thread managed by the Go runtime.",
					Result:     "goroutine 是由 Go 运行时管理的轻量级线程。", TargetLang: "zh-Hans", Delivery: "card",
					LLMProvider: "llm.balanced", LLMModel: "llm.balanced", LLMMS: 900, TotalMS: 1000},
				{ID: "01README000000000000000006", Kind: store.KindTranscribe, CreatedAt: ago(26 * time.Hour), Status: store.StatusDone,
					AppBundle: "com.apple.Notes", AppName: "Notes",
					RawText: "周末去超市 买牛奶 鸡蛋 还有咖啡豆", Result: "周末去超市：牛奶、鸡蛋、咖啡豆。", Delivery: "insert",
					ASRProvider: "asr.accurate", ASRModel: "asr.accurate", AudioMS: 3300, ASRMS: 380, TotalMS: 3900},
			},
			categories: []store.Category{{ID: "c-work", Name: "工作"}, {ID: "c-names", Name: "人名"}},
			terms: []store.Term{
				{ID: "t1", Term: "VoxtMini", Aliases: []string{"沃克斯特", "vox mini"}, Enabled: true, Category: "c-work"},
				{ID: "t2", Term: "Kubernetes", Aliases: []string{"库伯内特斯", "k8s"}, Enabled: true, Category: "c-work"},
				{ID: "t3", Term: "豆包", Aliases: []string{"逗包"}, Note: "火山引擎的大模型", Enabled: true},
				{ID: "t4", Term: "林知夏", Aliases: []string{"林志夏"}, Note: "设计组", Enabled: true, Category: "c-names"},
				{ID: "t5", Term: "DeepSeek", Aliases: []string{"迪普西克"}, Enabled: true},
				{ID: "t6", Term: "SwiftUI", Enabled: true, Category: "c-work"},
			},
		}
	}
	return readmeText{
		editorTitle: "Launch plan",
		editor: []string{
			"Thursday launch · to do",
			"1. Onboarding: final copy, new screenshots",
			"2. History: playback and follow-up questions",
			"3. Dictionary: categories and usage counts",
			"",
			"Meeting notes",
		},
		live:     "Let's walk through the onboarding with the design team tomorrow at three, then send everyone the release notes",
		trSource: "Let's walk through the onboarding with the design team tomorrow at three, then send everyone the release notes.",
		trResult: "明天下午三点和设计组过一遍引导页，然后把发布说明发给大家。",
		trLang:   "zh-Hans",
		rwAsk:    "write a message telling the team the launch moves to Thursday", rwTitle: "Launch moves to Thursday",
		rwAnswer: "Hi all, the launch moves to Thursday so we can finish the onboarding first.",
		rwFollow: "shorter and more casual", rwFollowTitle: "Launch moves",
		rwFollowAnswer: "Launch is Thursday now, onboarding first.",
		followQ:        "When is the review?", followA: "Thursday at 3 pm; the new agenda comes tonight.",
		history: []store.Entry{
			{ID: readmeHistMain, Kind: store.KindTranscribe, CreatedAt: ago(4 * time.Minute), Status: store.StatusDone,
				AppBundle: "com.apple.Notes", AppName: "Notes",
				RawText: "um so the design review moves to thursday at three and I'll send the new agenda tonight",
				Result:  "The design review moves to Thursday at 3 pm. I'll send the new agenda tonight.", Delivery: "insert",
				ASRProvider: "asr.accurate", ASRModel: "asr.accurate", LLMProvider: "llm.balanced", LLMModel: "llm.balanced",
				AudioMS: 6400, ASRMS: 540, LLMMS: 980, TotalMS: 8100, Audio: readmeHistMain + ".wav"},
			{ID: "01README000000000000000002", Kind: store.KindTranslate, CreatedAt: ago(11 * time.Minute), Status: store.StatusDone,
				AppBundle: "com.tinyspeck.slackmacgap", AppName: "Slack",
				RawText: "tell them friday morning's sync is cancelled", Result: "周五上午的同步会取消了。",
				TargetLang: "zh-Hans", Delivery: "insert",
				ASRProvider: "asr.accurate", ASRModel: "asr.accurate", LLMProvider: "llm.balanced", LLMModel: "llm.balanced",
				AudioMS: 3100, ASRMS: 410, LLMMS: 1200, TotalMS: 5000, Audio: "01README000000000000000002.wav"},
			{ID: "01README000000000000000003", Kind: store.KindRewrite, CreatedAt: ago(26 * time.Minute), Status: store.StatusDone,
				AppBundle: "com.apple.mail", AppName: "Mail",
				PromptText: "make it more formal", SourceText: "tests are still running so the launch prob slips to thursday, sorry!",
				Result: "The tests are still running, so the launch moves to Thursday. Apologies for the delay.", Delivery: "insert",
				ASRProvider: "asr.accurate", ASRModel: "asr.accurate", LLMProvider: "llm.balanced", LLMModel: "llm.balanced",
				AudioMS: 1900, ASRMS: 300, LLMMS: 1500, TotalMS: 3900},
			{ID: "01README000000000000000004", Kind: store.KindTranscribe, CreatedAt: ago(48 * time.Minute), Status: store.StatusDone,
				AppBundle: "com.tinyspeck.slackmacgap", AppName: "Slack",
				RawText: "running ten minutes late start without me", Result: "Running ten minutes late, start without me.", Delivery: "insert",
				ASRProvider: "asr.fast", ASRModel: "asr.fast", AudioMS: 2200, ASRMS: 260, TotalMS: 2600, Audio: "01README000000000000000004.wav"},
			{ID: "01README000000000000000005", Kind: store.KindTranslate, CreatedAt: ago(2 * time.Hour), Status: store.StatusDone,
				AppBundle: "com.google.Chrome", AppName: "Google Chrome", URLHost: "zh.wikipedia.org",
				SourceText: "语音识别是将人类的语音转换为文本的技术。",
				Result:     "Speech recognition turns human speech into text.", TargetLang: "en", Delivery: "card",
				LLMProvider: "llm.balanced", LLMModel: "llm.balanced", LLMMS: 900, TotalMS: 1000},
			{ID: "01README000000000000000006", Kind: store.KindTranscribe, CreatedAt: ago(26 * time.Hour), Status: store.StatusDone,
				AppBundle: "com.apple.Notes", AppName: "Notes",
				RawText: "groceries for the weekend milk eggs and coffee beans", Result: "Groceries for the weekend: milk, eggs and coffee beans.", Delivery: "insert",
				ASRProvider: "asr.accurate", ASRModel: "asr.accurate", AudioMS: 3300, ASRMS: 380, TotalMS: 3900},
		},
		categories: []store.Category{{ID: "c-work", Name: "Work"}, {ID: "c-names", Name: "Names"}},
		terms: []store.Term{
			{ID: "t1", Term: "VoxtMini", Aliases: []string{"vox mini", "voxed mini"}, Enabled: true, Category: "c-work"},
			{ID: "t2", Term: "Kubernetes", Aliases: []string{"cooper netties", "k8s"}, Enabled: true, Category: "c-work"},
			{ID: "t3", Term: "Siobhan", Aliases: []string{"shivon", "chevonne"}, Note: "Design team", Enabled: true, Category: "c-names"},
			{ID: "t4", Term: "Nguyen", Aliases: []string{"win", "new yen"}, Enabled: true, Category: "c-names"},
			{ID: "t5", Term: "DeepSeek", Aliases: []string{"deep seek"}, Enabled: true},
			{ID: "t6", Term: "SwiftUI", Aliases: []string{"swift you eye"}, Enabled: true, Category: "c-work"},
		},
	}
}

// seedReadme writes the history and the dictionary of lang's shots into
// a's library.
func seedReadme(a *app, lang string) error {
	ctx := context.Background()
	tx := readmeTexts(lang)
	h := a.lib.store.History()
	for _, e := range tx.history {
		if err := h.Put(ctx, e); err != nil {
			return err
		}
	}
	if err := h.SetFollowUps(ctx, readmeHistMain, []store.FollowUp{{Question: tx.followQ, Answer: tx.followA, At: time.Now().Add(-time.Minute)}}); err != nil {
		return err
	}
	d := a.lib.store.Dictionary()
	for _, c := range tx.categories {
		if err := d.PutCategory(ctx, c); err != nil {
			return err
		}
	}
	t0 := time.Now().Add(-72 * time.Hour)
	for i, tm := range tx.terms {
		tm.CreatedAt, tm.UpdatedAt = t0.Add(time.Duration(i)*time.Minute), t0.Add(time.Duration(i)*time.Minute)
		if err := d.Put(ctx, tm); err != nil {
			return err
		}
	}
	// Usage counts: VoxtMini most, a name once.
	for i, ids := range [][]string{{"t1", "t2", "t3"}, {"t1", "t6"}, {"t1", "t3"}, {"t1"}, {"t1", "t2"}} {
		if err := d.RecordUses(ctx, ids, time.Now().Add(-time.Duration(i+1)*time.Hour)); err != nil {
			return err
		}
	}
	return nil
}
