package emulate

import (
	"strings"
	"testing"
)

func TestParseActionBlocksExtractsCallsAndCleansText(t *testing.T) {
	t.Parallel()

	input := "Before\n```json action\n{\"tool\":\"bash\",\"parameters\":{\"command\":\"printf \\\"```\\\"\"}}\n```\nAfter"

	calls, clean, err := ParseActionBlocks(input, Config{MaxScanBytes: 1024, SmartQuotes: true})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls len = %d", len(calls))
	}
	if calls[0].Name != "bash" {
		t.Fatalf("tool name = %q", calls[0].Name)
	}
	if calls[0].Args["command"] != "printf \"```\"" {
		t.Fatalf("command = %#v", calls[0].Args["command"])
	}
	if !strings.HasPrefix(calls[0].ID, "call_") {
		t.Fatalf("call id = %q", calls[0].ID)
	}
	if clean != "Before\n\nAfter" {
		t.Fatalf("clean = %q", clean)
	}
}

func TestParseActionBlocksParsesStringParametersAndSmartQuotes(t *testing.T) {
	t.Parallel()

	input := "```json action\n{\u201ctool\u201d:\u201cread\u201d,\u201cparameters\u201d:\"{\\\"path\\\":\\\"README.md\\\"}\"}\n```"

	calls, clean, err := ParseActionBlocks(input, Config{SmartQuotes: true})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls len = %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Fatalf("tool name = %q", calls[0].Name)
	}
	if calls[0].Args["path"] != "README.md" {
		t.Fatalf("path = %#v", calls[0].Args["path"])
	}
	if clean != "" {
		t.Fatalf("clean = %q", clean)
	}
}

func TestParseActionBlocksRespectsMaxScanBytes(t *testing.T) {
	t.Parallel()

	input := strings.Repeat("x", 20) + "```json action\n{\"tool\":\"read\"}\n```"

	calls, clean, err := ParseActionBlocks(input, Config{MaxScanBytes: 10})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("expected no calls, got %d", len(calls))
	}
	if clean != strings.Repeat("x", 10) {
		t.Fatalf("clean = %q", clean)
	}
}
