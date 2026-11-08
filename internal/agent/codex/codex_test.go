package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/testutil"
)

const existing = `# my settings
model = "gpt-6.1-sol"
model_reasoning_effort = "high"

[features]
hooks = true
`

var proxyModels = []proxy.Model{
	{ID: "claude-opus-5-5", OwnedBy: "anthropic"},
	{ID: "claude-sonnet-5-high", OwnedBy: "antigravity"},
	{ID: "gpt-6.1-sol", OwnedBy: "openai"},
	{ID: "gemini-3-pro", OwnedBy: "google"},
	{ID: "palmyra-x6", OwnedBy: "WRITER"},
}

func setup(t *testing.T) (*config.Config, string) {
	t.Helper()
	home := testutil.Home(t)
	prev := BundledCatalog
	BundledCatalog = func(context.Context) ([]map[string]any, error) { return testutil.BundledCatalog(), nil }
	t.Cleanup(func() { BundledCatalog = prev })
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	return &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: "http://127.0.0.1:8317"}}}, path
}

func TestCatalogHidesClaudeAndUnverified(t *testing.T) {
	setup(t)
	cat, err := BuildCatalog(context.Background(), proxyModels, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cat.Slugs, ","); got != "gpt-6.1-sol,gemini-3-pro" {
		t.Fatalf("slugs = %s", got)
	}
	if strings.Contains(string(cat.JSON), "claude") {
		t.Fatal("catalog mentions a Claude model")
	}
	if strings.Contains(string(cat.JSON), `"upgrade"`) {
		t.Fatal("cloned entries must not carry the template's upgrade prompt")
	}
	cat, _ = BuildCatalog(context.Background(), proxyModels, []string{"palmyra-x6", "claude-opus-5-5"})
	if got := strings.Join(cat.Slugs, ","); got != "gpt-6.1-sol,gemini-3-pro,palmyra-x6" {
		t.Fatalf("with allow list, slugs = %s", got)
	}
}

func TestCatalogRefusesWhenNothingUsable(t *testing.T) {
	setup(t)
	if _, err := BuildCatalog(context.Background(), proxyModels[:2], nil); err == nil {
		t.Fatal("want error when only Claude models are served")
	}
}

func TestProxyThenLocalRoundTrip(t *testing.T) {
	cfg, path := setup(t)
	st := &config.State{}
	cat, err := BuildCatalog(context.Background(), proxyModels, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := RememberLocalModel(st); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUse(cfg, st, "spark", "gemini-3-pro", cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}

	b, _ := os.ReadFile(path)
	var doc struct {
		Model            string `toml:"model"`
		ModelProvider    string `toml:"model_provider"`
		ModelCatalogJSON string `toml:"model_catalog_json"`
		Providers        map[string]struct {
			BaseURL string `toml:"base_url"`
			WireAPI string `toml:"wire_api"`
			Auth    struct {
				Command string   `toml:"command"`
				Args    []string `toml:"args"`
			} `toml:"auth"`
		} `toml:"model_providers"`
	}
	if err := toml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("written config does not parse: %v\n%s", err, b)
	}
	p := doc.Providers["agentctl"]
	if doc.Model != "gemini-3-pro" || doc.ModelProvider != "agentctl" || p.BaseURL != "http://127.0.0.1:8317/v1" ||
		p.WireAPI != "responses" || p.Auth.Command != "/opt/bin/agentctl" || strings.Join(p.Auth.Args, " ") != "token spark" {
		t.Fatalf("unexpected config:\n%s", b)
	}
	if !strings.Contains(string(b), "# my settings") {
		t.Fatal("comments lost")
	}
	if _, err := os.Stat(doc.ModelCatalogJSON); err != nil {
		t.Fatalf("catalog not written: %v", err)
	}
	if s, _ := Current(cfg); s.Route != "spark" {
		t.Fatalf("route = %q", s.Route)
	}

	plan, err = PlanUse(cfg, st, agent.Local, "", Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != existing {
		t.Fatalf("local should restore the original exactly, got:\n%s", b)
	}
}

func TestRejectsModelNotInCatalog(t *testing.T) {
	cfg, _ := setup(t)
	cat, _ := BuildCatalog(context.Background(), proxyModels, nil)
	if _, err := PlanUse(cfg, &config.State{}, "spark", "claude-opus-5-5", cat); err == nil {
		t.Fatal("want error for a Claude model")
	}
}
