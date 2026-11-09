package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/testutil"
)

func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func must(t *testing.T, args ...string) string {
	t.Helper()
	out, err := execute(t, args...)
	if err != nil {
		t.Fatalf("agentctl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func setup(t *testing.T) (home, url string) {
	t.Helper()
	home = testutil.Home(t)
	prev := codex.BundledCatalog
	codex.BundledCatalog = func(context.Context) ([]map[string]any, error) { return testutil.BundledCatalog(), nil }
	t.Cleanup(func() { codex.BundledCatalog = prev })
	srv := testutil.Proxy(t, map[string]string{
		"claude-opus-5-5": "anthropic",
		"gpt-6.1-sol":     "openai",
		"palmyra-x6":      "WRITER",
	})
	return home, srv.URL
}

func TestSwitchingEndToEnd(t *testing.T) {
	home, url := setup(t)

	out := must(t, "proxy", "add", "spark", "--url", url+"/v1", "--key", testutil.Key)
	if !strings.Contains(out, "1 Claude · 1 all agents · 1 unverified") {
		t.Fatalf("add output:\n%s", out)
	}
	info, err := os.Stat(filepath.Join(home, ".config", "agentctl", "config.toml"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config must be private: %v %v", info.Mode(), err)
	}

	if out := must(t, "token", "spark"); strings.TrimSpace(out) != testutil.Key {
		t.Fatalf("token = %q", out)
	}

	// Without a terminal, applying needs --yes.
	if _, err := execute(t, "use", "codex", "spark"); err == nil {
		t.Fatal("want --yes error without a terminal")
	}
	must(t, "use", "codex", "spark", "--yes")
	must(t, "use", "claude", "spark", "--yes")

	out = must(t, "status")
	if strings.Count(out, "proxy spark") != 2 {
		t.Fatalf("status:\n%s", out)
	}

	catalog, err := os.ReadFile(filepath.Join(home, ".config", "agentctl", "codex", "spark.models.json"))
	if err != nil || strings.Contains(string(catalog), "claude") || strings.Contains(string(catalog), "palmyra") {
		t.Fatalf("catalog must list only verified non-Claude models: %v\n%s", err, catalog)
	}

	if _, err := execute(t, "proxy", "rm", "spark"); err == nil {
		t.Fatal("removing a proxy in use must fail")
	}
	must(t, "use", "codex", "local", "--yes")
	must(t, "use", "claude", "local", "--yes")
	if out := must(t, "status"); strings.Count(out, "own login") != 4 {
		t.Fatalf("status after local:\n%s", out)
	}
	must(t, "proxy", "rm", "spark")
}

func TestCodexRefusesClaudeModels(t *testing.T) {
	_, url := setup(t)
	must(t, "proxy", "add", "spark", "--url", url, "--key", testutil.Key)
	_, err := execute(t, "use", "codex", "spark", "--model", "claude-opus-5-5", "--yes")
	if err == nil || !strings.Contains(err.Error(), "only allowed in Claude Code") {
		t.Fatalf("err = %v", err)
	}
	if _, err := execute(t, "proxy", "allow", "spark", "claude-opus-5-5"); err == nil {
		t.Fatal("allowing a Claude model must fail")
	}
	must(t, "proxy", "allow", "spark", "palmyra-x6")
	must(t, "use", "codex", "spark", "--model", "palmyra-x6", "--yes")
}

func TestAddRejectsBadKey(t *testing.T) {
	_, url := setup(t)
	if _, err := execute(t, "proxy", "add", "spark", "--url", url, "--key", "wrong"); err == nil {
		t.Fatal("want error for a rejected key")
	}
	if out := must(t, "proxy", "ls"); !strings.Contains(out, "No proxies") {
		t.Fatalf("failed add must not save:\n%s", out)
	}
}

func TestReservedName(t *testing.T) {
	setup(t)
	if _, err := execute(t, "proxy", "add", "local", "--url", "http://x"); err == nil {
		t.Fatal(`"local" must be rejected`)
	}
}

func TestDevNullIsNotATerminal(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if isTerminal(devNull) {
		t.Fatal("/dev/null reported as a terminal")
	}
}

func TestTokenWithoutKey(t *testing.T) {
	_, url := setup(t)
	if _, err := execute(t, "proxy", "add", "open", "--url", url, "--force"); err != nil {
		t.Fatal(err)
	}
	if out := must(t, "token", "open"); strings.TrimSpace(out) != noKey {
		t.Fatalf("token = %q", out)
	}
}

func TestClaudeModelFlagIsRejected(t *testing.T) {
	_, url := setup(t)
	must(t, "proxy", "add", "spark", "--url", url, "--key", testutil.Key)
	_, err := execute(t, "use", "claude", "spark", "--model", "Opus 5.5", "--yes")
	if err == nil || !strings.Contains(err.Error(), "with /model") {
		t.Fatalf("want /model guidance, got %v", err)
	}
}

func TestTune(t *testing.T) {
	home, _ := setup(t)
	if _, err := execute(t, "tune", "codex"); err == nil {
		t.Fatal("without a terminal, tune needs --yes")
	}
	must(t, "tune", "codex", "--yes")
	got, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil || !strings.Contains(string(got), "[analytics]") || strings.Contains(string(got), "danger-full-access") {
		t.Fatalf("recommended codex settings: %v\n%s", err, got)
	}
	must(t, "tune", "codex", "--reset", "--yes")
	if got, _ := os.ReadFile(filepath.Join(home, ".codex", "config.toml")); strings.TrimSpace(string(got)) != "" {
		t.Fatalf("reset should remove every curated setting:\n%s", got)
	}
	st, err := config.LoadState()
	if err != nil || st.ClaudeProxyAcknowledged {
		t.Fatalf("tuning must not count as acknowledging the proxy warning: %+v %v", st, err)
	}
}

func TestTuneInstructions(t *testing.T) {
	home, _ := setup(t)
	must(t, "tune", "instructions", "--yes")
	got, err := os.ReadFile(filepath.Join(home, ".agents", "AGENTS.md"))
	if err != nil || !strings.Contains(string(got), "## Judgment") {
		t.Fatalf("instructions: %v\n%s", err, got)
	}
	must(t, "tune", "instructions", "--reset", "--yes")
}
