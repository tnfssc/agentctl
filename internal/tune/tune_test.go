package tune

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/tnfssc/agentctl/internal/testutil"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, agentName string, want map[string]bool) {
	t.Helper()
	plan, err := Plan(agentName, want)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
}

func recommended(t *testing.T, agentName string) map[string]bool {
	t.Helper()
	st, err := Inspect(agentName)
	if err != nil {
		t.Fatal(err)
	}
	return Recommended(st, false)
}

func TestClaudeRecommendedThenReset(t *testing.T) {
	home := testutil.Home(t)
	path := filepath.Join(home, ".claude", "settings.json")
	write(t, path, `{
  "theme": "dark",
  "env": {
    "DISABLE_TELEMETRY": "1",
    "ANTHROPIC_BASE_URL": "https://proxy"
  },
  "permissions": {
    "deny": [
      "Bash(rm -rf *)",
      "NotebookEdit"
    ]
  }
}
`)
	run(t, "claude", recommended(t, "claude"))
	got, _ := os.ReadFile(path)
	deny := gjson.GetBytes(got, "permissions.deny").String()
	for _, tool := range []string{"Bash(rm -rf *)", "Artifact", "EnterPlanMode", "DesignSync"} {
		if !strings.Contains(deny, tool) {
			t.Errorf("deny list missing %q: %s", tool, deny)
		}
	}
	if strings.Count(deny, "NotebookEdit") != 1 {
		t.Errorf("existing entries must not be duplicated: %s", deny)
	}
	for path, want := range map[string]string{
		"env.DISABLE_TELEMETRY": "", "env.DISABLE_ERROR_REPORTING": "1", "env.ANTHROPIC_BASE_URL": "https://proxy",
		"enableWorkflows": "false", "feedbackDrafts": "off", "permissions.defaultMode": "",
	} {
		if v := gjson.GetBytes(got, path).String(); v != want {
			t.Errorf("%s = %q, want %q", path, v, want)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "agentctl", "env.sh")); err == nil {
		t.Error("recommended items must not touch shell variables")
	}

	statuses, err := Inspect("claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range statuses {
		if st.Applied != (st.Recommended && !st.Risky) {
			t.Errorf("%s applied = %v", st.ID, st.Applied)
		}
	}

	run(t, "claude", nil)
	got, _ = os.ReadFile(path)
	want := `{
  "theme": "dark",
  "env": {
    "ANTHROPIC_BASE_URL": "https://proxy"
  },
  "permissions": {
    "deny": [
      "Bash(rm -rf *)"
    ]
  }
}
`
	if string(got) != want {
		t.Fatalf("reset to defaults left:\n%s", got)
	}
}

