package main

// mcpHost implements internal/mcp.Host against VoxtMini's real store,
// app enhancement groups, models, microphones, and external trigger service.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/hehehai/VoxtMini/internal/appctx"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/local"
	"github.com/hehehai/VoxtMini/internal/mcp"
	"github.com/hehehai/VoxtMini/internal/session"
	"github.com/hehehai/VoxtMini/internal/store"
	"github.com/hehehai/VoxtMini/internal/trigger"
)

type mcpHost struct{ a *app }

var _ mcp.Host = mcpHost{}

func (h mcpHost) Policy() mcp.Policy {
	if h.a.settings == nil {
		return mcp.Policy{}
	}
	s := h.a.settings.Get().MCP
	clients := make(map[string]mcp.ClientGrant, len(s.Clients))
	for _, c := range s.Clients {
		clients[c.Key] = mcp.ClientGrant{
			Allowed:       true,
			AlwaysManage:  c.AlwaysManage,
			AlwaysModels:  c.AlwaysModels,
			AlwaysTrigger: c.AlwaysTrigger,
		}
	}
	return mcp.Policy{
		Enabled:           s.Enabled,
		AllowManage:       s.AllowManage,
		AllowLocalModels:  s.AllowLocalModels,
		AllowRemoteModels: s.AllowLocalModels && s.AllowRemoteModels,
		AllowTrigger:      s.AllowTrigger,
		Clients:           clients,
	}
}

func (h mcpHost) Ready() error {
	if h.a.lib == nil || h.a.lib.store == nil {
		return errors.New("VoxtMini store is not ready")
	}
	return nil
}

func (h mcpHost) Changed() {
	h.a.post(func() {
		h.a.refreshMCPStatus()
	})
}

func (h mcpHost) Overview(ctx context.Context) (mcp.Overview, error) {
	p := h.Policy()
	st, _ := h.SessionStatus(ctx)
	mics, _ := h.ListMicrophones(ctx)
	models, _ := h.ListModels(ctx, "")
	ms := make([]mcp.ModelSummary, 0, len(models))
	for _, m := range models {
		ms = append(ms, mcp.ModelSummary{
			ID:     m.ID,
			Name:   m.Name,
			Kind:   m.Kind,
			Source: m.Source,
			Ready:  m.Ready,
		})
	}
	var counts mcp.OverviewCounts
	if h.a.lib != nil && h.a.lib.store != nil {
		if list, err := h.a.lib.store.History().List(ctx, store.HistoryQuery{}); err == nil {
			counts.History = len(list)
		}
		if terms, err := h.a.lib.store.Dictionary().List(ctx, ""); err == nil {
			counts.Terms = len(terms)
		}
		if cats, err := h.a.lib.store.Dictionary().Categories(ctx); err == nil {
			counts.Categories = len(cats)
		}
	}
	if h.a.aeBook != nil {
		counts.AppGroups = len(h.a.aeBook.get().Groups)
	}
	return mcp.Overview{
		AppVersion: h.a.version,
		Session:    st,
		Permissions: mcp.PolicySummary{
			Read:         p.Enabled,
			Manage:       p.Enabled && p.AllowManage,
			LocalModels:  p.Enabled && p.AllowLocalModels,
			RemoteModels: p.Enabled && p.AllowLocalModels && p.AllowRemoteModels,
			Trigger:      p.Enabled && p.AllowTrigger,
		},
		Counts:      counts,
		Models:      ms,
		Microphones: mics,
	}, nil
}

