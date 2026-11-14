package llm

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// respObject is a Responses API response (non-streamed, or the response
// in a stream's response.* event).
type respObject struct {
	Model  string `json:"model"`
	Status string `json:"status"` // completed | incomplete | failed | in_progress
	Output []struct {
		Type    string `json:"type"` // message; web_search_call, reasoning…: ignored
		Content []struct {
			Type    string `json:"type"` // output_text | refusal
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"output"`
	Usage             *respUsage `json:"usage"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Error *respError `json:"error"`
}

type respUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (u *respUsage) usage() Usage {
	if u == nil {
		return Usage{}
	}
	t := u.TotalTokens
	if t == 0 {
		t = u.InputTokens + u.OutputTokens
	}
	return Usage{PromptTokens: u.InputTokens, CompletionTokens: u.OutputTokens, TotalTokens: t}
}

type respError struct {
	Code    string `json:"code"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// err maps a Responses error (a failed response, an error event) to an
// *Error by its code, as the HTTP status would have.
func (e *respError) err(provider, model string) *Error {
	code := e.Code
	if code == "" {
		code = e.Type
	}
	status := 500
	low := strings.ToLower(code)
	switch {
	case strings.Contains(low, "rate_limit"), strings.Contains(low, "usage_limit"), strings.Contains(low, "quota"):
		status = 429
	case strings.Contains(low, "auth"), strings.Contains(low, "api_key"):
		status = 401
	case strings.Contains(low, "context_length"), strings.Contains(low, "invalid"):
		status = 400
	case strings.Contains(low, "model_not_found"):
		status = 404
	}
	detail := e.Message
	if code != "" {
		detail = code + ": " + e.Message
	}
	return HTTPError(provider, model, status, detail, 0)
}

// result reads a finished response: its text (the message items' output
// text; hosted tools' items are skipped), finish reason and usage.
func (o *respObject) result() Response {
	var text strings.Builder
	refused := false
	for _, item := range o.Output {
		if item.Type != "message" {
			continue
		}
		for _, c := range item.Content {
			switch c.Type {
			case "output_text":
				text.WriteString(c.Text)
			case "refusal":
				refused = refused || c.Refusal != ""
			}
		}
	}
	out := Response{Text: text.String(), Model: o.Model, Usage: o.Usage.usage(), FinishReason: FinishStop}
	if o.Status == "incomplete" {
		reason := ""
		if o.IncompleteDetails != nil {
			reason = o.IncompleteDetails.Reason
		}
		switch reason {
		case "max_output_tokens":
			out.FinishReason = FinishLength
		case "content_filter":
			out.FinishReason = FinishFilter
		default:
			out.FinishReason = FinishOther
		}
	}
	if out.Text == "" && refused {
		out.FinishReason = FinishFilter
	}
	return out
}

func (r *Responses) readOne(ctx context.Context, body io.Reader, timeout time.Duration) (Response, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxBody))
	if err != nil {
		return Response{}, ctxErr(ctx, r.preset.ID, err, timeout)
	}
	var o respObject
	if err := json.Unmarshal(raw, &o); err != nil {
		e := newErr(CodeBadResponse, "provider", r.preset.ID, "detail", clip(string(raw), 200))
		e.Err = err
		return Response{}, e
	}
	if o.Error != nil && (o.Error.Message != "" || o.Error.Code != "") || o.Status == "failed" {
		if o.Error == nil {
			o.Error = &respError{Code: "failed"}
		}
		return Response{}, o.Error.err(r.preset.ID, o.Model)
	}
	return o.result(), nil
}

// readStream reads the Responses API's events: text deltas
// (response.output_text.delta) go to onDelta; response.completed or
// response.incomplete ends it with the usage; response.failed and error
// events are errors. Other events (hosted tools' progress, reasoning) are
// skipped. A server that sends no deltas but a completed response with
// text has it sent as one delta.
func (r *Responses) readStream(ctx context.Context, body io.Reader, onDelta func(string), timeout time.Duration) (Response, error) {
	var out Response
	var text strings.Builder
	var perr error
	done := false
	err := readSSE(body, func(ev sseEvent) bool {
		if ev.Data == "[DONE]" {
			return false
		}
		var e struct {
			Type     string      `json:"type"`
			Delta    string      `json:"delta"`
			Response *respObject `json:"response"`
			Error    *respError  `json:"error"`
			Code     string      `json:"code"`
			Message  string      `json:"message"`
		}
		if err := json.Unmarshal([]byte(ev.Data), &e); err != nil {
			x := newErr(CodeBadResponse, "provider", r.preset.ID, "detail", clip(ev.Data, 200))
			x.Err = err
			perr = x
			return false
		}
		if e.Type == "" {
			e.Type = ev.Event
		}
		if e.Response != nil && e.Response.Model != "" {
			out.Model = e.Response.Model
		}
		switch e.Type {
		case "response.output_text.delta":
			if e.Delta != "" {
				text.WriteString(e.Delta)
				onDelta(e.Delta)
			}
		case "response.completed", "response.incomplete":
			if e.Response != nil {
				res := e.Response.result()
				out.FinishReason, out.Usage = res.FinishReason, res.Usage
				if text.Len() == 0 && res.Text != "" {
					text.WriteString(res.Text)
					onDelta(res.Text)
				}
			} else {
				out.FinishReason = FinishStop
			}
			done = true
			return false
		case "response.failed", "error":
			re := e.Error
			if re == nil && e.Response != nil {
				re = e.Response.Error
			}
			if re == nil {
				re = &respError{Code: e.Code, Message: e.Message}
			}
			if re.Code == "" && re.Type == "" && re.Message == "" {
				re.Code = "failed"
			}
			perr = re.err(r.preset.ID, out.Model)
			return false
		}
		return true
	})
	if perr != nil {
		return Response{}, perr
	}
	if err != nil || ctx.Err() != nil {
		if err == nil {
			err = ctx.Err()
		}
		return Response{}, ctxErr(ctx, r.preset.ID, err, timeout)
	}
	if !done {
		e := newErr(CodeNetwork, "provider", r.preset.ID, "detail", "stream ended early")
		e.Retryable = true
		return Response{}, e
	}
	out.Text = text.String()
	return out, nil
}
