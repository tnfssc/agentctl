package pi

import (
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

const userModels = `{
  // my own provider
  "providers": {
    "writer": {
      "baseUrl": "https://api.writer.com/v1",
      "api": "openai-completions",
      "apiKey": "!pass writer",
      "models": [{ "id": "palmyra-x6" }]
    }
  }
}
`

const userSettings = `{
  "defaultProvider": "openai-codex",
  "defaultModel": "gpt-6.1-sol",
  "defaultThinkingLevel": "high",
  "quietStartup": true
}
`

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) (dir string, cfg *config.Config) {
	t.Helper()
	home := testutil.Home(t)
	dir = filepath.Join(home, ".pi", "agent")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	write(t, filepath.Join(dir, "models.json"), userModels)
	write(t, filepath.Join(dir, "settings.json"), userSettings)
	write(t, filepath.Join(dir, "models-store.json"), `{"openai-codex":{"models":[
		{"id":"gpt-6-luna","name":"GPT-6 Luna","api":"openai-codex-responses","reasoning":true,"contextWindow":272000,"maxTokens":128000,"thinkingLevelMap":{"max":"max"}}]}}`)
	return dir, &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: "https://spark.example:8445"}}}
}

var proxyModels = []proxy.Model{{ID: "gpt-6-luna", OwnedBy: "openai"}, {ID: "gemini-3.1-pro", OwnedBy: "google"}}

func apply(t *testing.T, plan agent.Plan) {
	t.Helper()
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
}

func TestProxyThenLocalRestoresFiles(t *testing.T) {
	dir, cfg := setup(t)
	st := &config.State{}
	if err := RememberLocal(st); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUse(cfg, st, "spark", "gemini-3.1-pro", BuildModels(proxyModels))
	if err != nil {
		t.Fatal(err)
	}
	apply(t, plan)

	models, _ := os.ReadFile(filepath.Join(dir, "models.json"))
	if !strings.Contains(string(models), "// my own provider") || !strings.Contains(string(models), `"writer"`) {
		t.Fatalf("the user's provider and comment must stay:\n%s", models)
	}
	key := jsonc.GetString(models, "providers", "agentctl", "apiKey")
	if key != "!'/opt/bin/agentctl' token 'spark'" {
		t.Fatalf("apiKey = %q; Pi must ask agentctl for the key, never hold it", key)
	}
	if got := jsonc.GetString(models, "providers", "agentctl", "baseUrl"); got != "https://spark.example:8445/v1" {
		t.Fatalf("baseUrl = %q", got)
	}
	var doc struct {
		Providers map[string]struct{ Models []Model } `json:"providers"`
	}
	if err := jsonc.Unmarshal(models, &doc); err != nil {
		t.Fatal(err)
	}
	luna := doc.Providers["agentctl"].Models[0]
	if luna.API != "openai-responses" || luna.ContextWindow != 272000 || luna.Name != "GPT-6 Luna" {
		t.Fatalf("an OpenAI model Pi knows should use the Responses API and Pi's metadata: %+v", luna)
	}
	if gem := doc.Providers["agentctl"].Models[1]; gem.API != "" || gem.ContextWindow != 0 {
		t.Fatalf("an unknown model keeps Pi's defaults: %+v", gem)
	}

	s, err := Current(cfg)
	if err != nil || s.Route != "spark" || s.Model != "gemini-3.1-pro" {
		t.Fatalf("status = %+v %v", s, err)
	}

	plan, err = PlanUse(cfg, st, agent.Local, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, plan)
	for name, want := range map[string]string{"models.json": userModels, "settings.json": userSettings} {
		if got, _ := os.ReadFile(filepath.Join(dir, name)); string(got) != want {
			t.Errorf("%s not restored:\n%s", name, got)
		}
	}
	if s, _ := Current(cfg); s.Route != agent.Local || s.Model != "openai-codex/gpt-6.1-sol" {
		t.Fatalf("status after local = %+v", s)
	}
}

func TestLocalRemovesFilesAgentctlCreated(t *testing.T) {
	home := testutil.Home(t)
	dir := filepath.Join(home, ".pi", "agent")
	cfg := &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: "https://spark.example"}}}
	st := &config.State{}
	plan, err := PlanUse(cfg, st, "spark", "", BuildModels(proxyModels))
	if err != nil {
		t.Fatal(err)
	}
	apply(t, plan)
	if plan, err = PlanUse(cfg, st, agent.Local, "", nil); err != nil {
		t.Fatal(err)
	}
	apply(t, plan)
	if _, err := os.Stat(filepath.Join(dir, "models.json")); !os.IsNotExist(err) {
		t.Fatalf("models.json was agentctl's and should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("settings.json was agentctl's and should be gone: %v", err)
	}
}

func TestUnknownModelRefused(t *testing.T) {
	_, cfg := setup(t)
	if _, err := PlanUse(cfg, &config.State{}, "spark", "claude-opus-5-5", BuildModels(proxyModels)); err == nil {
		t.Fatal("a model the provider doesn't list must be refused")
	}
}