func (h mcpHost) ListHistory(ctx context.Context, req mcp.HistoryQuery) (mcp.HistoryPage, error) {
	offset := 0
	if req.Cursor != "" {
		if n, err := strconv.Atoi(req.Cursor); err == nil && n >= 0 {
			offset = n
		}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	q := store.HistoryQuery{
		Kind:   store.Kind(req.Kind),
		Search: req.Query,
		Limit:  limit + 1,
		Offset: offset,
	}
	rows, err := h.a.lib.store.History().List(ctx, q)
	if err != nil {
		return mcp.HistoryPage{}, err
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		nextCursor = strconv.Itoa(offset + limit)
	}
	out := make([]mcp.HistoryItem, 0, len(rows))
	for _, e := range rows {
		preview := e.Result
		if preview == "" {
			preview = e.RawText
		}
		if r := []rune(preview); len(r) > 120 {
			preview = string(r[:120]) + "…"
		}
		out = append(out, mcp.HistoryItem{
			ID:        e.ID,
			CreatedAt: e.CreatedAt,
			Kind:      string(e.Kind),
			Status:    string(e.Status),
			AppName:   e.AppName,
			Preview:   preview,
			HasAudio:  e.Audio != "",
		})
	}
	return mcp.HistoryPage{Entries: out, NextCursor: nextCursor}, nil
}

func (h mcpHost) GetHistory(ctx context.Context, id string) (mcp.HistoryDetail, error) {
	e, err := h.a.lib.store.History().Get(ctx, id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return mcp.HistoryDetail{}, mcp.ErrNotFound
		}
		return mcp.HistoryDetail{}, err
	}
	corrs := make([]mcp.Correction, 0, len(e.Corrections))
	for _, c := range e.Corrections {
		corrs = append(corrs, mcp.Correction{From: c.From, To: c.To})
	}
	fups, _ := h.a.lib.store.History().FollowUps(ctx, id)
	outFups := make([]mcp.FollowUp, 0, len(fups))
	for _, f := range fups {
		outFups = append(outFups, mcp.FollowUp{Question: f.Question, Answer: f.Answer, At: f.At})
	}
	var audio *mcp.AudioMeta
	if e.Audio != "" {
		audio = &mcp.AudioMeta{
			HasAudio:   true,
			DurationMS: e.AudioMS,
			Path:       h.a.lib.store.History().AudioPath(e.Audio),
		}
	}
	return mcp.HistoryDetail{
		ID:          e.ID,
		CreatedAt:   e.CreatedAt,
		Kind:        string(e.Kind),
		Status:      string(e.Status),
		Error:       e.Error,
		AppBundle:   e.AppBundle,
		AppName:     e.AppName,
		URLHost:     e.URLHost,
		RuleName:    e.RuleName,
		RawText:     e.RawText,
		SourceText:  e.SourceText,
		PromptText:  e.PromptText,
		Result:      e.Result,
		TargetLang:  e.TargetLang,
		Delivery:    e.Delivery,
		ASRModel:    e.ASRModel,
		LLMModel:    e.LLMModel,
		AudioMS:     e.AudioMS,
		ASRMS:       e.ASRMS,
		LLMMS:       e.LLMMS,
		TotalMS:     e.TotalMS,
		Corrections: corrs,
		FollowUps:   outFups,
		Audio:       audio,
	}, nil
}

func (h mcpHost) ListTerms(ctx context.Context, search, category string) ([]mcp.Term, error) {
	terms, err := h.a.lib.store.Dictionary().List(ctx, search)
	if err != nil {
		return nil, err
	}
	out := make([]mcp.Term, 0, len(terms))
	for _, t := range terms {
		if category != "" && t.Category != category {
			continue
		}
		out = append(out, mcpTermFromStore(t))
	}
	return out, nil
}

func (h mcpHost) ListCategories(ctx context.Context) ([]mcp.Category, error) {
	cats, err := h.a.lib.store.Dictionary().Categories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]mcp.Category, 0, len(cats))
	for _, c := range cats {
		out = append(out, mcp.Category{ID: c.ID, Name: c.Name})
	}
	return out, nil
}

func (h mcpHost) ListAppGroups(_ context.Context) ([]mcp.AppGroup, error) {
	if h.a.aeBook == nil {
		return nil, nil
	}
	groups := h.a.aeBook.get().Groups
	out := make([]mcp.AppGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, mcpAppGroupFromCtx(g))
	}
	return out, nil
}

func (h mcpHost) ListModels(_ context.Context, kind string) ([]mcp.ModelInfo, error) {
	if h.a.local == nil {
		return nil, nil
	}
	var out []mcp.ModelInfo
	add := func(lk local.Kind, label string) {
		for i, m := range h.a.local.Installed(lk) {
			src := modelSource(m)
			out = append(out, mcp.ModelInfo{
				ID:        m.ID,
				Name:      m.Name,
				Kind:      label,
				Source:    src,
				Installed: true,
				Ready:     true,
				Features:  append([]string(nil), m.Features...),
				Default:   i == 0 && !m.Remote,
			})
		}
	}
	if kind == "" || kind == "speech" {
		add(local.ASR, "speech")
	}
	if kind == "" || kind == "language" {
		add(local.LLM, "language")
	}
	return out, nil
}

