package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"strings"
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

func TestRefreshRebuildsEveryRoutedList(t *testing.T) {
	testutil.Home(t)
	prev := codex.BundledCatalog
	codex.BundledCatalog = func(context.Context) ([]map[string]any, error) { return testutil.BundledCatalog(), nil }
	t.Cleanup(func() { codex.BundledCatalog = prev })
	ctx := context.Background()
	models := maps.Clone(served)
	cfg := &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: testutil.Proxy(t, models).URL, Key: testutil.Key}}}
	st := &config.State{}

	if _, err := Refresh(ctx, cfg, st); !errors.Is(err, ErrNothingToRefresh) {
		t.Fatalf("refresh with every agent local: %v", err)
	}
	for _, name := range []string{"codex", "opencode", "pi"} {
		plan, err := Plan(ctx, cfg, st, Request{Agent: name, Route: "spark", Model: "gpt-6-luna"})
		if err != nil {
			t.Fatal(err)
		}
		if err := Apply(context.Background(), io.Discard, plan, st); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := Refresh(ctx, cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Empty() {
		t.Fatalf("refresh with unchanged models should change nothing:\n%s", plan.Diff())
	}

	models["gpt-7"] = "openai"
	if plan, err = Refresh(ctx, cfg, st); err != nil {
		t.Fatal(err)
	}
	touched := map[string]bool{}
	for _, c := range plan.Changes {
		if !bytes.Equal(c.Before, c.After) {
			touched[c.Agent] = true
			if !bytes.Contains(c.After, []byte("gpt-7")) && !strings.HasSuffix(c.Path, "settings.json") && !strings.Contains(c.Path, "config.toml") {
				t.Errorf("%s: %s does not list the new model", c.Agent, c.Path)
			}
		}
	}
	if len(touched) != 3 {
		t.Fatalf("every routed agent should get the new model, got %v", touched)
	}
	for _, name := range []string{"codex", "opencode", "pi"} {
		a, _ := FindAgent(name)
		if s, err := a.Status(cfg); err != nil || s.Model != "gpt-6-luna" {
			t.Errorf("%s: refresh must keep the chosen model: %+v %v", name, s, err)
		}
	}
}

func TestClaudeAckOnlyForProxyRoutes(t *testing.T) {
	st := &config.State{}
	if !NeedsClaudeAck(st, Request{Agent: "claude", Route: "spark"}) {
		t.Fatal("claude via proxy needs the warning")
	}
	if !NeedsClaudeAck(st, Request{Agent: "opencode", Route: "spark"}) {
		t.Fatal("OpenCode via proxy runs Claude Code against it too")
	}
	for _, r := range []Request{{Agent: "claude", Route: "local"}, {Agent: "codex", Route: "spark"}, {Agent: "pi", Route: "spark"}, {Agent: "opencode", Route: "local"}} {
		if NeedsClaudeAck(st, r) {
			t.Fatalf("%+v must not need the warning", r)
		}
	}
}
