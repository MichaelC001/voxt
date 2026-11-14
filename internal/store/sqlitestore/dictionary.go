package sqlitestore

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/store"
)

// dictionaryRepo stores the user's terms (dictionary_terms: migration
// 001; category, group_id, uses, last_used_at 006) and their categories
// (dictionary_categories, 006).
type dictionaryRepo struct{ db *sqlite.DB }

const termColumns = `id, term, aliases, note, enabled, created_at, updated_at, category, group_id, uses, last_used_at`

func (r *dictionaryRepo) Put(ctx context.Context, t store.Term) error {
	if err := validID(t.ID); err != nil {
		return err
	}
	t.Term = strings.TrimSpace(t.Term)
	if t.Term == "" {
		return fmt.Errorf("term %s: empty: %w", t.ID, core.ErrInvalid)
	}
	var aliases []string
	for _, a := range t.Aliases {
		if a = strings.TrimSpace(a); a != "" && !slices.Contains(aliases, a) {
			aliases = append(aliases, a)
		}
	}
	data, err := json.Marshal(aliases)
	if err != nil {
		return err
	}
	if aliases == nil {
		data = []byte("[]")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}
	enabled := 0
	if t.Enabled {
		enabled = 1
	}
	// A stored term keeps its uses: only RecordUses counts them.
	_, err = r.db.Execute(ctx, `INSERT INTO dictionary_terms (`+termColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET term = excluded.term, aliases = excluded.aliases, note = excluded.note,
  enabled = excluded.enabled, created_at = excluded.created_at, updated_at = excluded.updated_at,
  category = excluded.category, group_id = excluded.group_id`,
		t.ID, t.Term, string(data), t.Note, enabled, t.CreatedAt.UnixMilli(), t.UpdatedAt.UnixMilli(),
		t.Category, t.Group, max(t.Uses, 0), untilMS(t.LastUsed))
	return wrap("term "+t.Term, err)
}

func scanTerm(row []any) store.Term {
	t := store.Term{ID: asString(row[0]), Term: asString(row[1]), Note: asString(row[3]), Enabled: asInt(row[4]) != 0,
		CreatedAt: fromMS(row[5]), UpdatedAt: fromMS(row[6]),
		Category: asString(row[7]), Group: asString(row[8]), Uses: int(asInt(row[9])), LastUsed: untilTime(row[10])}
	json.Unmarshal(asBytes(row[2]), &t.Aliases)
	if len(t.Aliases) == 0 {
		t.Aliases = nil
	}
	return t
}

func (r *dictionaryRepo) Get(ctx context.Context, id string) (store.Term, error) {
	rows, err := r.db.Query(ctx, `SELECT `+termColumns+` FROM dictionary_terms WHERE id = ?`, id)
	if err != nil {
		return store.Term{}, wrap("term "+id, err)
	}
	if len(rows.Values) == 0 {
		return store.Term{}, fmt.Errorf("term %s: %w", id, core.ErrNotFound)
	}
	return scanTerm(rows.Values[0]), nil
}

func (r *dictionaryRepo) List(ctx context.Context, search string) ([]store.Term, error) {
	sql, args := `SELECT `+termColumns+` FROM dictionary_terms`, []any(nil)
	if s := strings.TrimSpace(search); s != "" {
		p := likePattern(s)
		sql += ` WHERE term LIKE ? ESCAPE '\' OR aliases LIKE ? ESCAPE '\' OR note LIKE ? ESCAPE '\'`
		args = []any{p, p, p}
	}
	rows, err := r.db.Query(ctx, sql+` ORDER BY term COLLATE NOCASE, id`, args...)
	if err != nil {
		return nil, wrap("terms", err)
	}
	out := make([]store.Term, 0, len(rows.Values))
	for _, row := range rows.Values {
		out = append(out, scanTerm(row))
	}
	return out, nil
}

func (r *dictionaryRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.Execute(ctx, `DELETE FROM dictionary_terms WHERE id = ?`, id)
	if err != nil {
		return wrap("term "+id, err)
	}
	if res.Changes == 0 {
		return fmt.Errorf("term %s: %w", id, core.ErrNotFound)
	}
	return nil
}

func (r *dictionaryRepo) RecordUses(ctx context.Context, ids []string, at time.Time) error {
	var stmts []sqlite.Statement
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		stmts = append(stmts, sqlite.Statement{SQL: `UPDATE dictionary_terms SET uses = uses + 1, last_used_at = ? WHERE id = ?`,
			Args: []any{at.UnixMilli(), id}})
	}
	if len(stmts) == 0 {
		return nil
	}
	_, err := r.db.Transaction(ctx, stmts)
	return wrap("term uses", err)
}

func (r *dictionaryRepo) Categories(ctx context.Context) ([]store.Category, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, created_at FROM dictionary_categories ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, wrap("categories", err)
	}
	out := make([]store.Category, 0, len(rows.Values))
	for _, row := range rows.Values {
		out = append(out, store.Category{ID: asString(row[0]), Name: asString(row[1]), CreatedAt: fromMS(row[2])})
	}
	return out, nil
}

func (r *dictionaryRepo) PutCategory(ctx context.Context, c store.Category) error {
	if err := validID(c.ID); err != nil {
		return err
	}
	c.Name = strings.Join(strings.Fields(c.Name), " ")
	if c.Name == "" {
		return fmt.Errorf("category %s: empty: %w", c.ID, core.ErrInvalid)
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	_, err := r.db.Execute(ctx, `INSERT INTO dictionary_categories (id, name, created_at) VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET name = excluded.name`, c.ID, c.Name, c.CreatedAt.UnixMilli())
	return wrap("category "+c.Name, err)
}

func (r *dictionaryRepo) DeleteCategory(ctx context.Context, id string) error {
	res, err := r.db.Transaction(ctx, []sqlite.Statement{
		{SQL: `UPDATE dictionary_terms SET category = '' WHERE category = ?`, Args: []any{id}},
		{SQL: `DELETE FROM dictionary_categories WHERE id = ?`, Args: []any{id}},
	})
	if err != nil {
		return wrap("category "+id, err)
	}
	if len(res) == 2 && res[1].Changes == 0 {
		return fmt.Errorf("category %s: %w", id, core.ErrNotFound)
	}
	return nil
}
