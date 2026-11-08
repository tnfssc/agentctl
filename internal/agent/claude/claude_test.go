package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/testutil"
)

const existing = `{
  "theme": "dark",
  "env": {
    "DISABLE_ERROR_REPORTING": "1"
  },
  "permissions": {
    "deny": [
      "NotebookEdit"
    ]
  }
}
`

func setup(t *testing.T, settings string) (*config.Config, string) {
	t.Helper()
	home := testutil.Home(t)
	path := filepath.Join(home, ".claude", "settings.json")
	if settings != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: "https://spark.example:8445/v1"}}}
	return cfg, path
}

func apply(t *testing.T, cfg *config.Config, route string) []byte {
	t.Helper()
	plan, err := PlanUse(cfg, route)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	path, _ := SettingsPath()
	b, _ := os.ReadFile(path)
	return b
}

func TestProxyThenLocalRoundTrip(t *testing.T) {
	cfg, _ := setup(t, existing)

	got := apply(t, cfg, "spark")
	for path, want := range map[string]string{
		"env.ANTHROPIC_BASE_URL":                         "https://spark.example:8445",
		"env.CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY": "1",
		"env.DISABLE_ERROR_REPORTING":                    "1",
		"apiKeyHelper":                                   "/opt/bin/agentctl token spark",
		"theme":                                          "dark",
	} {
		if v := gjson.GetBytes(got, path).String(); v != want {
			t.Errorf("%s = %q, want %q", path, v, want)
		}
	}
	if gjson.GetBytes(got, "env.ANTHROPIC_MODEL").Exists() {
		t.Fatal("route switch must not pin ANTHROPIC_MODEL")
	}
	st, err := Current(cfg)
	if err != nil || st.Route != "spark" || st.Model != "" {
		t.Fatalf("Current = %+v, %v", st, err)
	}

	if got := apply(t, cfg, agent.Local); string(got) != existing {
		t.Fatalf("local should restore the original file exactly, got:\n%s", got)
	}
	if st, _ := Current(cfg); st.Route != agent.Local {
		t.Fatalf("route after local = %q", st.Route)
	}
}

func TestLocalRemovesForeignOverrides(t *testing.T) {
	cfg, _ := setup(t, `{"apiKeyHelper": "vault read key", "env": {"ANTHROPIC_AUTH_TOKEN": "x"}}`)
	plan, err := PlanUse(cfg, agent.Local)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 2 {
		t.Fatalf("want warnings about both overrides, got %v", plan.Warnings)
	}
	if after := string(plan.Changes[0].After); strings.TrimSpace(after) != "{}" {
		t.Fatalf("after = %q", after)
	}
}

func TestNoSettingsFileStaysAbsentForLocal(t *testing.T) {
	cfg, _ := setup(t, "")
	plan, err := PlanUse(cfg, agent.Local)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Empty() {
		t.Fatalf("local on a missing file should change nothing, diff:\n%s", plan.Diff())
	}
}

func TestInvalidJSONIsRefused(t *testing.T) {
	cfg, _ := setup(t, `{"theme": `)
	if _, err := PlanUse(cfg, "spark"); err == nil {
		t.Fatal("want error for invalid JSON")
	}
}

func TestSymlinkedSettingsStayLinked(t *testing.T) {
	cfg, path := setup(t, "")
	target := filepath.Join(t.TempDir(), "dotfiles-settings.json")
	if err := os.WriteFile(target, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	apply(t, cfg, "spark")
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("settings.json is no longer a symlink: %v", err)
	}
	b, _ := os.ReadFile(target)
	if !strings.Contains(string(b), "ANTHROPIC_BASE_URL") {
		t.Fatal("symlink target was not updated")
	}
}

func TestRouteSwitchRemovesModelEnvOverride(t *testing.T) {
	for _, route := range []string{"spark", agent.Local} {
		t.Run(route, func(t *testing.T) {
			cfg, _ := setup(t, `{"model":"gpt-6.1-sol","env":{"ANTHROPIC_MODEL":"Opus 5.5","DISABLE_ERROR_REPORTING":"1"}}`)
			got := apply(t, cfg, route)
			if gjson.GetBytes(got, "env.ANTHROPIC_MODEL").Exists() {
				t.Fatal("stale model env override should be removed")
			}
			if gjson.GetBytes(got, "model").String() != "gpt-6.1-sol" {
				t.Fatal("Claude Code's saved model must be preserved")
			}
			if gjson.GetBytes(got, "env.DISABLE_ERROR_REPORTING").String() != "1" {
				t.Fatal("unrelated env must be preserved")
			}
		})
	}
}
