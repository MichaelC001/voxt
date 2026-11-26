package mcp

// Data management tools (require Policy.AllowManage + gate confirmation).

import (
	"fmt"
	"strings"
)

const maxTermsBatch = 100

func manageTools() []tool {
	delTerm := def("delete_term", "Delete one dictionary term by id.", ScopeManage,
		func(c *call, in struct {
			ID string `json:"id" jsonschema:"Term id to delete"`
		}) (any, error) {
			if strings.TrimSpace(in.ID) == "" {
				return nil, fail("bad_arguments", "id is required")
			}
			if err := c.srv.gate.confirm(c, AskManage, "Delete dictionary term", 1, false); err != nil {
				return nil, err
			}
			if err := c.srv.opt.Host.DeleteTerm(c.ctx, in.ID); err != nil {
				return nil, err
			}
			c.count = 1
			return map[string]any{"deleted": true, "id": in.ID}, nil
		})
	delTerm.destructive = true

	delGroup := def("delete_app_group", "Delete one app enhancement group by id.", ScopeManage,
		func(c *call, in struct {
			ID string `json:"id" jsonschema:"App group id to delete"`
		}) (any, error) {
			if strings.TrimSpace(in.ID) == "" {
				return nil, fail("bad_arguments", "id is required")
			}
			if err := c.srv.gate.confirm(c, AskManage, "Delete app enhancement group", 1, false); err != nil {
				return nil, err
			}
			if err := c.srv.opt.Host.DeleteAppGroup(c.ctx, in.ID); err != nil {
				return nil, err
			}
			c.count = 1
			return map[string]any{"deleted": true, "id": in.ID}, nil
		})
	delGroup.destructive = true

	delHistory := def("delete_history", "Delete a single history entry (and its recording) by id.", ScopeManage,
		func(c *call, in struct {
			ID string `json:"id" jsonschema:"Single history entry id to delete"`
		}) (any, error) {
			if strings.TrimSpace(in.ID) == "" {
				return nil, fail("bad_arguments", "id is required")
			}
			// Deleting history always asks for explicit confirmation.
			if err := c.srv.gate.confirm(c, AskManage, "Delete one history entry", 1, true); err != nil {
				return nil, err
			}
			if err := c.srv.opt.Host.DeleteHistory(c.ctx, in.ID); err != nil {
				return nil, err
			}
			c.count = 1
			return map[string]any{"deleted": true, "id": in.ID}, nil
		})
	delHistory.destructive = true

	return []tool{
		def("upsert_terms", "Create or update one or more dictionary terms (batch up to 100).", ScopeManage,
			func(c *call, in struct {
				Terms []TermInput `json:"terms" jsonschema:"Terms to create or update (1-100)"`
			}) (any, error) {
				if len(in.Terms) == 0 {
					return nil, fail("bad_arguments", "terms must contain at least one item")
				}
				if len(in.Terms) > maxTermsBatch {
					return nil, fail("bad_arguments", fmt.Sprintf("at most %d terms per call", maxTermsBatch))
				}
				for i, tm := range in.Terms {
					if strings.TrimSpace(tm.Spelling) == "" {
						return nil, fail("bad_arguments", fmt.Sprintf("terms[%d].spelling is required", i))
					}
				}
				summary := fmt.Sprintf("Add or update %d dictionary term(s)", len(in.Terms))
				if err := c.srv.gate.confirm(c, AskManage, summary, len(in.Terms), len(in.Terms) > 20); err != nil {
					return nil, err
				}
				out, err := c.srv.opt.Host.UpsertTerms(c.ctx, in.Terms)
				if err != nil {
					return nil, err
				}
				if out == nil {
					out = []Term{}
				}
				c.count = len(out)
				return map[string]any{"terms": out}, nil
			}),

		delTerm,

		def("upsert_category", "Create or rename a dictionary category.", ScopeManage,
			func(c *call, in CategoryInput) (any, error) {
				if strings.TrimSpace(in.Name) == "" {
					return nil, fail("bad_arguments", "name is required")
				}
				if err := c.srv.gate.confirm(c, AskManage, "Create or rename dictionary category", 1, false); err != nil {
					return nil, err
				}
				cat, err := c.srv.opt.Host.UpsertCategory(c.ctx, in)
				if err != nil {
					return nil, err
				}
				c.count = 1
				return cat, nil
			}),

		def("upsert_app_group", "Create or update an app enhancement group.", ScopeManage,
			func(c *call, in AppGroupInput) (any, error) {
				if strings.TrimSpace(in.Name) == "" {
					return nil, fail("bad_arguments", "name is required")
				}
				if err := c.srv.gate.confirm(c, AskManage, "Create or update app enhancement group", 1, false); err != nil {
					return nil, err
				}
				grp, err := c.srv.opt.Host.UpsertAppGroup(c.ctx, in)
				if err != nil {
					return nil, err
				}
				c.count = 1
				return grp, nil
			}),

		delGroup,
		delHistory,
	}
}
