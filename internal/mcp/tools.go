package mcp

// Tool registry, dynamic rescope, common middleware, and server instructions.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Scope gates a group of MCP tools behind Policy flags.
type Scope string

const (
	ScopeRead    Scope = "read"
	ScopeManage  Scope = "manage"
	ScopeModels  Scope = "models"
	ScopeTrigger Scope = "trigger"
)

func (sc Scope) allowed(p Policy) bool {
	if !p.Enabled {
		return false
	}
	switch sc {
	case ScopeRead:
		return true
	case ScopeManage:
		return p.AllowManage
	case ScopeModels:
		return p.AllowLocalModels
	case ScopeTrigger:
		return p.AllowTrigger
	}
	return false
}

type tool struct {
	name, desc  string
	scope       Scope
	readOnly    bool
	destructive bool
	openWorld   bool
	schema      *jsonschema.Schema
	run         func(c *call, args json.RawMessage) (any, error)
}

func def[In any](name, desc string, sc Scope, run func(*call, In) (any, error)) tool {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("mcp: %s: %v", name, err))
	}
	return tool{
		name:      name,
		desc:      desc,
		scope:     sc,
		readOnly:  sc == ScopeRead,
		openWorld: sc == ScopeModels,
		schema:    schema,
		run: func(c *call, raw json.RawMessage) (any, error) {
			var in In
			if len(raw) > 0 && string(raw) != "null" {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, fail("bad_arguments", "invalid arguments: "+err.Error())
				}
			}
			return run(c, in)
		},
	}
}

func toolset() []tool {
	return slices.Concat(readTools(), manageTools(), modelTools(), triggerTools())
}

type toolError struct {
	Code       string `json:"error"`
	Message    string `json:"message"`
	RetryAfter int    `json:"retryAfter,omitempty"`
}

func (e *toolError) Error() string { return e.Code + ": " + e.Message }

func fail(code, msg string) error { return &toolError{Code: code, Message: msg} }

func mapHostErr(err error) error {
	if err == nil {
		return nil
	}
	var te *toolError
	if errors.As(err, &te) {
		return te
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return fail("not_found", err.Error())
	case errors.Is(err, ErrBusy):
		return fail("busy", err.Error())
	case errors.Is(err, ErrRemoteOff):
		return fail("remote_off", err.Error())
	case errors.Is(err, ErrOff):
		return fail("off", err.Error())
	}
	return err
}

type call struct {
	ctx    context.Context
	srv    *Server
	req    *sdk.CallToolRequest
	client Client
	policy Policy
	count  int
}

func (c *call) toolName() string {
	if c.req != nil && c.req.Params != nil {
		return c.req.Params.Name
	}
	return ""
}

const maxResult = 512 << 10

func (s *Server) newSession(c *conn) *sdk.Server {
	srv := sdk.NewServer(&sdk.Implementation{
		Name:    "voxtmini",
		Title:   "VoxtMini",
		Version: s.opt.Version,
	}, &sdk.ServerOptions{
		Instructions: instructions,
		Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{ListChanged: true}},
	})
	c.mu.Lock()
	c.session = srv
	c.listed = map[string]bool{}
	c.mu.Unlock()
	s.rescope(c, s.opt.Host.Policy())
	return srv
}

// Rescope updates every live session's tool list after Policy changes.
func (s *Server) Rescope() {
	p := s.opt.Host.Policy()
	s.mu.Lock()
	cs := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		cs = append(cs, c)
	}
	s.mu.Unlock()
	for _, c := range cs {
		s.rescope(c, p)
	}
}

func (s *Server) rescope(c *conn, p Policy) {
	c.mu.Lock()
	srv, listed := c.session, c.listed
	c.mu.Unlock()
	if srv == nil {
		return
	}
	var gone []string
	for _, t := range toolset() {
		on := t.scope.allowed(p) || (c.who().Self && t.scope == ScopeRead)
		switch {
		case on && !listed[t.name]:
			srv.AddTool(sdkTool(t), s.handler(c, t))
			listed[t.name] = true
		case !on && listed[t.name]:
			gone = append(gone, t.name)
			delete(listed, t.name)
		}
	}
	if len(gone) > 0 {
		srv.RemoveTools(gone...)
	}
}

