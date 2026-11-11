package tools_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/testutil"
	"github.com/tnfssc/agentctl/internal/tools"
)

func TestInstallsWhatIsMissingThenNothing(t *testing.T) {
	home := testutil.Home(t)
	fake := testutil.Tools(t, "uv", "pipx", "brew", "npm")
	if tools.Inspect().Ready() {
		t.Fatal("nothing is installed yet")
	}
	runs, warnings := tools.Plan()
	if len(warnings) > 0 {
		t.Fatalf("warnings %v", warnings)
	}
	plan := agent.Plan{Agent: tools.Name, Runs: runs}
	if err := plan.RunAll(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	mcpc := "brew install mcpc"
	if runtime.GOOS != "darwin" {
		mcpc = "npm install -g @apify/mcpc"
	}
	if got := fake.Calls(); !slices.Equal(got, []string{"uv tool install tavily-cli", mcpc}) {
		t.Fatalf("installers run: %q", got)
	}
	shared := filepath.Join(home, ".agents", "skills")
	for _, f := range []string{"tavily-search/SKILL.md", "tavily-search/references/setup.md", "mcpc/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(shared, f)); err != nil {
			t.Fatalf("skill file %s: %v", f, err)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(shared, "tavily-search")); len(entries) != 2 {
		t.Fatalf("only the skill folder, nothing else from the repo: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(home, "evil")); err == nil {
		t.Fatal("tar entries outside the skill folder are ignored")
	}
	if dst, _ := os.Readlink(filepath.Join(home, ".claude", "skills", "mcpc")); dst != filepath.Join(shared, "mcpc") {
		t.Fatalf("Claude Code links the shared skill, got %q", dst)
	}
	rec, err := os.ReadFile(filepath.Join(home, ".config", "agentctl", "guides", "tools.md"))
	if err != nil || !strings.Contains(string(rec), "uv tool install tavily-cli") || !strings.Contains(string(rec), "apify/mcpc") {
		t.Fatalf("record for tools-undo: %s %v", rec, err)
	}
	if !tools.Inspect().Ready() {
		t.Fatalf("installed: %+v", tools.Inspect())
	}
	if runs, _ := tools.Plan(); len(runs) != 0 {
		t.Fatalf("nothing left to do, got %d runs", len(runs))
	}
}

func TestUsersOwnSkillIsLinkedNotReplaced(t *testing.T) {
	home := testutil.Home(t)
	testutil.Tools(t, "uv", "npm", "brew")
	own := filepath.Join(home, ".agents", "skills", "mcpc", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	runs, _ := tools.Plan()
	var titles []string
	for _, r := range runs {
		titles = append(titles, r.Title)
	}
	if !slices.Contains(titles, "link ~/.claude/skills/mcpc") {
		t.Fatalf("runs %q", titles)
	}
	if err := (agent.Plan{Runs: runs}).RunAll(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(own); string(b) != "mine" {
		t.Fatalf("skill replaced: %q", b)
	}
}

func TestWithoutPackageManagersItSaysWhatsNeeded(t *testing.T) {
	testutil.Home(t)
	testutil.Tools(t)
	runs, warnings := tools.Plan()
	for _, r := range runs {
		if !strings.Contains(r.Title, "skill") && !strings.HasPrefix(r.Title, "link") {
			t.Fatalf("no installer, no install run: %q", r.Title)
		}
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "uv or pipx") || !strings.Contains(warnings[1], "npm") {
		t.Fatalf("warnings %q", warnings)
	}
}
