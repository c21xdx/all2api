package zed

import (
	"testing"

	"github.com/lhpqaq/all2api/internal/core"
)

func floatPtr(v float64) *float64 {
	return &v
}

func TestBuildPayloadSetsThinkingToolingAndMergedMessages(t *testing.T) {
	t.Parallel()

	z := &zedUpstream{}
	req := core.CoreRequest{
		Model:       "gpt-5.2",
		Thinking:    true,
		System:      "base system",
		MaxTokens:   0,
		Temperature: floatPtr(0.7),
		Tools: []core.ToolDef{{
			Name:        "lookup",
			Description: "Search docs",
			InputSchema: map[string]any{"type": "object"},
		}},
		ToolChoice: core.ToolChoice{Mode: "tool", Name: "lookup"},
		Messages: []core.Message{
			{Role: "system", Content: "extra system"},
			{Role: "user", Content: "hello"},
			{Role: "tool", ToolCallID: "call_1", Content: "tool output"},
			{Role: "assistant", Content: "done", ToolCalls: []core.ToolCall{{ID: "call_2", Name: "lookup", Args: map[string]any{"q": "go"}}}},
			{Role: "user", Content: "final"},
		},
	}

	payload, err := z.buildPayload(req)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}

	if payload.Provider != "open_ai" {
		t.Fatalf("provider = %q", payload.Provider)
	}
	if payload.ThreadID == "" || payload.PromptID == "" {
		t.Fatal("expected generated thread and prompt ids")
	}
	provReq := payload.ProviderRequest
	if provReq.MaxTokens != 8192 {
		t.Fatalf("max tokens = %d", provReq.MaxTokens)
	}
	if provReq.System != "base system\n\nextra system" {
		t.Fatalf("system = %q", provReq.System)
	}
	if provReq.Thinking["type"] != "enabled" {
		t.Fatalf("thinking = %#v", provReq.Thinking)
	}
	if provReq.ToolChoice.(map[string]interface{})["name"] != "lookup" {
		t.Fatalf("tool choice = %#v", provReq.ToolChoice)
	}
	if len(provReq.Tools) != 1 {
		t.Fatalf("tools len = %d", len(provReq.Tools))
	}
	if len(provReq.Messages) != 3 {
		t.Fatalf("messages len = %d", len(provReq.Messages))
	}

	first := provReq.Messages[0]
	if first.Role != "user" {
		t.Fatalf("first role = %q", first.Role)
	}
	firstContent, ok := first.Content.([]map[string]interface{})
	if !ok {
		t.Fatalf("first content type = %T", first.Content)
	}
	if len(firstContent) != 2 || firstContent[0]["type"] != "text" || firstContent[1]["type"] != "tool_result" {
		t.Fatalf("first content = %#v", firstContent)
	}

	second := provReq.Messages[1]
	secondContent, ok := second.Content.([]map[string]interface{})
	if !ok {
		t.Fatalf("second content type = %T", second.Content)
	}
	if len(secondContent) != 2 || secondContent[1]["type"] != "tool_use" {
		t.Fatalf("second content = %#v", secondContent)
	}

	third := provReq.Messages[2]
	thirdContent, ok := third.Content.([]map[string]interface{})
	if !ok {
		t.Fatalf("third content type = %T", third.Content)
	}
	if len(thirdContent) != 1 || thirdContent[0]["text"] != "final" {
		t.Fatalf("third content = %#v", thirdContent)
	}
}
