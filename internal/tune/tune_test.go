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
	return Recommended(st)
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
		if st.Applied != st.Recommended {
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
	run(t, "codex", Recommended(statuses))
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
	codexLink := filepath.Join(home, ".codex", "AGENTS.md")
	write(t, codexLink, "codex has its own file\n")

	statuses, err := Inspect(Instructions)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses[0].Notes) != 1 || !strings.Contains(statuses[0].Notes[0], codexLink) {
		t.Fatalf("want a note about the separate Codex file, got %v", statuses[0].Notes)
	}
	run(t, Instructions, Recommended(statuses))

	got, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(got), mine) || !strings.Contains(string(got), "<!-- agentctl:judgment -->") ||
		!strings.Contains(string(got), "`<repo>/.agents/notes/") {
		t.Fatalf("instructions file:\n%s", got)
	}
	if target, err := os.Readlink(filepath.Join(home, ".claude", "CLAUDE.md")); err != nil || target != path {
		t.Fatalf("Claude Code should be linked to %s: %q %v", path, target, err)
	}
	if b, _ := os.ReadFile(codexLink); string(b) != "codex has its own file\n" {
		t.Fatal("an existing agent file must be left alone")
	}
	if st, _ := Inspect(Instructions); !st[0].Applied {
		t.Fatal("applied sections should read as applied")
	}

	run(t, Instructions, nil)
	if got, _ := os.ReadFile(path); string(got) != mine {
		t.Fatalf("reset should leave only the user's text, got:\n%s", got)
	}
}