func (h mcpHost) ListMicrophones(ctx context.Context) ([]mcp.Microphone, error) {
	devs, err := h.a.triggerDevices(ctx)
	if err != nil {
		return nil, err
	}
	prefs := h.a.settings.Get().Audio.Devices
	prio := make(map[string]int, len(prefs))
	for i, id := range prefs {
		prio[id] = i + 1
	}
	out := make([]mcp.Microphone, 0, len(devs))
	for _, d := range devs {
		out = append(out, mcp.Microphone{
			UID:       d.ID,
			Name:      d.Name,
			Transport: d.Transport,
			IsDefault: d.Default,
			Priority:  prio[d.ID],
		})
	}
	return out, nil
}

func (h mcpHost) UpsertTerms(ctx context.Context, terms []mcp.TermInput) ([]mcp.Term, error) {
	now := time.Now()
	dict := h.a.lib.store.Dictionary()
	out := make([]mcp.Term, 0, len(terms))
	for _, in := range terms {
		id := strings.TrimSpace(in.ID)
		var existing store.Term
		if id != "" {
			if t, err := dict.Get(ctx, id); err == nil {
				existing = t
			}
		} else {
			id = newMCPID("term")
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		} else if existing.ID != "" {
			enabled = existing.Enabled
		}
		created := existing.CreatedAt
		if created.IsZero() {
			created = now
		}
		st := store.Term{
			ID:        id,
			Term:      strings.TrimSpace(in.Spelling),
			Aliases:   in.Hints,
			Note:      existing.Note,
			Enabled:   enabled,
			Category:  in.Category,
			Group:     in.Group,
			CreatedAt: created,
			UpdatedAt: now,
		}
		if err := dict.Put(ctx, st); err != nil {
			return nil, err
		}
		out = append(out, mcpTermFromStore(st))
	}
	h.a.post(func() { h.a.loadDict() })
	return out, nil
}

func (h mcpHost) DeleteTerm(ctx context.Context, id string) error {
	if _, err := h.a.lib.store.Dictionary().Get(ctx, id); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return mcp.ErrNotFound
		}
		return err
	}
	if err := h.a.lib.store.Dictionary().Delete(ctx, id); err != nil {
		return err
	}
	h.a.post(func() { h.a.loadDict() })
	return nil
}

func (h mcpHost) UpsertCategory(ctx context.Context, in mcp.CategoryInput) (mcp.Category, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newMCPID("cat")
	}
	cat := store.Category{ID: id, Name: strings.TrimSpace(in.Name), CreatedAt: time.Now()}
	if err := h.a.lib.store.Dictionary().PutCategory(ctx, cat); err != nil {
		return mcp.Category{}, err
	}
	h.a.post(func() { h.a.loadDict() })
	return mcp.Category{ID: cat.ID, Name: cat.Name}, nil
}

func (h mcpHost) UpsertAppGroup(ctx context.Context, in mcp.AppGroupInput) (mcp.AppGroup, error) {
	if h.a.aeBook == nil {
		return mcp.AppGroup{}, errors.New("app enhancement unavailable")
	}
	rules := cloneRules(h.a.aeBook.get())
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newMCPID("grp")
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	apps := make([]appctx.App, 0, len(in.Apps))
	for _, b := range in.Apps {
		if b = strings.TrimSpace(b); b != "" {
			apps = append(apps, appctx.App{BundleID: b})
		}
	}
	feats := appctx.DefaultFeatures
	if len(in.Features) > 0 {
		feats = appctx.Features{}
		for _, f := range in.Features {
			switch strings.ToLower(strings.TrimSpace(f)) {
			case appctx.Transcribe:
				feats.Transcribe = true
			case appctx.Translate:
				feats.Translate = true
			case appctx.Rewrite:
				feats.Rewrite = true
			}
		}
	}
	grp := appctx.Group{
		ID:       id,
		Name:     strings.TrimSpace(in.Name),
		Enabled:  enabled,
		Prompt:   in.Prompt,
		Apps:     apps,
		URLs:     append([]string(nil), in.Domains...),
		AutoKey:  strings.TrimSpace(in.AfterKey),
		Features: feats,
	}
	replaced := false
	for i := range rules.Groups {
		if rules.Groups[i].ID == id {
			rules.Groups[i] = grp
			replaced = true
			break
		}
	}
	if !replaced {
		rules.Groups = append(rules.Groups, grp)
	}
	rules = rules.Normalized()
	h.a.aeBook.set(rules)
	if err := h.a.aeBook.save(ctx, h.a.lib.store.Documents()); err != nil {
		return mcp.AppGroup{}, err
	}
	h.a.post(func() {
		h.a.aeRules = cloneRules(rules)
		h.a.syncTrees()
	})
	return mcpAppGroupFromCtx(grp), nil
}

