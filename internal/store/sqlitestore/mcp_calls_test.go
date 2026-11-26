package sqlitestore

import (
	"context"
	"testing"
	"time"

	"github.com/hehehai/VoxtMini/internal/store"
)

func TestMCPCallsCRUDAndFilters(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	entries := []store.MCPCall{
		{At: base, ClientKey: "c1", ClientName: "Codex", Tool: "voxtmini_overview", Scope: "read", Count: 1, Result: "ok", DurationMS: 12, Bytes: 400},
		{At: base.Add(time.Minute), ClientKey: "c2", ClientName: "Claude", Tool: "list_terms", Scope: "read", Count: 5, Result: "ok", DurationMS: 8, Bytes: 900},
		{At: base.Add(2 * time.Minute), ClientKey: "c1", ClientName: "Codex", Tool: "upsert_terms", Scope: "manage", Count: 2, Result: "scope_off", DurationMS: 3, Bytes: 80},
	}
	for _, e := range entries {
		if err := s.MCPCalls().Record(ctx, e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	all, err := s.MCPCalls().List(ctx, store.MCPCallQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d rows, want 3", len(all))
	}
	// Newest first.
	if all[0].Tool != "upsert_terms" || all[2].Tool != "voxtmini_overview" {
		t.Fatalf("order: %+v", all)
	}

	byClient, err := s.MCPCalls().List(ctx, store.MCPCallQuery{ClientKey: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byClient) != 2 {
		t.Fatalf("client filter: got %d, want 2", len(byClient))
	}

	byResult, err := s.MCPCalls().List(ctx, store.MCPCallQuery{Result: "scope_off"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byResult) != 1 || byResult[0].Tool != "upsert_terms" {
		t.Fatalf("result filter: %+v", byResult)
	}

	paged, err := s.MCPCalls().List(ctx, store.MCPCallQuery{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(paged) != 1 || paged[0].Tool != "list_terms" {
		t.Fatalf("paged: %+v", paged)
	}

	if err := s.MCPCalls().Clear(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := s.MCPCalls().List(ctx, store.MCPCallQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("after Clear: got %d rows", len(after))
	}
}

func TestMCPCallsRetention(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Cap at 5 rows for testing.
	s.mcpCalls.maxRows = 5

	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	// Insert an old row (31 days old).
	if err := s.MCPCalls().Record(ctx, store.MCPCall{
		At: now.AddDate(0, 0, -31), ClientKey: "old", ClientName: "Old", Tool: "old_tool", Scope: "read", Result: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	// Insert 7 fresh rows; old row should be pruned by age and total capped at 5.
	for i := range 7 {
		if err := s.MCPCalls().Record(ctx, store.MCPCall{
			At:         now.Add(time.Duration(i) * time.Second),
			ClientKey:  "c",
			ClientName: "Client",
			Tool:       "tool",
			Scope:      "read",
			Count:      i,
			Result:     "ok",
		}); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := s.MCPCalls().List(ctx, store.MCPCallQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}
	if rows[0].Count != 6 || rows[4].Count != 2 {
		t.Fatalf("unexpected kept counts: newest=%d oldest=%d", rows[0].Count, rows[4].Count)
	}
}
