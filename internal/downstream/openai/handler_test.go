package openai

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lhpqaq/all2api/internal/config"
	"github.com/lhpqaq/all2api/internal/core"
	"github.com/lhpqaq/all2api/internal/diag"
)

func TestExtractMessagesAndFlatten(t *testing.T) {
	t.Parallel()

	call := inboundToolCall{ID: "call_1", Type: "function"}
	call.Function.Name = "search"
	call.Function.Arguments = `{"q":"go"}`

	msgs := []chatMsg{
		{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "hello"},
				map[string]any{"type": "image", "image_url": "ignored"},
			},
		},
		{Role: "assistant", Content: "done", ToolCalls: []inboundToolCall{call}},
	}

	got := extractMessages(msgs)
	if len(got) != 2 {
		t.Fatalf("messages len = %d", len(got))
	}
	if got[0].Content != "hello\n" {
		t.Fatalf("flattened content = %q", got[0].Content)
	}
	if got[1].ToolCalls[0].Name != "search" {
		t.Fatalf("tool name = %q", got[1].ToolCalls[0].Name)
	}
	if got[1].ToolCalls[0].Args["q"] != "go" {
		t.Fatalf("tool args = %#v", got[1].ToolCalls[0].Args)
	}
	if flatten(map[string]any{"k": "v"}) != `{"k":"v"}` {
		t.Fatalf("unexpected fallback flatten output: %q", flatten(map[string]any{"k": "v"}))
	}
}

func TestExtractToolChoiceVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input any
		want  core.ToolChoice
	}{
		{name: "nil", input: nil, want: core.ToolChoice{Mode: "auto"}},
		{name: "required string", input: "required", want: core.ToolChoice{Mode: "any"}},
		{name: "named string", input: "lookup", want: core.ToolChoice{Mode: "tool", Name: "lookup"}},
		{name: "function map", input: map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}}, want: core.ToolChoice{Mode: "tool", Name: "lookup"}},
		{name: "tool map", input: map[string]any{"type": "tool", "name": "lookup"}, want: core.ToolChoice{Mode: "tool", Name: "lookup"}},
		{name: "any map", input: map[string]any{"type": "any"}, want: core.ToolChoice{Mode: "any"}},
		{name: "none map", input: map[string]any{"type": "none"}, want: core.ToolChoice{Mode: "auto"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractToolChoice(tt.input); got != tt.want {
				t.Fatalf("tool choice = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMergeToolChoiceUsesFunctionCallFallback(t *testing.T) {
	t.Parallel()

	got := mergeToolChoice("auto", map[string]any{"name": "lookup"})
	if got != (core.ToolChoice{Mode: "tool", Name: "lookup"}) {
		t.Fatalf("merged choice = %#v", got)
	}

	primary := mergeToolChoice("required", map[string]any{"name": "ignored"})
	if primary != (core.ToolChoice{Mode: "any"}) {
		t.Fatalf("expected primary choice to win, got %#v", primary)
	}
}

func TestResponsesToChatTransformsResponsesPayload(t *testing.T) {
	t.Parallel()

	body := map[string]any{
		"model":         "gpt-5",
		"stream":        true,
		"tool_choice":   "required",
		"function_call": map[string]any{"name": "lookup"},
		"instructions":  "system note",
		"input": []any{
			map[string]any{"role": "developer", "content": "dev note"},
			map[string]any{"role": "user", "content": "hello"},
			map[string]any{"type": "function_call_output", "output": "tool result", "call_id": "call_1"},
		},
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}}},
	}

	out := responsesToChat(body)
	messages, ok := out["messages"].([]any)
	if !ok {
		t.Fatalf("messages type = %T", out["messages"])
	}
	if len(messages) != 4 {
		t.Fatalf("messages len = %d", len(messages))
	}

	roles := make([]string, 0, len(messages))
	for _, raw := range messages {
		msg := raw.(map[string]any)
		roles = append(roles, msg["role"].(string))
	}
	if strings.Join(roles, ",") != "system,system,user,tool" {
		t.Fatalf("roles = %v", roles)
	}
	if out["stream"] != true {
		t.Fatalf("stream = %#v", out["stream"])
	}
	if out["tool_choice"] != "required" {
		t.Fatalf("tool_choice = %#v", out["tool_choice"])
	}
}

func TestWithDiagContextAndRequestIP(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-All2API-Debug", "false")
	req.Header.Set("X-Correlation-Id", "corr-1")
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.1")
	req.RemoteAddr = "10.0.0.2:1234"

	ctx := withDiagContext(req, config.Config{Logging: config.LoggingConfig{Debug: true}})
	if diag.Debug(ctx) {
		t.Fatal("expected request header to disable debug")
	}
	if diag.RequestID(ctx) != "corr-1" {
		t.Fatalf("request id = %q", diag.RequestID(ctx))
	}
	if requestIP(req) != "203.0.113.10" {
		t.Fatalf("request ip = %q", requestIP(req))
	}
}

func TestWriteChatNonStreamIncludesThinkingAndToolCalls(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	writeChatNonStream(rr, "gpt-test", core.CoreResult{
		Text:     "hello",
		Thinking: "considered",
		ToolCalls: []core.ToolCall{{
			ID:   "call_1",
			Name: "lookup",
			Args: map[string]any{"q": "go"},
		}},
	})

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	choices := body["choices"].([]any)
	choice := choices[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("finish reason = %#v", choice["finish_reason"])
	}
	message := choice["message"].(map[string]any)
	if message["reasoning_content"] != "considered" {
		t.Fatalf("reasoning content = %#v", message["reasoning_content"])
	}
	toolCalls := message["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("tool calls len = %d", len(toolCalls))
	}
}

func TestWriteChatStreamAndRealStream(t *testing.T) {
	t.Parallel()

	streamRecorder := httptest.NewRecorder()
	writeChatStream(streamRecorder, "gpt-test", core.CoreResult{
		Text:     "hello",
		Thinking: "plan",
		ToolCalls: []core.ToolCall{{
			ID:   "call_1",
			Name: "lookup",
			Args: map[string]any{"q": "go"},
		}},
	})
	streamBody := streamRecorder.Body.String()
	if !strings.Contains(streamBody, `"reasoning_content":"plan"`) {
		t.Fatalf("missing reasoning delta: %s", streamBody)
	}
	if !strings.Contains(streamBody, `"tool_calls"`) {
		t.Fatalf("missing tool call delta: %s", streamBody)
	}
	if !strings.Contains(streamBody, "data: [DONE]") {
		t.Fatalf("missing done marker: %s", streamBody)
	}

	realRecorder := httptest.NewRecorder()
	ch := make(chan core.StreamEvent, 3)
	ch <- core.StreamEvent{TextDelta: "hello"}
	ch <- core.StreamEvent{ThinkingDelta: "plan"}
	ch <- core.StreamEvent{Done: true}
	writeChatRealStream(realRecorder, "gpt-test", ch)
	realBody := realRecorder.Body.String()
	if !strings.Contains(realBody, `"content":"hello"`) {
		t.Fatalf("missing streamed text delta: %s", realBody)
	}
	if !strings.Contains(realBody, `"reasoning_content":"plan"`) {
		t.Fatalf("missing streamed thinking delta: %s", realBody)
	}
	if !strings.Contains(realBody, "data: [DONE]") {
		t.Fatalf("missing final done marker: %s", realBody)
	}
}
