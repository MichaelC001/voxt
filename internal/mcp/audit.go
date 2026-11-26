package mcp

// Content-free audit log for MCP calls and refused connections.
// Records never include user text, audio content, prompts, or API keys.

import (
	"context"
	"encoding/json"
	"log"
	"slices"
	"sync"
	"time"
)

// Refused is the audit Result for a connection VoxtMini declined.
const Refused = "refused"

// CallRecord is one audited MCP call or refused connection.
type CallRecord struct {
	At         time.Time `json:"at"`
	ClientKey  string    `json:"clientKey"`
	ClientName string    `json:"clientName"`
	Tool       string    `json:"tool"`
	Scope      string    `json:"scope"`
	Count      int       `json:"count,omitempty"`
	Result     string    `json:"result"` // "ok", "refused", or error code
	DurationMS int64     `json:"durationMs"`
	Bytes      int       `json:"bytes"`
}

// Audit records MCP calls. Implementations must never persist user text,
// audio, prompts, or credentials.
type Audit interface {
	Record(ctx context.Context, rec CallRecord)
}

// Docs is the minimal document persistence contract for DocAudit
// (compatible with store.Documents).
type Docs interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Put(ctx context.Context, key string, doc []byte) error
}

const (
	auditDocKey = "mcp:audit"
	auditMax    = 2000
	auditKeep   = 30 * 24 * time.Hour
	auditDelay  = 2 * time.Second
)

// DocAudit is an Audit implementation backed by Docs (or memory when docs is nil).
type DocAudit struct {
	docs Docs
	now  func() time.Time

	mu      sync.Mutex
	entries []CallRecord // oldest first
	timer   *time.Timer
}

// OpenDocAudit loads persisted audit entries from docs (nil for memory-only).
func OpenDocAudit(docs Docs, now func() time.Time) *DocAudit {
	if now == nil {
		now = time.Now
	}
	a := &DocAudit{docs: docs, now: now}
	if docs != nil {
		if b, ok, err := docs.Get(context.Background(), auditDocKey); err == nil && ok {
			if err := json.Unmarshal(b, &a.entries); err != nil {
				log.Printf("mcp: audit log unreadable, starting fresh: %v", err)
				a.entries = nil
			}
		}
	}
	a.trim()
	return a
}

// Record appends rec to the audit log.
func (a *DocAudit) Record(_ context.Context, rec CallRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, rec)
	a.trim()
	a.scheduleFlush()
}

// Entries returns all kept records, newest first.
func (a *DocAudit) Entries() []CallRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := slices.Clone(a.entries)
	slices.Reverse(out)
	return out
}

// Clear removes all audit records and flushes immediately.
func (a *DocAudit) Clear() {
	a.mu.Lock()
	a.entries = nil
	a.mu.Unlock()
	a.Flush()
}

// Stats returns today's count, total count, and latest record for clientKey.
func (a *DocAudit) Stats(clientKey string) (today, total int, last CallRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	y, m, d := a.now().Date()
	for _, e := range a.entries {
		if e.ClientKey != clientKey || e.Result == Refused {
			continue
		}
		total++
		if ey, em, ed := e.At.Date(); ey == y && em == m && ed == d {
			today++
		}
		last = e
	}
	return today, total, last
}

// Flush persists the current entries immediately.
func (a *DocAudit) Flush() {
	a.mu.Lock()
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
	b, err := json.Marshal(a.entries)
	a.mu.Unlock()
	if err != nil || a.docs == nil {
		return
	}
	if err := a.docs.Put(context.Background(), auditDocKey, b); err != nil {
		log.Printf("mcp: audit flush: %v", err)
	}
}

func (a *DocAudit) scheduleFlush() {
	if a.timer != nil || a.docs == nil {
		return
	}
	a.timer = time.AfterFunc(auditDelay, a.Flush)
}

func (a *DocAudit) trim() {
	cut := a.now().Add(-auditKeep)
	i := 0
	for i < len(a.entries) && a.entries[i].At.Before(cut) {
		i++
	}
	if n := len(a.entries) - i; n > auditMax {
		i += n - auditMax
	}
	if i > 0 {
		a.entries = slices.Clone(a.entries[i:])
	}
}
