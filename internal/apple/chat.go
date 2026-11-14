package apple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hehehai/VoxtMini/internal/core"
	"github.com/hehehai/VoxtMini/internal/helper/proto"
	"github.com/hehehai/VoxtMini/internal/llm"
)

// Window is Apple Intelligence's context: the prompt and the answer
// together, in tokens.
const Window = 4096

// MaxAnswer bounds an answer's tokens when the request names none.
const MaxAnswer = 2048

// Chat is Apple Intelligence as an llm.Backend: one apple.chat per
// request, its deltas on the caller's goroutine. llm.Client wraps it
// (Services.LLM): JSON repair, thinking stripped.
type Chat struct {
	h      Caller
	params func() Params // the user's settings; nil: none
}

var _ llm.Backend = (*Chat)(nil)

func (*Chat) Info() llm.BackendInfo { return llm.BackendInfo{ID: IntelligenceID} }

// Budget is the answer's token budget for a prompt of promptTokens: the
// request's (want, 0 for none, then MaxAnswer), within what the window
// leaves; false when the prompt leaves no room for an answer.
func Budget(promptTokens, want int) (int, bool) {
	room := Window - promptTokens
	if room < 16 {
		return 0, false
	}
	if want <= 0 {
		want = MaxAnswer
	}
	return min(want, room), true
}

// Chat runs req as one answer.
func (c *Chat) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	if c.h == nil {
		return llm.Response{}, &llm.Error{Code: llm.CodeConfig, Params: map[string]string{"provider": IntelligenceID, "field": "helper"}, Err: ErrUnavailable}
	}
	var p proto.AppleChatParams
	prompt := 0
	for _, m := range req.Messages {
		p.Messages = append(p.Messages, proto.AppleMessage{Role: string(m.Role), Content: m.Content})
		prompt += llm.EstimateTokens(m.Content) + 4
	}
	if len(p.Messages) == 0 {
		return llm.Response{}, &llm.Error{Code: llm.CodeBadRequest, Params: map[string]string{"provider": IntelligenceID, "status": "0", "detail": "no messages"}}
	}
	want := req.MaxTokens
	t := llm.DefaultTemperature(req.Skill)
	if req.Temperature != nil {
		t = *req.Temperature
	}
	if c.params != nil {
		c.params().apply(&t, &want)
	}
	budget, ok := Budget(prompt, want)
	if !ok {
		return llm.Response{}, &llm.Error{Code: llm.CodeContextLength, Params: map[string]string{"provider": IntelligenceID, "model": IntelligenceID}}
	}
	p.MaxTokens = budget
	p.Temperature = &t

	start := time.Now()
	pump := &deltas{signal: make(chan struct{}, 1)}
	type result struct {
		raw json.RawMessage
		err error
	}
	done := make(chan result, 1)
	go func() {
		raw, err := c.h.Call(ctx, jobID(), proto.MethodAppleChat, p, pump.note)
		done <- result{raw, err}
	}()
	var res result
	for waiting := true; waiting; {
		select {
		case <-pump.signal:
			pump.deliver(req.OnDelta)
		case res = <-done:
			waiting = false
		}
	}
	pump.deliver(req.OnDelta)
	if res.err != nil {
		return llm.Response{}, chatErr(ctx, res.err)
	}
	var out proto.AppleChatResult
	if err := json.Unmarshal(res.raw, &out); err != nil {
		return llm.Response{}, &llm.Error{Code: llm.CodeBadResponse, Params: map[string]string{"provider": IntelligenceID, "detail": err.Error()}, Err: err}
	}
	finish := llm.FinishStop
	if out.Finish == "length" {
		finish = llm.FinishLength
	}
	return llm.Response{Text: out.Text, FinishReason: finish, Model: IntelligenceID, Provider: Provider,
		Usage:   llm.Usage{PromptTokens: prompt, CompletionTokens: llm.EstimateTokens(out.Text), TotalTokens: prompt + llm.EstimateTokens(out.Text), Estimated: true},
		Latency: time.Since(start)}, nil
}

// deltas hands the job's deltas (the helper's reader goroutine) to the
// caller's goroutine, in order.
type deltas struct {
	mu      sync.Mutex
	pending []string
	signal  chan struct{}
}

func (d *deltas) note(m proto.Message) {
	var p proto.AppleDeltaParams
	if m.Method != proto.MethodAppleDelta || m.DecodeParams(&p) != nil || p.Text == "" {
		return
	}
	d.mu.Lock()
	d.pending = append(d.pending, p.Text)
	d.mu.Unlock()
	select {
	case d.signal <- struct{}{}:
	default:
	}
}

func (d *deltas) deliver(fn func(string)) {
	d.mu.Lock()
	out := d.pending
	d.pending = nil
	d.mu.Unlock()
	if fn == nil {
		return
	}
	for _, s := range out {
		fn(s)
	}
}

// chatErr is a helper failure as llm's error.
func chatErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err() // llm.Client tells a timeout from a cancel
	}
	params := map[string]string{"provider": IntelligenceID}
	var pe *proto.Error
	if errors.As(err, &pe) {
		switch pe.Code {
		case proto.CodeContextLength:
			params["model"] = IntelligenceID
			return &llm.Error{Code: llm.CodeContextLength, Params: params, Err: pe}
		case proto.CodeRefused:
			return &llm.Error{Code: llm.CodeFiltered, Params: params, Err: fmt.Errorf("%w: %w", ErrRefused, pe)}
		case proto.CodeUnsupported, proto.CodeMethodNotFound:
			params["feature"] = capability(pe)
			if capability(pe) == proto.CapabilityLocale {
				return &llm.Error{Code: llm.CodeUnsupported, Params: params, Err: fmt.Errorf("%w: %w", ErrLanguage, pe)}
			}
			return &llm.Error{Code: llm.CodeUnsupported, Params: params, Err: fmt.Errorf("%w: %w", ErrUnavailable, pe)}
		case proto.CodeBusy:
			return &llm.Error{Code: llm.CodeRateLimited, Params: params, Err: pe}
		case proto.CodeCancelled:
			return &llm.Error{Code: llm.CodeCancelled, Err: pe}
		case proto.CodeBadRequest:
			params["status"], params["detail"] = "0", pe.Message
			return &llm.Error{Code: llm.CodeBadRequest, Params: params, Err: pe}
		}
		params["status"] = "0"
		return &llm.Error{Code: llm.CodeServer, Params: params, Err: pe}
	}
	params["status"] = "0"
	return &llm.Error{Code: llm.CodeServer, Params: params, Retryable: errors.Is(err, core.ErrHelper), Err: err}
}
