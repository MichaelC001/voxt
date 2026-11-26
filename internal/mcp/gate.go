package mcp

// User approval gate and per-agent rate limiter.

import (
	"context"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

// AskKind classifies a confirmation prompt shown to the user.
type AskKind string

const (
	// AskConnect asks whether a new agent may access VoxtMini.
	AskConnect AskKind = "connect"
	// AskManage asks whether an agent may modify dictionary, rules, or history.
	AskManage AskKind = "manage"
	// AskModel asks whether an agent may invoke a speech or language model.
	AskModel AskKind = "model"
	// AskTrigger asks whether an agent may trigger a recording session.
	AskTrigger AskKind = "trigger"
)

// Question is presented to the user through Options.Prompter.
type Question struct {
	Kind      AskKind
	Client    Client
	Tool      string
	Summary   string // short human-readable description (no raw secrets)
	Count     int    // number of items affected, if applicable
	EveryTime bool   // true for destructive actions that cannot use "always"
}

// Answer is the user's decision on a Question.
type Answer int

const (
	Unanswered Answer = iota
	Deny
	AllowOnce
	AllowMinute
	AllowAlways
)

const (
	refuseFor  = 10 * time.Minute
	quietFor   = 3 * time.Minute
	minuteSpan = time.Minute
	ratePerMin = 120
	rateBurst  = 60
)

type gate struct {
	s       *Server
	mu      sync.Mutex
	asking  map[string]*pending
	refused map[string]time.Time
	quiet   map[string]time.Time
	lines   map[string]*sync.Mutex
	minute  map[string]time.Time
	always  map[string]bool
}

type pending struct {
	done   chan struct{}
	answer Answer
}

func newGate(s *Server) *gate {
	return &gate{
		s:       s,
		asking:  map[string]*pending{},
		refused: map[string]time.Time{},
		quiet:   map[string]time.Time{},
		lines:   map[string]*sync.Mutex{},
		minute:  map[string]time.Time{},
		always:  map[string]bool{},
	}
}

func gateKey(kind AskKind, clientKey string) string {
	return string(kind) + "\x00" + clientKey
}

// connect ensures the agent has been approved for first-time access.
// Concurrent calls from the same agent coalesce onto a single prompt.
func (g *gate) connect(c *call) error {
	cl := c.client
	if cl.Self {
		return nil
	}
	if grant, ok := c.policy.Client(cl.Key); ok && grant.Allowed {
		return nil
	}
	key := gateKey(AskConnect, cl.Key)
	now := g.s.opt.Now()
	g.mu.Lock()
	if g.always[key] {
		g.mu.Unlock()
		return nil
	}
	if until, ok := g.refused[key]; ok && now.Before(until) {
		g.mu.Unlock()
		return declined()
	}
	if until, ok := g.quiet[key]; ok && now.Before(until) {
		g.mu.Unlock()
		return fail("not_confirmed", "the user did not answer VoxtMini's prompt; ask them to check VoxtMini and try again later")
	}
	p := g.asking[key]
	if p == nil {
		p = &pending{done: make(chan struct{})}
		g.asking[key] = p
		go g.askConnect(key, cl, p, c.req)
	}
	g.mu.Unlock()

	stop := waiting(c.ctx, c.req, "Waiting for the user to allow this agent in VoxtMini…")
	defer stop()
	select {
	case <-p.done:
	case <-c.ctx.Done():
		return notConfirmed()
	}
	switch p.answer {
	case Deny:
		return declined()
	case Unanswered:
		return notConfirmed()
	}
	return nil
}

func (g *gate) askConnect(key string, cl Client, p *pending, req *sdk.CallToolRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), g.s.opt.AskTimeout)
	defer cancel()
	q := Question{Kind: AskConnect, Client: cl}
	if req != nil && req.Params != nil {
		q.Tool = req.Params.Name
	}
	a := g.ask(ctx, q)
	g.mu.Lock()
	switch a {
	case Deny:
		g.refused[key] = g.s.opt.Now().Add(refuseFor)
	case Unanswered:
		g.quiet[key] = g.s.opt.Now().Add(quietFor)
	case AllowAlways:
		g.always[key] = true
	}
	delete(g.asking, key)
	g.mu.Unlock()
	p.answer = a
	close(p.done)
}

