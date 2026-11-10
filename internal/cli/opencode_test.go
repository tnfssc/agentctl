package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/testutil"
)

func TestOpenCodeStatusAndPair(t *testing.T) {
	home := testutil.Home(t)
	f := testutil.OpenCode(t, home)
	out := must(t, "opencode")
	for _, want := range []string{"running at http://127.0.0.1:49374", testutil.OpenCodeVersion, "update   " + testutil.OpenCodeLatest, "pair at  http://127.0.0.1:49374"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}

	out = must(t, "opencode", "pair")
	if !strings.Contains(out, "http://127.0.0.1:49374/auth/connect/CODE") || !strings.Contains(out, "only works on this machine") {
		t.Fatalf("pair with the listen address:\n%s", out)
	}

	must(t, "opencode", "url", "https://box.example.ts.net/")
	cfg, _ := os.ReadFile(filepath.Join(home, ".config", "agentctl", "config.toml"))
	if !strings.Contains(string(cfg), "[opencode]\nurl = 'https://box.example.ts.net'") {
		t.Fatalf("config:\n%s", cfg)
	}
	out = must(t, "opencode", "pair")
	if !strings.Contains(out, "https://box.example.ts.net/auth/connect/CODE") || strings.Contains(out, "only works on this machine") {
		t.Fatalf("pair output:\n%s", out)
	}
	if !slices.Contains(f.Calls(t), "pair --url https://box.example.ts.net") {
		t.Fatalf("calls = %v", f.Calls(t))
	}
	if _, err := execute(t, "opencode", "pair", "--url", "https://box.example.ts.net/web"); err == nil {
		t.Fatal("an address with a path should be refused")
	}
}

func TestOpenCodeSteps(t *testing.T) {
	f := testutil.OpenCode(t, testutil.Home(t))
	if _, err := execute(t, "opencode", "signout"); err == nil {
		t.Fatal("signing out without a terminal needs --yes")
	}
	must(t, "opencode", "signout", "--yes")
	must(t, "opencode", "stop", "--yes")
	if out := must(t, "opencode"); !strings.Contains(out, "stopped") {
		t.Fatalf("status after stop:\n%s", out)
	}
	must(t, "opencode", "start")
	must(t, "opencode", "update", "--yes")
	calls := f.Calls(t)
	for _, want := range []string{"service unset password", "service stop", "service start", "upgrade"} {
		if !slices.Contains(calls, want) {
			t.Fatalf("calls = %v, missing %q", calls, want)
		}
	}

	f.Write(t, "version", "1.1.4")
	if _, err := execute(t, "opencode", "start"); err == nil || !strings.Contains(err.Error(), "needs v2") {
		t.Fatalf("v1 should be refused: %v", err)
	}
}

func TestUseOpenCodeTurnsOnClaudeAgent(t *testing.T) {
	home, url := setup(t)
	f := testutil.OpenCode(t, home)
	must(t, "proxy", "add", "spark", "--url", url+"/v1", "--key", testutil.Key)
	out := must(t, "use", "opencode", "spark", "--yes")
	if !strings.Contains(out, "Turned on OpenCode's claude-agent") {
		t.Fatalf("use output:\n%s", out)
	}
	// Already connected, and the route already in place: nothing more to do.
	out = must(t, "use", "opencode", "spark", "--yes")
	logins := 0
	for _, c := range f.Calls(t) {
		if c == "auth login claude-agent --method claude-login" {
			logins++
		}
	}
	if logins != 1 || strings.Contains(out, "Turned on") {
		t.Fatalf("logins = %d, output:\n%s", logins, out)
	}
}
