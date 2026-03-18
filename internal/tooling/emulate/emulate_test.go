package emulate

import (
	"strings"
	"testing"

	"github.com/lhpqaq/all2api/internal/core"
)

func TestCompactSchemaSortsAndMarksRequired(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"properties": map[string]any{
			"enabled": map[string]any{"type": "boolean"},
			"count":   map[string]any{"type": "integer"},
			"path":    map[string]any{"type": "string"},
		},
		"required": []any{"path", "enabled"},
	}

	got := compactSchema(schema)
	if got != "count?:int,enabled!:bool,path!:str" {
		t.Fatalf("compact schema = %q", got)
	}
}

func TestInjectToolingAndActionBlockExample(t *testing.T) {
	t.Parallel()

	tools := []core.ToolDef{
		{
			Name:        "read",
			Description: "Read a file from disk",
			InputSchema: map[string]any{
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []any{"path"},
			},
		},
		{Name: "ask_followup_question"},
	}

	got := InjectTooling("base system", tools, core.ToolChoice{Mode: "tool", Name: "read"})
	if !strings.Contains(got, "base system\n\n---\n\n") {
		t.Fatalf("missing system separator in %q", got)
	}
	if !strings.Contains(got, "read(path!:str) - Read a file from disk") {
		t.Fatalf("missing tool signature in %q", got)
	}
	if !strings.Contains(got, `You MUST call "read"`) {
		t.Fatalf("missing force constraint in %q", got)
	}

	example := ActionBlockExample(tools)
	if !strings.Contains(example, `"tool": "read"`) {
		t.Fatalf("missing read tool in example: %q", example)
	}
	if !strings.Contains(example, `"path": "README.md"`) {
		t.Fatalf("missing example path in %q", example)
	}
}

func TestExtractThinkingHandlesWrappedAndUnclosedBlocks(t *testing.T) {
	t.Parallel()

	input := "Visible\n`<thinking>\nfirst\n</thinking>`\nMore\n<thinking>second"

	thinking, clean := ExtractThinking(input)
	if thinking != "first\n\nsecond" {
		t.Fatalf("thinking = %q", thinking)
	}
	if clean != "Visible\n\nMore" {
		t.Fatalf("clean = %q", clean)
	}
}

func TestLooksLikeRefusalAndForceToolingPrompt(t *testing.T) {
	t.Parallel()

	if !LooksLikeRefusal("I am a support assistant for Cursor and cannot search the internet.") {
		t.Fatal("expected refusal to be detected")
	}
	if LooksLikeRefusal("Here is the tool call you requested.") {
		t.Fatal("unexpected refusal detection")
	}

	prompt := ForceToolingPrompt(core.ToolChoice{Mode: "tool", Name: "bash"})
	if !strings.Contains(prompt, `You MUST call "bash".`) {
		t.Fatalf("prompt = %q", prompt)
	}
}
