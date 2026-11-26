package sqlitestore

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/hehehai/VoxtMini/internal/store"
)

const (
	mcpAuditKeepDays = 30
	mcpAuditMaxRows  = 5000
)

type mcpCallRepo struct {
	db      *sqlite.DB
	maxRows int // 0 uses mcpAuditMaxRows (overridden in tests)
}

var _ store.MCPCalls = (*mcpCallRepo)(nil)

func (r *mcpCallRepo) limit() int {
	if r.maxRows > 0 {
		return r.maxRows
	}
	return mcpAuditMaxRows
}

func (r *mcpCallRepo) Record(ctx context.Context, c store.MCPCall) error {
	at := c.At
	if at.IsZero() {
		at = time.Now()
	}
	atMS := at.UnixMilli()
	cutoffMS := at.AddDate(0, 0, -mcpAuditKeepDays).UnixMilli()
	stmts := []sqlite.Statement{
		{
			SQL: `INSERT INTO mcp_calls (at_ms, client_key, client_name, tool, scope, item_count, result, duration_ms, bytes)
			      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{atMS, c.ClientKey, c.ClientName, c.Tool, c.Scope, c.Count, c.Result, c.DurationMS, c.Bytes},
		},
		{
			SQL:  `DELETE FROM mcp_calls WHERE at_ms < ?`,
			Args: []any{cutoffMS},
		},
		{
			SQL:  `DELETE FROM mcp_calls WHERE id NOT IN (SELECT id FROM mcp_calls ORDER BY at_ms DESC, id DESC LIMIT ?)`,
			Args: []any{r.limit()},
		},
	}
	_, err := r.db.Transaction(ctx, stmts)
	return wrap("mcp_calls.record", err)
}

func (r *mcpCallRepo) List(ctx context.Context, q store.MCPCallQuery) ([]store.MCPCall, error) {
	var conds []string
	var args []any
	if q.ClientKey != "" {
		conds = append(conds, `client_key = ?`)
		args = append(args, q.ClientKey)
	}
	if q.Tool != "" {
		conds = append(conds, `tool = ?`)
		args = append(args, q.Tool)
	}
	if q.Result != "" {
		conds = append(conds, `result = ?`)
		args = append(args, q.Result)
	}
	sql := `SELECT id, at_ms, client_key, client_name, tool, scope, item_count, result, duration_ms, bytes FROM mcp_calls`
	if len(conds) > 0 {
		sql += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	sql += ` ORDER BY at_ms DESC, id DESC`
	sql, args = page(sql, args, q.Limit, q.Offset)

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, wrap("mcp_calls.list", err)
	}
	out := make([]store.MCPCall, 0, len(rows.Values))
	for _, row := range rows.Values {
		out = append(out, store.MCPCall{
			ID:         asInt(row[0]),
			At:         fromMS(row[1]),
			ClientKey:  asString(row[2]),
			ClientName: asString(row[3]),
			Tool:       asString(row[4]),
			Scope:      asString(row[5]),
			Count:      int(asInt(row[6])),
			Result:     asString(row[7]),
			DurationMS: asInt(row[8]),
			Bytes:      int(asInt(row[9])),
		})
	}
	return out, nil
}

func (r *mcpCallRepo) Clear(ctx context.Context) error {
	_, err := r.db.Execute(ctx, `DELETE FROM mcp_calls`)
	return wrap("mcp_calls.clear", err)
}
