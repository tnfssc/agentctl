package app

import (
	"context"
	"errors"
	"testing"

	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/testutil"
)

var served = map[string]string{"claude-opus-5-5": "anthropic", "gpt-6.1-sol": "openai", "gpt-6-luna": "openai"}

func TestProbeSpotsDisguisedNames(t *testing.T) {
	testutil.Home(t)
	plain := ProbeProxy(context.Background(), config.Proxy{URL: testutil.Proxy(t, served).URL, Key: testutil.Key})
	if err := plain.Err(); err != nil || len(plain.Disguised) != 0 || plain.Counts.Claude != 1 || plain.Counts.Other != 2 {
		t.Fatalf("plain probe = %+v", plain)
	}
	cloaked := ProbeProxy(context.Background(), config.Proxy{URL: testutil.CloakingProxy(t, served).URL, Key: testutil.Key})
	if len(cloaked.Disguised) != 2 {
		t.Fatalf("want 2 disguised names, got %v", cloaked.Disguised)
	}
}

func TestProbeReportsBadKey(t *testing.T) {
	testutil.Home(t)
	r := ProbeProxy(context.Background(), config.Proxy{URL: testutil.Proxy(t, served).URL, Key: "nope"})
	if r.HealthErr != nil || r.ModelsErr == nil {
		t.Fatalf("want healthy proxy with rejected key, got %+v", r)
	}
}

func TestRefreshCodex(t *testing.T) {
	testutil.Home(t)
	prev := codex.BundledCatalog
	codex.BundledCatalog = func(context.Context) ([]map[string]any, error) { return testutil.BundledCatalog(), nil }
	t.Cleanup(func() { codex.BundledCatalog = prev })
	ctx := context.Background()
	cfg := &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: testutil.Proxy(t, served).URL, Key: testutil.Key}}}
	st := &config.State{}

	if _, err := RefreshCodex(ctx, cfg, st); !errors.Is(err, ErrNothingToRefresh) {
		t.Fatalf("refresh on local: %v", err)
	}
	plan, err := Plan(ctx, cfg, st, Request{Agent: "codex", Route: "spark", Model: "gpt-6-luna"})
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(plan, st); err != nil {
		t.Fatal(err)
	}
	plan, err = RefreshCodex(ctx, cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Empty() {
		t.Fatalf("refresh with unchanged models should change nothing:\n%s", plan.Diff())
	}
}

func TestClaudeAckOnlyForProxyRoutes(t *testing.T) {
	st := &config.State{}
	if !NeedsClaudeAck(st, Request{Agent: "claude", Route: "spark"}) {
		t.Fatal("claude via proxy needs the warning")
	}
	if NeedsClaudeAck(st, Request{Agent: "claude", Route: "local"}) || NeedsClaudeAck(st, Request{Agent: "codex", Route: "spark"}) {
		t.Fatal("only claude via proxy needs the warning")
	}
}
