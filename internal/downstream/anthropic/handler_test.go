package anthropic

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lhpqaq/all2api/internal/config"
	"github.com/lhpqaq/all2api/internal/core"
	"github.com/lhpqaq/all2api/internal/diag"
)

func TestExtractSystemAndMessages(t *testing.T) {
	t.Parallel()

	if extractSystem(nil) != "" {
		t.Fatal("expected nil system to flatten to empty string")
	}
	if extractSystem("system") != "system" {
		t.Fatal("expected string system to be preserved")
	}

	msgs := []message{{
		Role: "user",
		Content: []any{
			map[string]any{"type": "text", "text": "hello"},
			map[string]any{"type": "tool_result", "tool_use_id": "call_1", "content": "result"},
			map[string]any{"type": "tool_use", "id": "call_2", "name": "lookup", "input": map[string]any{"q": "go"}},
		},
	}}

	got := extractMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("messages len = %d", len(got))
	}
	if got[0].Content != "hello\nresult\n" {
		t.Fatalf("content = %q", got[0].Content)
	}
	if got[0].ToolCallID != "call_1" {
		t.Fatalf("tool call id = %q", got[0].ToolCallID)
	}
	if len(got[0].ToolCalls) != 1 || got[0].ToolCalls[0].Name != "lookup" {
		t.Fatalf("tool calls = %#v", got[0].ToolCalls)
	}
}

func TestExtractToolChoiceVariants(t *testing.T) {
	t.Parallel()

	if got := extractToolChoice(nil); got != (core.ToolChoice{Mode: "auto"}) {
		t.Fatalf("nil tool choice = %#v", got)
	}
	if got := extractToolChoice(map[string]any{"type": "any"}); got != (core.ToolChoice{Mode: "any"}) {
		t.Fatalf("any tool choice = %#v", got)
	}
	if got := extractToolChoice(map[string]any{"type": "tool", "name": "lookup"}); got != (core.ToolChoice{Mode: "tool", Name: "lookup"}) {
		t.Fatalf("tool choice = %#v", got)
	}
}

func TestWithDiagContextAndWriteStream(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set("X-All2API-Debug", "1")
	req.Header.Set("X-Request-Id", "req-1")

	ctx := withDiagContext(req, config.Config{})
	if !diag.Debug(ctx) {
		t.Fatal("expected debug to be enabled")
	}
	if diag.RequestID(ctx) != "req-1" {
		t.Fatalf("request id = %q", diag.RequestID(ctx))
	}

	rr := httptest.NewRecorder()
	writeStream(rr, "claude-test", core.CoreResult{
		Text: "hello",
		ToolCalls: []core.ToolCall{{
			ID:   "call_1",
			Name: "lookup",
			Args: map[string]any{"q": "go"},
		}},
	})
	body := rr.Body.String()
	if !strings.Contains(body, "event: message_start") {
		t.Fatalf("missing message_start event: %s", body)
	}
	if !strings.Contains(body, `"type":"text_delta"`) {
		t.Fatalf("missing text delta: %s", body)
	}
	if !strings.Contains(body, `"type":"tool_use"`) {
		t.Fatalf("missing tool_use block: %s", body)
	}
	if !strings.Contains(body, `"type":"message_stop"`) {
		t.Fatalf("missing message_stop event: %s", body)
	}
}
