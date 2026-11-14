package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// rsText is a finished Responses API response: a hosted search call, then
// the message.
func rsText(text string) map[string]any {
	return map[string]any{
		"id": "resp_1", "model": "m-reported", "status": "completed",
		"output": []any{
			map[string]any{"type": "web_search_call", "id": "ws_1", "status": "completed"},
			map[string]any{"type": "message", "role": "assistant", "content": []any{
				map[string]any{"type": "output_text", "text": text, "annotations": []any{}},
			}},
		},
		"usage": map[string]any{"input_tokens": 11, "output_tokens": 7, "total_tokens": 18},
	}
}

func ev(v map[string]any) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", v["type"], b)
}

func rsStream(deltas ...string) []string {
	out := []string{ev(map[string]any{"type": "response.created", "response": map[string]any{"model": "m-s", "status": "in_progress"}})}
	for _, d := range deltas {
		out = append(out, ev(map[string]any{"type": "response.output_text.delta", "delta": d}))
	}
	return append(out, ev(map[string]any{"type": "response.completed", "response": map[string]any{
		"model": "m-s", "status": "completed",
		"usage": map[string]any{"input_tokens": 5, "output_tokens": 2, "total_tokens": 7}}}))
}

func rsBackend(t *testing.T, f *fake, cfg ProviderConfig) Backend {
	t.Helper()
	if cfg.ID == "" {
		cfg.ID = "openai"
	}
	if cfg.API == "" {
		cfg.API = APIResponses
	}
	cfg.BaseURL = f.srv.URL
	b, err := NewBackend(cfg, f.srv.Client(), StaticSecrets{"llm.openai": "sk-test", "llm.dashscope": "dk", "llm.volcengine": "vk", "llm.xai": "xk"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestResponsesNonStreaming(t *testing.T) {
	f := newFake(t, jsonReply(200, rsText("hola")))
	b := rsBackend(t, f, ProviderConfig{})
	if in := b.Info(); in.Transport != TransportResponses || in.ID != "openai" {
		t.Fatalf("info %+v", in)
	}
	seed := int64(3)
	resp, err := (&Client{Backend: b}).Chat(context.Background(), Request{
		Model:       "gpt-x",
		Messages:    []Message{{Role: System, Content: "sys"}, {Role: User, Content: "hello"}, {Role: Assistant, Content: "hi"}, {Role: User, Content: "again"}},
		Temperature: F64(0.2), MaxTokens: 8, Stop: []string{"\n"}, Seed: &seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "hola" || resp.Model != "m-reported" || resp.FinishReason != FinishStop || resp.Usage != (Usage{11, 7, 18, false}) {
		t.Fatalf("resp %+v", resp)
	}
	r := f.requests()[0]
	if r.Path != "/responses" || r.Header.Get("Authorization") != "Bearer sk-test" {
		t.Fatalf("req %+v", r)
	}
	want := `{"input":[{"content":"hello","role":"user"},{"content":"hi","role":"assistant"},{"content":"again","role":"user"}],"instructions":"sys","max_output_tokens":16,"model":"gpt-x","store":false,"stream":false,"temperature":0.2}`
	if got, _ := json.Marshal(r.Body); string(got) != want {
		t.Fatalf("body\n%s\nwant\n%s", got, want)
	}
}

func TestResponsesStreaming(t *testing.T) {
	f := newFake(t, sse(rsStream("Hel", "lo")...))
	var got []string
	resp, err := rsBackend(t, f, ProviderConfig{}).Chat(context.Background(), Request{Messages: msgs("x"), OnDelta: func(d string) { got = append(got, d) }})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Hello" || strings.Join(got, "|") != "Hel|lo" || resp.Model != "m-s" || resp.Usage != (Usage{5, 2, 7, false}) || resp.FinishReason != FinishStop {
		t.Fatalf("resp %+v deltas %q", resp, got)
	}
	if body := f.requests()[0].Body; body["stream"] != true || body["model"] != "gpt-4.1-mini" {
		t.Fatalf("body %v", body)
	}
	// No deltas, only the completed response's text: sent as one delta.
	f2 := newFake(t, sse(ev(map[string]any{"type": "response.completed", "response": rsText("whole")})))
	got = nil
	resp, err = rsBackend(t, f2, ProviderConfig{}).Chat(context.Background(), Request{Messages: msgs("x"), OnDelta: func(d string) { got = append(got, d) }})
	if err != nil || resp.Text != "whole" || len(got) != 1 {
		t.Fatalf("%v %+v %q", err, resp, got)
	}
}

func TestResponsesIncomplete(t *testing.T) {
	inc := rsText("cut")
	inc["status"], inc["incomplete_details"] = "incomplete", map[string]any{"reason": "max_output_tokens"}
	f := newFake(t, jsonReply(200, inc))
	resp, err := rsBackend(t, f, ProviderConfig{}).Chat(context.Background(), Request{Messages: msgs("x"), MaxTokens: 100})
	if err != nil || resp.FinishReason != FinishLength || resp.Text != "cut" {
		t.Fatalf("%v %+v", err, resp)
	}
	f2 := newFake(t, sse(ev(map[string]any{"type": "response.output_text.delta", "delta": "cu"}),
		ev(map[string]any{"type": "response.incomplete", "response": inc})))
	resp, err = rsBackend(t, f2, ProviderConfig{}).Chat(context.Background(), Request{Messages: msgs("x"), OnDelta: func(string) {}})
	if err != nil || resp.FinishReason != FinishLength || resp.Text != "cu" {
		t.Fatalf("%v %+v", err, resp)
	}
	// Probe accepts an answer cut short.
	f3 := newFake(t, jsonReply(200, inc))
	if _, err := Probe(context.Background(), rsBackend(t, f3, ProviderConfig{}), ""); err != nil {
		t.Fatal(err)
	}
	if f3.requests()[0].Body["max_output_tokens"] != float64(minOutputTokens) {
		t.Fatalf("probe body %v", f3.requests()[0].Body)
	}
}

func TestResponsesErrors(t *testing.T) {
	failed := func(code, msg string) string {
		return ev(map[string]any{"type": "response.failed", "response": map[string]any{"status": "failed", "error": map[string]any{"code": code, "message": msg}}})
	}
	for _, c := range []struct {
		name string
		h    http.HandlerFunc
		code string
	}{
		{"failed event", sse(failed("rate_limit_exceeded", "slow down")), CodeRateLimited},
		{"failed server", sse(failed("server_error", "oops")), CodeServer},
		{"error event", sse(ev(map[string]any{"type": "error", "code": "context_length_exceeded", "message": "too long"})), CodeContextLength},
		{"error event, nested", sse(ev(map[string]any{"type": "error", "error": map[string]any{"type": "invalid_request_error", "message": "bad"}})), CodeBadRequest},
		{"truncated", sse(ev(map[string]any{"type": "response.output_text.delta", "delta": "a"})), CodeNetwork},
		{"failed body", jsonReply(200, map[string]any{"status": "failed", "error": map[string]any{"code": "server_error", "message": "x"}}), CodeServer},
		{"http 401", jsonReply(401, map[string]any{"error": map[string]any{"message": "bad key"}}), CodeAuth},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t, c.h)
			req := Request{Messages: msgs("x")}
			if !strings.HasSuffix(c.name, "body") && !strings.HasPrefix(c.name, "http") {
				req.OnDelta = func(string) {}
			}
			_, err := rsBackend(t, f, ProviderConfig{}).Chat(context.Background(), req)
			if Code(err) != c.code {
				t.Fatalf("err %v, want %s", err, c.code)
			}
		})
	}
}

func TestResponsesJSONFormats(t *testing.T) {
	schema := &Schema{Name: "out", Type: "object"}
	for _, c := range []struct {
		id, mode string
		schema   *Schema
		want     string // text.format.type, "" none
	}{
		{"openai", "", schema, "json_schema"},
		{"openai", "", nil, "json_object"},
		{"dashscope", "", schema, "json_object"},
		{"volcengine", "", schema, ""},
	} {
		f := newFake(t, jsonReply(200, rsText(`{"a":1}`)))
		_, err := rsBackend(t, f, ProviderConfig{ID: c.id}).Chat(context.Background(), Request{Messages: msgs("x"), Format: FormatJSON, Schema: c.schema})
		if err != nil {
			t.Fatal(err)
		}
		body := f.requests()[0].Body
		got := ""
		if tx, ok := body["text"].(map[string]any); ok {
			got, _ = tx["format"].(map[string]any)["type"].(string)
		}
		if got != c.want || !strings.Contains(body["instructions"].(string), "JSON") {
			t.Errorf("%s %v: format %q, want %q; %v", c.id, c.schema != nil, got, c.want, body)
		}
	}
}

func TestResponsesRelax(t *testing.T) {
	// ExtraBody (DashScope's enable_thinking) is merged, then dropped
	// when refused; store and temperature likewise.
	f := newFake(t,
		jsonReply(400, map[string]any{"error": map[string]any{"message": "Unsupported parameter: temperature"}}),
		jsonReply(400, map[string]any{"error": map[string]any{"message": "unknown field store"}}),
		jsonReply(400, map[string]any{"error": map[string]any{"message": "enable_thinking is not supported"}}),
		jsonReply(200, rsText("ok")))
	b := rsBackend(t, f, ProviderConfig{ID: "dashscope", ExtraBody: map[string]any{"x": 1}})
	if _, err := b.Chat(context.Background(), Request{Messages: msgs("x"), Temperature: F64(0.3)}); err != nil {
		t.Fatal(err)
	}
	rs := f.requests()
	if len(rs) != 4 {
		t.Fatalf("%d requests", len(rs))
	}
	if rs[0].Body["enable_thinking"] != false || rs[0].Body["x"] != float64(1) || rs[0].Body["temperature"] != 0.3 || rs[0].Body["store"] != false {
		t.Fatalf("first %v", rs[0].Body)
	}
	last := rs[3].Body
	if _, ok := last["temperature"]; ok {
		t.Error("temperature kept")
	}
	if _, ok := last["store"]; ok {
		t.Error("store kept")
	}
	if _, ok := last["enable_thinking"]; ok || last["model"] != "qwen-plus" {
		t.Errorf("last %v", last)
	}
}

func TestAPIChoice(t *testing.T) {
	for _, c := range []struct {
		cfg  ProviderConfig
		want string // transport, "" refused
	}{
		{ProviderConfig{ID: "openai"}, TransportOpenAI},
		{ProviderConfig{ID: "openai", API: APIChat}, TransportOpenAI},
		{ProviderConfig{ID: "openai", API: APIResponses}, TransportResponses},
		{ProviderConfig{ID: "volcengine", API: APIResponses}, TransportResponses},
		{ProviderConfig{ID: "dashscope-intl", API: APIResponses}, TransportResponses},
		{ProviderConfig{ID: "c2", Preset: "custom", BaseURL: "http://127.0.0.1:9/v1", API: APIResponses}, TransportResponses},
		{ProviderConfig{ID: "anthropic"}, TransportAnthropic},
		{ProviderConfig{ID: "deepseek", API: APIResponses}, ""},
		{ProviderConfig{ID: "anthropic", API: APIResponses}, ""},
		{ProviderConfig{ID: "openai", API: "assistants"}, ""},
	} {
		b, err := NewBackend(c.cfg, nil, nil)
		if c.want == "" {
			if Code(err) != CodeConfig {
				t.Errorf("%+v: %v", c.cfg, err)
			}
			continue
		}
		if err != nil || b.Info().Transport != c.want {
			t.Errorf("%+v: %v", c.cfg, err)
			continue
		}
		if _, ok := b.(ModelLister); !ok {
			t.Errorf("%+v: no Models", c.cfg)
		}
	}
}

func TestSearchSupported(t *testing.T) {
	p := func(id string) Preset { q, _ := LookupPreset(id); return q }
	for _, c := range []struct {
		id, api string
		want    bool
	}{
		{"openai", "", false}, {"openai", APIResponses, true},
		{"xai", APIResponses, true}, {"volcengine", APIResponses, true}, {"dashscope", APIResponses, true},
		{"anthropic", "", true}, {"zhipu", "", true}, {"zhipu", APIResponses, false},
		{"gemini", "", false}, {"deepseek", "", false}, {"custom", APIResponses, false},
	} {
		if got := SearchSupported(p(c.id), c.api); got != c.want {
			t.Errorf("%s %q: %v", c.id, c.api, got)
		}
	}
}

// TestWebSearchOnlyForRewrite: the search tool goes with rewrites only,
// and only when the setup turns it on.
func TestWebSearchOnlyForRewrite(t *testing.T) {
	tools := func(cfg ProviderConfig, skill Skill, reply http.HandlerFunc) any {
		f := newFake(t, reply)
		if cfg.BaseURL == "" {
			cfg.BaseURL = f.srv.URL
		}
		b, err := NewBackend(cfg, f.srv.Client(), StaticSecrets{"llm.openai": "k", "llm.zhipu": "z", "llm.anthropic": "a"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.Chat(context.Background(), Request{Messages: msgs("x"), Skill: skill}); err != nil {
			t.Fatal(err)
		}
		return f.requests()[0].Body["tools"]
	}
	rs := ProviderConfig{ID: "openai", API: APIResponses, WebSearch: true}
	if got, _ := json.Marshal(tools(rs, SkillRewrite, jsonReply(200, rsText("ok")))); string(got) != `[{"type":"web_search"}]` {
		t.Errorf("responses tools %s", got)
	}
	for _, s := range []Skill{SkillTranscribeEnhance, SkillTranslateText, ""} {
		if got := tools(rs, s, jsonReply(200, rsText("ok"))); got != nil {
			t.Errorf("%s: tools %v", s, got)
		}
	}
	rs.WebSearch = false
	if got := tools(rs, SkillRewrite, jsonReply(200, rsText("ok"))); got != nil {
		t.Errorf("search off: tools %v", got)
	}
	zh := ProviderConfig{ID: "zhipu", WebSearch: true}
	if got, _ := json.Marshal(tools(zh, SkillRewrite, jsonReply(200, oaText("ok")))); string(got) != `[{"type":"web_search","web_search":{"enable":"True","search_engine":"search_std"}}]` {
		t.Errorf("zhipu tools %s", got)
	}
	if got := tools(zh, SkillTranslateText, jsonReply(200, oaText("ok"))); got != nil {
		t.Errorf("zhipu translate tools %v", got)
	}
	// The OpenAI-compatible Gemini and other chat services never search.
	if got := tools(ProviderConfig{ID: "openai", WebSearch: true}, SkillRewrite, jsonReply(200, oaText("ok"))); got != nil {
		t.Errorf("openai chat tools %v", got)
	}
	an := ProviderConfig{ID: "anthropic", WebSearch: true}
	anReply := jsonReply(200, map[string]any{"model": "c", "content": []any{map[string]any{"type": "text", "text": "ok"}}, "stop_reason": "end_turn"})
	if got, _ := json.Marshal(tools(an, SkillRewrite, anReply)); string(got) != `[{"max_uses":5,"name":"web_search","type":"web_search_20250305"}]` {
		t.Errorf("anthropic tools %s", got)
	}
}

func TestAnthropicSearchBlocks(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "I'll search for that."},
		map[string]any{"type": "server_tool_use", "id": "srvtoolu_1", "name": "web_search", "input": map[string]any{"query": "q"}},
		map[string]any{"type": "web_search_tool_result", "tool_use_id": "srvtoolu_1", "content": []any{map[string]any{"type": "web_search_result", "url": "https://x.test", "title": "X"}}},
		map[string]any{"type": "text", "text": "The answer ", "citations": []any{map[string]any{"type": "web_search_result_location", "url": "https://x.test"}}},
		map[string]any{"type": "text", "text": "is 42."},
	}
	f := newFake(t, jsonReply(200, map[string]any{"model": "c", "content": content, "stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 3, "output_tokens": 4}}))
	p, _ := LookupPreset("anthropic")
	p.BaseURL = f.srv.URL
	resp, err := NewAnthropic(p, f.srv.Client(), keys()).Chat(context.Background(), Request{Messages: msgs("x")})
	if err != nil || resp.Text != "The answer is 42." {
		t.Fatalf("%v %q", err, resp.Text)
	}
	data := func(v map[string]any) string { b, _ := json.Marshal(v); return "data: " + string(b) + "\n\n" }
	delta := func(i int, s string) string {
		return data(map[string]any{"type": "content_block_delta", "index": i, "delta": map[string]any{"type": "text_delta", "text": s}})
	}
	f2 := newFake(t, sse(
		data(map[string]any{"type": "message_start", "message": map[string]any{"model": "c", "usage": map[string]any{"input_tokens": 3}}}),
		data(map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}),
		delta(0, "Searching."),
		data(map[string]any{"type": "content_block_start", "index": 1, "content_block": map[string]any{"type": "server_tool_use", "id": "s", "name": "web_search"}}),
		data(map[string]any{"type": "content_block_delta", "index": 1, "delta": map[string]any{"type": "input_json_delta", "partial_json": `{"query":"q"}`}}),
		data(map[string]any{"type": "content_block_start", "index": 2, "content_block": map[string]any{"type": "web_search_tool_result", "tool_use_id": "s", "content": []any{}}}),
		data(map[string]any{"type": "content_block_start", "index": 3, "content_block": map[string]any{"type": "text", "text": ""}}),
		delta(3, "Forty"), delta(3, "-two"),
		data(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 9}}),
		data(map[string]any{"type": "message_stop"})))
	p.BaseURL = f2.srv.URL
	var streamed strings.Builder
	resp, err = NewAnthropic(p, f2.srv.Client(), keys()).Chat(context.Background(), Request{Messages: msgs("x"), OnDelta: func(s string) { streamed.WriteString(s) }})
	if err != nil || resp.Text != "Forty-two" || streamed.String() != "Searching.Forty-two" || resp.Usage.CompletionTokens != 9 {
		t.Fatalf("%v %+v streamed %q", err, resp, streamed.String())
	}
}

func TestResponsesFirstTokenTimeout(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	c := &Client{Backend: rsBackend(t, f, ProviderConfig{}), Sleep: noSleep}
	start := time.Now()
	_, err := c.Chat(context.Background(), Request{Messages: msgs("x"), FirstTokenTimeout: 100 * time.Millisecond, Timeout: 5 * time.Second})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeTimeout || e.Params["wait"] != WaitFirstToken || time.Since(start) > 3*time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
	if f.requests()[0].Body["stream"] != true {
		t.Error("a progress bound streams")
	}
}
