package orchestrator

import (
	"context"
	"testing"

	"github.com/lhpqaq/all2api/internal/config"
	"github.com/lhpqaq/all2api/internal/core"
	"github.com/lhpqaq/all2api/internal/upstream"
)

type routingTestUpstream struct {
	models map[string]bool
}

func (u *routingTestUpstream) Do(context.Context, core.CoreRequest) (core.CoreResult, error) {
	return core.CoreResult{}, nil
}

func (u *routingTestUpstream) HasModel(_ context.Context, model string) (bool, error) {
	return u.models[model], nil
}

func TestSplitUpstreamModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      string
		wantA      string
		wantB      string
		wantResult bool
	}{
		{input: "cursor/gpt-5", wantA: "cursor", wantB: "gpt-5", wantResult: true},
		{input: " cursor / gpt-5 ", wantA: "cursor", wantB: "gpt-5", wantResult: true},
		{input: "cursor", wantResult: false},
		{input: "cursor/", wantResult: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			a, b, ok := splitUpstreamModel(tt.input)
			if ok != tt.wantResult || a != tt.wantA || b != tt.wantB {
				t.Fatalf("got (%q, %q, %t), want (%q, %q, %t)", a, b, ok, tt.wantA, tt.wantB, tt.wantResult)
			}
		})
	}
}

func TestFindUpstreamForModel(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Upstreams: map[string]config.UpstreamConf{
			"first":  {Type: "first"},
			"second": {Type: "second"},
		},
	}
	reg, err := upstream.NewRegistry(cfg)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	reg.RegisterFactory("first", func(string, config.UpstreamConf) (upstream.Upstream, upstream.Capabilities, error) {
		return &routingTestUpstream{models: map[string]bool{}}, upstream.Capabilities{}, nil
	})
	reg.RegisterFactory("second", func(string, config.UpstreamConf) (upstream.Upstream, upstream.Capabilities, error) {
		return &routingTestUpstream{models: map[string]bool{"match": true}}, upstream.Capabilities{}, nil
	})

	orch := &Orchestrator{cfg: cfg, reg: reg}
	name, ok := orch.findUpstreamForModel(context.Background(), "match")
	if !ok {
		t.Fatal("expected a matching upstream")
	}
	if name != "second" {
		t.Fatalf("upstream = %q", name)
	}

	if _, ok := orch.findUpstreamForModel(context.Background(), "missing"); ok {
		t.Fatal("expected no match for missing model")
	}
}