func TestClaudeNoApprovalsWritesShellEnv(t *testing.T) {
	home := testutil.Home(t)
	if err := os.MkdirAll(filepath.Join(home, ".config", "fish"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CODE_EVEN_MORE_DANGEROUSLY_SKIP_PERMISSIONS", "")
	plan, err := Plan("claude", map[string]bool{"no-approvals": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) == 0 || !strings.Contains(plan.Warnings[0], "env.sh") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	settings, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if gjson.GetBytes(settings, "permissions.defaultMode").String() != "bypassPermissions" ||
		!gjson.GetBytes(settings, "skipDangerousModePermissionPrompt").Bool() {
		t.Fatalf("settings: %s", settings)
	}
	sh, _ := os.ReadFile(filepath.Join(home, ".config", "agentctl", "env.sh"))
	fish, _ := os.ReadFile(filepath.Join(home, ".config", "fish", "conf.d", "agentctl.fish"))
	if !strings.Contains(string(sh), "export CLAUDE_CODE_EVEN_MORE_DANGEROUSLY_SKIP_PERMISSIONS=1") ||
		!strings.Contains(string(fish), "set -gx CLAUDE_CODE_EVEN_MORE_DANGEROUSLY_SKIP_PERMISSIONS 1") {
		t.Fatalf("snippets:\n%s\n%s", sh, fish)
	}
	if st, _ := Inspect("claude"); !st[len(st)-1].Applied {
		t.Fatal("no-approvals should read as applied")
	}

	run(t, "claude", nil)
	sh, _ = os.ReadFile(filepath.Join(home, ".config", "agentctl", "env.sh"))
	if strings.Contains(string(sh), "export") {
		t.Fatalf("reset should remove the variable:\n%s", sh)
	}
	if settings, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); len(settings) != 0 && strings.TrimSpace(string(settings)) != "{}" {
		t.Fatalf("reset should leave no settings: %s", settings)
	}
}

func TestCodexRecommendedThenReset(t *testing.T) {
	home := testutil.Home(t)
	path := filepath.Join(home, ".codex", "config.toml")
	original := `# mine
model = "gpt-6.1-sol"
approval_policy = "never" # yolo

[features]
hooks = true
`
	write(t, path, original)
	statuses, err := Inspect("codex")
	if err != nil {
		t.Fatal(err)
	}
	if statuses[3].Applied {
		t.Fatal("never + default sandbox is not the full no-approvals item")
	}
	run(t, "codex", Recommended(statuses, false))
	got, _ := os.ReadFile(path)
	want := `# mine
model = "gpt-6.1-sol"
approval_policy = "never" # yolo

[features]
hooks = true
image_generation = false
tool_suggest = false

[analytics]
enabled = false
`
	if string(got) != want {
		t.Fatalf("got:\n%s", got)
	}
	run(t, "codex", nil)
	if got, _ := os.ReadFile(path); string(got) != original {
		t.Fatalf("reset should restore the original file, got:\n%s", got)
	}
}

func TestInstructionsSectionsAndLinks(t *testing.T) {
	home := testutil.Home(t)
	path := filepath.Join(home, ".agents", "AGENTS.md")
	mine := "# My rules\n\nUse tabs.\n"
	write(t, path, mine)

	statuses, err := Inspect(Instructions)
	if err != nil {
		t.Fatal(err)
	}
	run(t, Instructions, Recommended(statuses, false))

	got, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(got), mine) || !strings.Contains(string(got), "<!-- agentctl:judgment -->") ||
		!strings.Contains(string(got), "`<repo>/.agents/notes/") {
		t.Fatalf("instructions file:\n%s", got)
	}
	if target, err := os.Readlink(filepath.Join(home, ".claude", "CLAUDE.md")); err != nil || target != path {
		t.Fatalf("Claude Code should be linked to %s: %q %v", path, target, err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".codex", "AGENTS.md")); err == nil {
		t.Fatal("Codex needs no link")
	}
	if st, _ := Inspect(Instructions); !st[0].Applied {
		t.Fatal("applied sections should read as applied")
	}

	run(t, Instructions, nil)
	if got, _ := os.ReadFile(path); string(got) != mine {
		t.Fatalf("reset should leave only the user's text, got:\n%s", got)
	}
}

func TestInstructionsImportIntoSeparateClaudeFile(t *testing.T) {
	home := testutil.Home(t)
	claudeMD := filepath.Join(home, ".claude", "CLAUDE.md")
	mine := "# my own\n"
	write(t, claudeMD, mine)
	run(t, Instructions, Recommended(must(Inspect(Instructions)), false))
	b, _ := os.ReadFile(claudeMD)
	if !strings.HasPrefix(string(b), mine) || !strings.Contains(string(b), "\n@~/.agents/AGENTS.md\n") {
		t.Fatalf("CLAUDE.md should keep its text and import the shared file:\n%s", b)
	}
	if plan, err := Plan(Instructions, Recommended(must(Inspect(Instructions)), false)); err != nil || !plan.Empty() {
		t.Fatalf("applying again should change nothing: %v\n%s", err, plan.Diff())
	}
	run(t, Instructions, nil)
	if b, _ := os.ReadFile(claudeMD); string(b) != mine {
		t.Fatalf("reset should remove the import, got:\n%s", b)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestGitRecommendedThenReset(t *testing.T) {
	home := testutil.Home(t)
	path := filepath.Join(home, ".gitconfig")
	mine := "[user]\n\tname = Someone\n[pull]\n\trebase = false\n"
	write(t, path, mine)

	statuses, err := Inspect(Git)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].Applied {
		t.Fatal("pull.rebase alone is not the whole merge-on-pull item")
	}
	run(t, Git, Recommended(statuses, false))
	got, _ := os.ReadFile(path)
	for _, want := range []string{"name = Someone", "ff = true", "autoSetupRemote = true", "conflictStyle = zdiff3", "enabled = true", "algorithm = histogram", "prune = true"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("gitconfig missing %q:\n%s", want, got)
		}
	}
	if st, _ := Inspect(Git); !st[0].Applied || !st[len(st)-1].Applied {
		t.Fatal("applied items should read as applied")
	}
	if plan, err := Plan(Git, Recommended(must(Inspect(Git)), false)); err != nil || !plan.Empty() {
		t.Fatalf("applying again should change nothing: %v\n%s", err, plan.Diff())
	}

	run(t, Git, map[string]bool{})
	if got, _ := os.ReadFile(path); string(got) != "[user]\n\tname = Someone\n" {
		t.Fatalf("reset unsets every managed key, the user's pull.rebase included, and keeps the rest:\n%s", got)
	}
}

func TestGitWithoutConfigCreatesNothingOnReset(t *testing.T) {
	home := testutil.Home(t)
	run(t, Git, map[string]bool{"prune": true})
	run(t, Git, map[string]bool{})
	if _, err := os.Stat(filepath.Join(home, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("a config agentctl created and emptied should be gone: %v", err)
	}
}

func TestRecommendedIsEverythingButRiskyNeedsItsOwnYes(t *testing.T) {
	testutil.Home(t)
	for _, target := range Targets {
		statuses := must(Inspect(target))
		safe, all := Recommended(statuses, false), Recommended(statuses, true)
		for _, st := range statuses {
			if !all[st.ID] {
				t.Errorf("%s: %s should be recommended", target, st.ID)
			}
			if safe[st.ID] == st.Risky {
				t.Errorf("%s: %s risky=%v but in the safe set=%v", target, st.ID, st.Risky, safe[st.ID])
			}
		}
	}
	risky, err := RiskyAll()
	if err != nil || len(risky) != 2 {
		t.Fatalf("Claude Code's and Codex's no-approvals are the risky ones: %v %v", risky, err)
	}
	plan, err := PlanAll(false)
	if err != nil || plan.Agent != All || plan.Empty() {
		t.Fatalf("plan %+v %v", plan, err)
	}
	for _, c := range plan.Changes {
		if strings.Contains(string(c.After), "bypassPermissions") || strings.Contains(string(c.After), "danger-full-access") {
			t.Fatalf("risky settings slipped into the safe plan: %s", c.Path)
		}
	}
}