// confirm prompts for write, model, or trigger calls unless already
// covered by a persisted grant, an in-memory AllowAlways, or a 1-minute lease.
func (g *gate) confirm(c *call, kind AskKind, summary string, count int, everyTime bool) error {
	cl := c.client
	if cl.Self {
		return fail("scope_off", "self-test does not run write, model, or trigger tools")
	}
	l := g.line(cl.Key)
	l.Lock()
	defer l.Unlock()

	key := gateKey(kind, cl.Key)
	now := g.s.opt.Now()

	if !everyTime {
		if g.policyAllows(c.policy, cl.Key, kind) {
			return nil
		}
		g.mu.Lock()
		allowed := g.always[key] || now.Before(g.minute[key])
		g.mu.Unlock()
		if allowed {
			return nil
		}
	}

	q := Question{
		Kind:      kind,
		Client:    cl,
		Tool:      c.toolName(),
		Summary:   summary,
		Count:     count,
		EveryTime: everyTime,
	}
	ctx, cancel := context.WithTimeout(c.ctx, g.s.opt.AskTimeout)
	defer cancel()
	stop := waiting(ctx, c.req, "Waiting for the user to confirm this action in VoxtMini…")
	a := g.ask(ctx, q)
	stop()

	switch a {
	case Deny:
		return declined()
	case Unanswered:
		return notConfirmed()
	case AllowMinute:
		if !everyTime {
			g.mu.Lock()
			g.minute[key] = g.s.opt.Now().Add(minuteSpan)
			g.mu.Unlock()
		}
	case AllowAlways:
		if !everyTime {
			g.mu.Lock()
			g.always[key] = true
			g.mu.Unlock()
		}
	}
	return nil
}

func (g *gate) ask(ctx context.Context, q Question) Answer {
	if g.s.opt.Prompter == nil {
		return Deny
	}
	return g.s.opt.Prompter.Ask(ctx, q)
}

func (g *gate) policyAllows(p Policy, clientKey string, kind AskKind) bool {
	grant, ok := p.Client(clientKey)
	if !ok {
		return false
	}
	switch kind {
	case AskManage:
		return grant.AlwaysManage
	case AskModel:
		return grant.AlwaysModels
	case AskTrigger:
		return grant.AlwaysTrigger
	}
	return false
}

func (g *gate) line(clientKey string) *sync.Mutex {
	g.mu.Lock()
	defer g.mu.Unlock()
	l := g.lines[clientKey]
	if l == nil {
		l = &sync.Mutex{}
		g.lines[clientKey] = l
	}
	return l
}

// Forget clears all remembered decisions for clientKey and disconnects it.
func (s *Server) Forget(clientKey string) {
	s.gate.mu.Lock()
	for _, k := range []AskKind{AskConnect, AskManage, AskModel, AskTrigger} {
		gk := gateKey(k, clientKey)
		delete(s.gate.refused, gk)
		delete(s.gate.quiet, gk)
		delete(s.gate.minute, gk)
		delete(s.gate.always, gk)
	}
	s.gate.mu.Unlock()
	s.Disconnect(clientKey)
}

// AskAgain clears write/model/trigger leases for clientKey without disconnecting.
func (s *Server) AskAgain(clientKey string) {
	s.gate.mu.Lock()
	for _, k := range []AskKind{AskManage, AskModel, AskTrigger} {
		gk := gateKey(k, clientKey)
		delete(s.gate.minute, gk)
		delete(s.gate.always, gk)
	}
	s.gate.mu.Unlock()
}

func declined() error {
	return fail("user_declined", "the user declined this action in VoxtMini: do not retry unless asked")
}

func notConfirmed() error {
	return fail("not_confirmed", "the user did not answer in VoxtMini in time: ask them to check VoxtMini")
}

// waiting sends periodic MCP progress notifications while waiting on the user.
func waiting(ctx context.Context, req *sdk.CallToolRequest, msg string) (stop func()) {
	if req == nil || req.Params == nil || req.Session == nil {
		return func() {}
	}
	token := req.Params.GetProgressToken()
	if token == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		n := 0.0
		for {
			_ = req.Session.NotifyProgress(ctx, &sdk.ProgressNotificationParams{
				ProgressToken: token,
				Progress:      n,
				Message:       msg,
			})
			n++
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return func() { close(done) }
}

// limiter enforces 120 calls/min per agent key.
type limiter struct {
	now func() time.Time
	mu  sync.Mutex
	by  map[string]*rate.Limiter
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, by: map[string]*rate.Limiter{}}
}

func (l *limiter) allow(key string) (wait time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim := l.by[key]
	if lim == nil {
		lim = rate.NewLimiter(rate.Every(time.Minute/ratePerMin), rateBurst)
		l.by[key] = lim
	}
	now := l.now()
	r := lim.ReserveN(now, 1)
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return d.Round(time.Second) + time.Second, false
	}
	return 0, true
}
