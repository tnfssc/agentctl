package upgrade_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/testutil"
	"github.com/tnfssc/agentctl/internal/upgrade"
)

func TestInspectReadsVersionsAndNewestReleases(t *testing.T) {
	testutil.AgentCLIs(t)
	got := map[string]upgrade.Status{}
	for _, s := range upgrade.InspectAll(context.Background()) {
		got[s.Name] = s
	}
	if c := got["claude"]; c.Version != "2.1.310" || c.Latest != "2.1.320" || !c.UpdateAvailable() {
		t.Fatalf("claude %+v", c)
	}
	if c := got["codex"]; c.Version != "0.164.0" || c.UpdateAvailable() || c.Worth() {
		t.Fatalf("codex is current: %+v", c)
	}
	// Pi is current, but its extensions may not be.
	if p := got["pi"]; p.UpdateAvailable() || !p.Worth() {
		t.Fatalf("pi %+v", p)
	}
}

func TestMissingCLIIsNotAnError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	s := upgrade.Inspect(context.Background(), upgrade.CLIs[0])
	if s.Installed() || s.Worth() || s.LatestErr != nil {
		t.Fatalf("%+v", s)
	}
}

func TestUpdateRunsTheCLIsOwnUpdater(t *testing.T) {
	f := testutil.AgentCLIs(t)
	ctx := context.Background()
	pi, _ := upgrade.Find("pi")
	if err := upgrade.Update(ctx, io.Discard, pi); err != nil {
		t.Fatal(err)
	}
	codex, _ := upgrade.Find("codex")
	if err := upgrade.Update(ctx, io.Discard, codex); err == nil || !strings.Contains(err.Error(), "EACCES") {
		t.Fatalf("a failed update should say why: %v", err)
	}
	if calls := f.Calls(t); !slices.Equal(calls, []string{"pi update --all --no-approve", "codex update"}) {
		t.Fatalf("calls = %v", calls)
	}
}
