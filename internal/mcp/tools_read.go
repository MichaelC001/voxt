package mcp

// Read-only tools (available whenever MCP is enabled).

import "strings"

func readTools() []tool {
	return []tool{
		def("voxtmini_overview", "App status, active MCP permissions, counts, ready models, and microphones. Call first.", ScopeRead,
			func(c *call, _ struct{}) (any, error) {
				ov, err := c.srv.opt.Host.Overview(c.ctx)
				if err != nil {
					return nil, err
				}
				ov.Permissions = PolicySummary{
					Read:         c.policy.Enabled,
					Manage:       c.policy.Enabled && c.policy.AllowManage,
					LocalModels:  c.policy.Enabled && c.policy.AllowLocalModels,
					RemoteModels: c.policy.Enabled && c.policy.AllowLocalModels && c.policy.AllowRemoteModels,
					Trigger:      c.policy.Enabled && c.policy.AllowTrigger,
				}
				c.count = 1
				return ov, nil
			}),

		def("list_history", "Search and page recent transcription, translation, and rewrite history summaries.", ScopeRead,
			func(c *call, in struct {
				Kind   string `json:"kind,omitempty" jsonschema:"Filter: transcribe, translate, or rewrite"`
				Query  string `json:"query,omitempty" jsonschema:"Text search query"`
				Limit  int    `json:"limit,omitempty" jsonschema:"Max entries (1-50, default 20)"`
				Cursor string `json:"cursor,omitempty" jsonschema:"Pagination cursor from previous page"`
			}) (any, error) {
				if in.Kind != "" && in.Kind != "transcribe" && in.Kind != "translate" && in.Kind != "rewrite" {
					return nil, fail("bad_arguments", "kind must be transcribe, translate, or rewrite")
				}
				limit := in.Limit
				if limit <= 0 {
					limit = 20
				}
				if limit > 50 {
					limit = 50
				}
				page, err := c.srv.opt.Host.ListHistory(c.ctx, HistoryQuery{
					Kind:   in.Kind,
					Query:  strings.TrimSpace(in.Query),
					Limit:  limit,
					Cursor: in.Cursor,
				})
				if err != nil {
					return nil, err
				}
				if page.Entries == nil {
					page.Entries = []HistoryItem{}
				}
				c.count = len(page.Entries)
				return page, nil
			}),

		def("get_history", "Get full details for one history entry, including corrections, follow-ups, and audio metadata.", ScopeRead,
			func(c *call, in struct {
				ID string `json:"id" jsonschema:"History entry id"`
			}) (any, error) {
				if strings.TrimSpace(in.ID) == "" {
					return nil, fail("bad_arguments", "id is required")
				}
				d, err := c.srv.opt.Host.GetHistory(c.ctx, in.ID)
				if err != nil {
					return nil, err
				}
				c.count = 1
				return d, nil
			}),

		def("list_terms", "List dictionary terms, optionally filtered by search text or category.", ScopeRead,
			func(c *call, in struct {
				Search   string `json:"search,omitempty" jsonschema:"Substring filter on spelling or hints"`
				Category string `json:"category,omitempty" jsonschema:"Category name or id"`
			}) (any, error) {
				terms, err := c.srv.opt.Host.ListTerms(c.ctx, strings.TrimSpace(in.Search), strings.TrimSpace(in.Category))
				if err != nil {
					return nil, err
				}
				if terms == nil {
					terms = []Term{}
				}
				c.count = len(terms)
				return map[string]any{"terms": terms}, nil
			}),

		def("list_categories", "List dictionary categories.", ScopeRead,
			func(c *call, _ struct{}) (any, error) {
				cats, err := c.srv.opt.Host.ListCategories(c.ctx)
				if err != nil {
					return nil, err
				}
				if cats == nil {
					cats = []Category{}
				}
				c.count = len(cats)
				return map[string]any{"categories": cats}, nil
			}),

		def("list_app_groups", "List app enhancement groups, their apps, domains, prompts, and post-paste keys.", ScopeRead,
			func(c *call, _ struct{}) (any, error) {
				groups, err := c.srv.opt.Host.ListAppGroups(c.ctx)
				if err != nil {
					return nil, err
				}
				if groups == nil {
					groups = []AppGroup{}
				}
				c.count = len(groups)
				return map[string]any{"groups": groups}, nil
			}),

		def("list_models", "List available local, Apple, and remote models (never includes API keys).", ScopeRead,
			func(c *call, in struct {
				Kind string `json:"kind,omitempty" jsonschema:"Filter: speech, language, or all (default all)"`
			}) (any, error) {
				kind := strings.ToLower(strings.TrimSpace(in.Kind))
				if kind == "" {
					kind = "all"
				}
				if kind != "all" && kind != "speech" && kind != "language" {
					return nil, fail("bad_arguments", "kind must be speech, language, or all")
				}
				models, err := c.srv.opt.Host.ListModels(c.ctx, kind)
				if err != nil {
					return nil, err
				}
				if models == nil {
					models = []ModelInfo{}
				}
				c.count = len(models)
				return map[string]any{"models": models}, nil
			}),

		def("list_microphones", "List available macOS audio input devices with Core Audio UIDs and priorities.", ScopeRead,
			func(c *call, _ struct{}) (any, error) {
				mics, err := c.srv.opt.Host.ListMicrophones(c.ctx)
				if err != nil {
					return nil, err
				}
				if mics == nil {
					mics = []Microphone{}
				}
				c.count = len(mics)
				return map[string]any{"microphones": mics}, nil
			}),
	}
}