func (h mcpHost) DeleteAppGroup(ctx context.Context, id string) error {
	if h.a.aeBook == nil {
		return mcp.ErrNotFound
	}
	rules := cloneRules(h.a.aeBook.get())
	idx := -1
	for i := range rules.Groups {
		if rules.Groups[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return mcp.ErrNotFound
	}
	rules.Groups = append(rules.Groups[:idx], rules.Groups[idx+1:]...)
	h.a.aeBook.set(rules)
	if err := h.a.aeBook.save(ctx, h.a.lib.store.Documents()); err != nil {
		return err
	}
	h.a.post(func() {
		h.a.aeRules = cloneRules(rules)
		h.a.syncTrees()
	})
	return nil
}

func (h mcpHost) DeleteHistory(ctx context.Context, id string) error {
	if _, err := h.a.lib.store.History().Get(ctx, id); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return mcp.ErrNotFound
		}
		return err
	}
	if err := h.a.lib.store.History().Delete(ctx, id); err != nil {
		return err
	}
	h.a.post(func() { h.a.historyChanged() })
	return nil
}

func (h mcpHost) TriggerSession(ctx context.Context, req mcp.TriggerRequest) (mcp.TriggerResult, error) {
	if h.a.triggerSvc == nil {
		return mcp.TriggerResult{}, errors.New("trigger service unavailable")
	}
	res, err := h.a.triggerSvc.Trigger(ctx, trigger.Request{
		Feature:        req.Feature,
		Action:         trigger.Action(req.Action),
		Device:         req.Device,
		TargetLanguage: req.TargetLanguage,
		Deliver:        req.Deliver,
		Source:         "mcp",
	})
	if err != nil {
		if errors.Is(err, session.ErrBusy) {
			return mcp.TriggerResult{}, mcp.ErrBusy
		}
		return mcp.TriggerResult{}, err
	}
	mic := ""
	if res.Device != nil {
		mic = res.Device.Name
		if mic == "" {
			mic = res.Device.ID
		}
	}
	return mcp.TriggerResult{
		State:      res.State,
		Feature:    res.Feature,
		Microphone: mic,
		Message:    res.Text,
	}, nil
}

func (h mcpHost) SessionStatus(_ context.Context) (mcp.SessionStatus, error) {
	if h.a.triggerSvc == nil {
		return mcp.SessionStatus{State: "idle"}, nil
	}
	st := h.a.triggerSvc.Status()
	mic := ""
	if st.Device != nil {
		mic = st.Device.Name
		if mic == "" {
			mic = st.Device.ID
		}
	}
	return mcp.SessionStatus{
		State:      st.State,
		Feature:    st.Feature,
		Microphone: mic,
		LastResult: st.Text,
		LastError:  st.Error,
	}, nil
}

func mcpTermFromStore(t store.Term) mcp.Term {
	return mcp.Term{
		ID:       t.ID,
		Spelling: t.Term,
		Hints:    append([]string(nil), t.Aliases...),
		Category: t.Category,
		Group:    t.Group,
		Enabled:  t.Enabled,
		Uses:     t.Uses,
	}
}

func mcpAppGroupFromCtx(g appctx.Group) mcp.AppGroup {
	apps := make([]string, 0, len(g.Apps))
	for _, a := range g.Apps {
		apps = append(apps, a.BundleID)
	}
	var feats []string
	if g.Features.Transcribe {
		feats = append(feats, appctx.Transcribe)
	}
	if g.Features.Translate {
		feats = append(feats, appctx.Translate)
	}
	if g.Features.Rewrite {
		feats = append(feats, appctx.Rewrite)
	}
	return mcp.AppGroup{
		ID:       g.ID,
		Name:     g.Name,
		Enabled:  g.Enabled,
		Apps:     apps,
		Domains:  append([]string(nil), g.URLs...),
		Prompt:   g.Prompt,
		AfterKey: g.AutoKey,
		Features: feats,
	}
}

func modelSource(m local.Model) string {
	if m.Remote {
		return "remote"
	}
	if strings.HasPrefix(m.ID, "apple:") {
		return "apple"
	}
	return "local"
}

func newMCPID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
