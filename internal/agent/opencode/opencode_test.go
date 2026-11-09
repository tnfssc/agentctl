package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/jsonc"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/testutil"
)

const userConfig = `{
  "$schema": "https://opencode.ai/config.json",
  // mine
  "provider": {
    "writer": {
      "npm": "@ai-sdk/openai-compatible",
      "options": { "baseURL": "https://api.writer.com/v1" },
      "models": { "palmyra-x6": { "name": "Palmyra X6" } },
    },
  },
  "model": "openai/gpt-6.1-sol",
  "permission": { "external_directory": { "/**": "allow" } },
}
`

var proxyModels = []proxy.Model{{ID: "gpt-6-luna", OwnedBy: "openai"}, {ID: "palmyra-x6", OwnedBy: "writer"}}

func setup(t *testing.T) (path string, cfg *config.Config) {
	t.Helper()
	home := testutil.Home(t)
	path = filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(userConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: "https://spark.example:8445", Key: "sk-secret"}}}
}

func use(t *testing.T, cfg *config.Config, st *config.State, route, model string, list []proxy.Model) agent.Plan {
	t.Helper()
	plan, err := PlanUse(context.Background(), cfg, st, route, model, list)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestProxyThenLocalRestoresConfig(t *testing.T) {
	path, cfg := setup(t)
	st := &config.State{}
	if err := RememberLocal(st); err != nil {
		t.Fatal(err)
	}
	use(t, cfg, st, "spark", "", proxyModels)

	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "sk-secret") {
		t.Fatal("the key must never be in OpenCode's config")
	}
	keyPath, _ := KeyPath("spark")
	if key, err := os.ReadFile(keyPath); err != nil || string(key) != "sk-secret\n" {
		t.Fatalf("key file: %q %v", key, err)
	}
	if fi, _ := os.Stat(keyPath); fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file must be private, is %v", fi.Mode().Perm())
	}
	for path, want := range map[string]string{
		"providers.agentctl.settings.apiKey":           "{file:" + keyPath + "}",
		"providers.agentctl.settings.baseURL":          "https://spark.example:8445/v1",
		"providers.agentctl.models.gpt-6-luna.package": "@ai-sdk/openai",
		"providers.claude-agent.settings.baseURL":      "https://spark.example:8445",
		"providers.claude-agent.settings.authToken":    "{file:" + keyPath + "}",
		"model": "agentctl/gpt-6-luna",
		"providers.agentctl.models.palmyra-x6.package": "",
		"provider.writer.options.baseURL":              "https://api.writer.com/v1",
	} {
		if v := jsonc.GetString(got, strings.Split(path, ".")...); v != want {
			t.Errorf("%s = %q, want %q", path, v, want)
		}
	}
	if !strings.Contains(string(got), "// mine") {
		t.Fatal("comments must stay")
	}
	if s, err := Current(cfg); err != nil || s.Route != "spark" || s.Model != "gpt-6-luna" {
		t.Fatalf("status = %+v %v", s, err)
	}

	// Switching again (e.g. a refresh) changes nothing.
	plan, err := PlanUse(context.Background(), cfg, st, "spark", "gpt-6-luna", proxyModels)
	if err != nil || !plan.Empty() {
		t.Fatalf("re-planning the same route should change nothing: %v\n%s", err, plan.Diff())
	}

	use(t, cfg, st, agent.Local, "", nil)
	if got, _ := os.ReadFile(path); string(got) != userConfig {
		t.Fatalf("local should restore the config byte for byte:\n%s", got)
	}
}

func TestLocalLeavesTheUsersOwnClaudeAgentAlone(t *testing.T) {
	path, cfg := setup(t)
	own := strings.Replace(userConfig, `"model":`, `"providers": { "claude-agent": { "settings": { "baseURL": "https://gateway.corp" } } },
  "model":`, 1)
	if err := os.WriteFile(path, []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	use(t, cfg, &config.State{}, agent.Local, "", nil)
	if got, _ := os.ReadFile(path); string(got) != own {
		t.Fatalf("claude-agent settings agentctl didn't write must stay:\n%s", got)
	}
}

func TestNoConfigBecomesNoConfig(t *testing.T) {
	home := testutil.Home(t)
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	cfg := &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: "https://spark.example"}}}
	st := &config.State{}
	use(t, cfg, st, "spark", "", proxyModels)
	keyPath, _ := KeyPath("spark")
	if key, _ := os.ReadFile(keyPath); string(key) != "agentctl-no-key\n" {
		t.Fatalf("a proxy without a key still needs a placeholder: %q", key)
	}
	use(t, cfg, st, agent.Local, "", nil)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("opencode.json was agentctl's and should be gone: %v", err)
	}
}

func TestJSONCFileWins(t *testing.T) {
	home := testutil.Home(t)
	dir := filepath.Join(home, ".config", "opencode")
	for _, name := range []string{"opencode.json", "opencode.jsonc"} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if p, _ := ConfigPath(); filepath.Base(p) != "opencode.jsonc" {
		t.Fatalf("OpenCode reads opencode.jsonc last, so agentctl must edit it: %s", p)
	}
}
