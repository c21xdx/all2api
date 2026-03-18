package upstream

import (
	"context"
	"errors"
	"testing"

	"github.com/lhpqaq/all2api/internal/config"
	"github.com/lhpqaq/all2api/internal/core"
)

type fakeUpstream struct{}

func (fakeUpstream) Do(context.Context, core.CoreRequest) (core.CoreResult, error) {
	return core.CoreResult{}, nil
}

type checkerListerUpstream struct {
	has          bool
	err          error
	models       []string
	checkerCalls int
	listerCalls  int
}

func (u *checkerListerUpstream) Do(context.Context, core.CoreRequest) (core.CoreResult, error) {
	return core.CoreResult{}, nil
}

func (u *checkerListerUpstream) HasModel(context.Context, string) (bool, error) {
	u.checkerCalls++
	return u.has, u.err
}

func (u *checkerListerUpstream) ListModels(context.Context) ([]string, error) {
	u.listerCalls++
	return u.models, u.err
}

type listerOnlyUpstream struct {
	models []string
	err    error
	calls  int
}

func (u *listerOnlyUpstream) Do(context.Context, core.CoreRequest) (core.CoreResult, error) {
	return core.CoreResult{}, nil
}

func (u *listerOnlyUpstream) ListModels(context.Context) ([]string, error) {
	u.calls++
	return u.models, u.err
}

func TestHasModelUsesCheckerBeforeLister(t *testing.T) {
	t.Parallel()

	up := &checkerListerUpstream{has: true, models: []string{"other"}}
	ok, err := HasModel(context.Background(), up, "target")
	if err != nil {
		t.Fatalf("HasModel returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected model to be reported as supported")
	}
	if up.checkerCalls != 1 {
		t.Fatalf("checker calls = %d", up.checkerCalls)
	}
	if up.listerCalls != 0 {
		t.Fatalf("lister calls = %d", up.listerCalls)
	}
}

func TestHasModelFallsBackToLister(t *testing.T) {
	t.Parallel()

	up := &listerOnlyUpstream{models: []string{"a", "target"}}
	ok, err := HasModel(context.Background(), up, "target")
	if err != nil {
		t.Fatalf("HasModel returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected model to be found in lister output")
	}
	if up.calls != 1 {
		t.Fatalf("lister calls = %d", up.calls)
	}
}

func TestHasModelHandlesNilAndListerErrors(t *testing.T) {
	t.Parallel()

	ok, err := HasModel(context.Background(), nil, "target")
	if err != nil {
		t.Fatalf("unexpected error for nil upstream: %v", err)
	}
	if ok {
		t.Fatal("nil upstream should not report model support")
	}

	boom := errors.New("boom")
	_, err = HasModel(context.Background(), &listerOnlyUpstream{err: boom}, "target")
	if !errors.Is(err, boom) {
		t.Fatalf("expected propagated error, got %v", err)
	}
}

func TestRegistryCachesFactoryResults(t *testing.T) {
	t.Parallel()

	reg, err := NewRegistry(config.Config{
		Upstreams: map[string]config.UpstreamConf{
			"main": {Type: "mock"},
		},
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	factoryCalls := 0
	reg.RegisterFactory("mock", func(string, config.UpstreamConf) (Upstream, Capabilities, error) {
		factoryCalls++
		return &fakeUpstream{}, Capabilities{NativeToolCalls: true}, nil
	})

	first, firstCap, err := reg.Get("main")
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	second, secondCap, err := reg.Get("main")
	if err != nil {
		t.Fatalf("second get: %v", err)
	}

	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d", factoryCalls)
	}
	if first != second {
		t.Fatal("expected cached upstream instance to be reused")
	}
	if firstCap != secondCap {
		t.Fatalf("cached capabilities changed: %#v vs %#v", firstCap, secondCap)
	}
}

func TestRegistryErrorsForUnknownOrUnregisteredTypes(t *testing.T) {
	t.Parallel()

	reg, err := NewRegistry(config.Config{
		Upstreams: map[string]config.UpstreamConf{
			"main": {Type: "missing"},
		},
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}

	if _, _, err := reg.Get("unknown"); err == nil {
		t.Fatal("expected unknown upstream error")
	}
	if _, _, err := reg.Get("main"); err == nil {
		t.Fatal("expected missing factory error")
	}
}
