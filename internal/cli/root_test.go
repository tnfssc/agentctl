package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

func TestGuideAndTuneGit(t *testing.T) {
	home := testutil.Home(t)
	if out := must(t, "guide"); !strings.Contains(out, "signing") {
		t.Fatalf("guide list:\n%s", out)
	}
	if out := must(t, "guide", "signing"); !strings.Contains(out, "gpg.format ssh") || !strings.Contains(out, "ask before") {
		t.Fatalf("signing guide:\n%s", out)
	}
	if _, err := execute(t, "guide", "nope"); err == nil {
		t.Fatal("an unknown guide should fail")
	}
	out := must(t, "tune", "git", "--yes")
	if !strings.Contains(out, "agentctl guide signing") {
		t.Fatalf("unsigned commits should point at the guide:\n%s", out)
	}
	cfg, _ := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if !strings.Contains(string(cfg), "conflictStyle = zdiff3") {
		t.Fatalf(".gitconfig:\n%s", cfg)
	}
	must(t, "tune", "git", "--reset", "--yes")
	if _, err := os.Stat(filepath.Join(home, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("reset should remove the config agentctl created: %v", err)
	}
}

func TestUpdateRefusesSourceBuilds(t *testing.T) {
	testutil.Home(t)
	prev := version
	version = ""
	t.Cleanup(func() { version = prev })
	if _, err := execute(t, "update", "--yes"); err == nil || !strings.Contains(err.Error(), "built from source") {
		t.Fatalf("a dev build must not update itself: %v", err)
	}
}

func TestUpdateAllRunsEachToolsUpdater(t *testing.T) {
	home := testutil.Home(t)
	tf := testutil.T3(t, home)
	testutil.OpenCode(t, home)
	clis := testutil.AgentCLIs(t)
	prev := version
	version = "" // a source build: agentctl leaves itself alone
	t.Cleanup(func() { version = prev })

	out := must(t, "update", "--check", "all")
	for _, want := range []string{"Claude Code", "2.1.310 → 2.1.320", "Pi and its extensions", "extras may have updates", "agentctl", "built from source"} {
		if !strings.Contains(out, want) {
			t.Errorf("check lacks %q:\n%s", want, out)
		}
	}
	if len(clis.Calls(t)) != 0 {
		t.Fatal("--check must not update anything")
	}
	if _, err := execute(t, "update", "all"); err == nil {
		t.Fatal("without a terminal, updating needs --yes")
	}
	out, _ = execute(t, "update", "all", "--yes")
	calls := clis.Calls(t)
	if !slices.Contains(calls, "claude update") || !slices.Contains(calls, "pi update --all --no-approve") || slices.Contains(calls, "codex update") {
		t.Fatalf("only tools with news update: %v\n%s", calls, out)
	}
	if !slices.Contains(tf.Calls(t), "update --channel nightly --yes") || !strings.Contains(out, "updated OpenCode") {
		t.Fatalf("the servers update too:\n%s", out)
	}
	if _, err := execute(t, "update", "emacs"); err == nil {
		t.Fatal("unknown tools are refused")
	}
}

func TestTuneOnlyTurnsOnJustThoseItems(t *testing.T) {
	home := testutil.Home(t)
	must(t, "tune", "instructions", "--only", "web,mcp", "--yes")
	b, err := os.ReadFile(filepath.Join(home, ".agents", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "## Web") || !strings.Contains(got, "## MCP") || strings.Contains(got, "## Judgment") {
		t.Fatalf("only Web and MCP should be on:\n%s", got)
	}
	// What's on stays on.
	must(t, "tune", "instructions", "--only", "links", "--yes")
	if b, _ = os.ReadFile(filepath.Join(home, ".agents", "AGENTS.md")); !strings.Contains(string(b), "## Web") || !strings.Contains(string(b), "## Links") {
		t.Fatalf("earlier items must stay:\n%s", b)
	}
	if _, err := execute(t, "tune", "instructions", "--only", "nope", "--yes"); err == nil || !strings.Contains(err.Error(), "web") {
		t.Fatalf("unknown ids are refused with the list: %v", err)
	}
	if _, err := execute(t, "tune", "all", "--only", "web"); err == nil {
		t.Fatal("--only needs one target")
	}
}

func TestToolsInstallsWhatTheInstructionsPointAt(t *testing.T) {
	testutil.Home(t)
	fake := testutil.Tools(t, "uv", "brew", "npm")
	out := must(t, "tune", "all", "--dry-run")
	if !strings.Contains(out, "run uv tool install tavily-cli") || !strings.Contains(out, "run skill mcpc") {
		t.Fatalf("the recommended setup installs the tools too:\n%s", out)
	}
	if out = must(t, "tools", "--dry-run"); !strings.Contains(out, "run uv tool install tavily-cli") || len(fake.Calls()) > 0 {
		t.Fatalf("dry run lists, installs nothing (%v):\n%s", fake.Calls(), out)
	}
	if _, err := execute(t, "tools"); !errors.Is(err, errNeedsYes) {
		t.Fatalf("not a terminal needs --yes: %v", err)
	}
	must(t, "tools", "--yes")
	if len(fake.Calls()) != 2 {
		t.Fatalf("installed tvly and mcpc: %v", fake.Calls())
	}
	if out = must(t, "tools", "--yes"); !strings.Contains(out, "nothing to do") {
		t.Fatalf("second run: %s", out)
	}
}