func sdkTool(t tool) *sdk.Tool {
	destructive, open := t.destructive, t.openWorld
	return &sdk.Tool{
		Name:        t.name,
		Description: t.desc,
		InputSchema: t.schema,
		Annotations: &sdk.ToolAnnotations{
			ReadOnlyHint:    t.readOnly,
			DestructiveHint: &destructive,
			OpenWorldHint:   &open,
		},
	}
}

func (s *Server) handler(c *conn, t tool) sdk.ToolHandler {
	return func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		start := s.opt.Now()
		cl := c.named(req)
		cc := &call{
			ctx:    ctx,
			srv:    s,
			req:    req,
			client: cl,
			policy: s.opt.Host.Policy(),
		}
		out, err := s.run(cc, t)
		var body []byte
		if err == nil {
			body, err = json.Marshal(out)
			if err == nil && len(body) > maxResult {
				err = fail("too_large", "response exceeds 512 KB; request fewer items")
			}
		}
		res := &sdk.CallToolResult{}
		result := "ok"
		if err != nil {
			var te *toolError
			if !errors.As(err, &te) {
				te = &toolError{Code: "failed", Message: err.Error()}
			}
			result = te.Code
			body, _ = json.Marshal(te)
			res.IsError = true
			res.StructuredContent = te
		} else {
			res.StructuredContent = json.RawMessage(body)
		}
		res.Content = []sdk.Content{&sdk.TextContent{Text: string(body)}}
		ms := s.opt.Now().Sub(start).Milliseconds()
		log.Printf("mcp: %s %s %s %d items %dms", cl.Display(), t.name, result, cc.count, ms)
		if s.opt.Audit != nil && !cl.Self {
			s.opt.Audit.Record(ctx, CallRecord{
				At:         start,
				ClientKey:  cl.Key,
				ClientName: cl.Display(),
				Tool:       t.name,
				Scope:      string(t.scope),
				Count:      cc.count,
				Result:     result,
				DurationMS: ms,
				Bytes:      len(body),
			})
			s.notifyChanged()
		}
		return res, nil
	}
}

func (s *Server) run(c *call, t tool) (any, error) {
	if err := s.opt.Host.Ready(); err != nil {
		return nil, fail("not_ready", err.Error())
	}
	if !c.policy.Enabled && !c.client.Self {
		return nil, fail("off", ErrOff.Error())
	}
	if !t.scope.allowed(c.policy) && !(c.client.Self && t.scope == ScopeRead) {
		return nil, fail("scope_off", scopeOffMsg(t.scope))
	}
	if wait, ok := s.lim.allow(c.client.Key); !ok {
		return nil, &toolError{
			Code:       "rate_limited",
			Message:    "too many calls; wait and try again",
			RetryAfter: max(1, int(wait/time.Second)),
		}
	}
	if err := s.gate.connect(c); err != nil {
		return nil, err
	}
	out, err := t.run(c, c.req.Params.Arguments)
	return out, mapHostErr(err)
}

func scopeOffMsg(sc Scope) string {
	switch sc {
	case ScopeManage:
		return "managing dictionary, rules, and history is turned off in VoxtMini Settings"
	case ScopeModels:
		return "calling local models via MCP is turned off in VoxtMini Settings"
	case ScopeTrigger:
		return "triggering recording sessions via MCP is turned off in VoxtMini Settings"
	}
	return ErrOff.Error()
}

func (c *conn) named(req *sdk.CallToolRequest) Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if info := req.ClientInfo(); info != nil && c.client.Name == "" {
		c.client.Name = clean(info.Name)
	}
	return c.client
}

// instructions is sent in MCP serverInfo; kept <= 1.5 KB with the first
// 512 characters self-contained for Codex.
const instructions = `VoxtMini is the user's local-first macOS voice input app (Transcribe, Translate, Rewrite, Dictionary, App enhancement). Start with voxtmini_overview to see active permissions, models, and microphones. Audio and text stay on this Mac unless the user explicitly enables remote models. Never search the web for VoxtMini or operate its windows with screen/GUI automation: when VoxtMini asks the user to approve you or enable a permission, tell them and wait. Check error codes (scope_off, remote_off, user_declined, busy) before retrying.`
