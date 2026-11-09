package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/testutil"
)

func setupT3(t *testing.T) (home string, f *testutil.FakeT3) {
	t.Helper()
	home = testutil.Home(t)
	f = testutil.T3(t, home)
	prev := t3.DistTagsURL
	t3.DistTagsURL = f.Server.URL + "/dist-tags"
	t.Cleanup(func() { t3.DistTagsURL = prev })
	return home, f
}

func TestT3StatusAndUpdate(t *testing.T) {
	_, f := setupT3(t)
	out := must(t, "t3")
	for _, want := range []string{"running as testbox", testutil.T3Version, testutil.T3Nightly, "pair at  " + f.Server.URL} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}
	if _, err := execute(t, "t3", "update"); err == nil {
		t.Fatal("updating without a terminal needs --yes")
	}
	must(t, "t3", "update", "--yes")
	if !slices.Contains(f.Calls(t), "update --channel nightly --yes") {
		t.Fatalf("calls = %v", f.Calls(t))
	}
}

func TestT3PairUsesConfiguredURL(t *testing.T) {
	home, f := setupT3(t)
	must(t, "t3", "url", "https://box.example.ts.net/")
	cfg, _ := os.ReadFile(filepath.Join(home, ".config", "agentctl", "config.toml"))
	if !strings.Contains(string(cfg), "[t3]\nurl = 'https://box.example.ts.net'") {
		t.Fatalf("config:\n%s", cfg)
	}
	out := must(t, "t3", "pair", "--label", "phone")
	if !strings.Contains(out, "https://box.example.ts.net/pair#token=TOKEN12CHARS") || strings.Contains(out, "only works on this machine") {
		t.Fatalf("pair output:\n%s", out)
	}

	must(t, "t3", "url", "--clear")
	cfg, _ = os.ReadFile(filepath.Join(home, ".config", "agentctl", "config.toml"))
	if strings.Contains(string(cfg), "t3") {
		t.Fatalf("a cleared URL should leave no [t3] table:\n%s", cfg)
	}
	out = must(t, "t3", "pair")
	if !strings.Contains(out, f.Server.URL+"/pair#token=") || !strings.Contains(out, "only works on this machine") {
		t.Fatalf("pair output with the listen address:\n%s", out)
	}
}

func TestT3RevokeIdle(t *testing.T) {
	_, f := setupT3(t)
	old := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	f.Write(t, "sessions.json", `[
	{"sessionId":"aaaa1111-old","subject":"one-time-token","client":{"label":"tablet"},"issuedAt":"`+old+`","expiresAt":"2030-01-01T00:00:00Z","lastConnectedAt":"`+old+`"},
	{"sessionId":"bbbb2222-new","subject":"one-time-token","client":{"label":"desk","os":"Linux","browser":"Chrome"},"issuedAt":"`+recent+`","expiresAt":"2030-01-01T00:00:00Z","lastConnectedAt":"`+recent+`"}
	]`)
	out := must(t, "t3", "devices")
	if !strings.Contains(out, "Chrome · Linux") || !strings.Contains(out, "1 idle") {
		t.Fatalf("devices:\n%s", out)
	}
	must(t, "t3", "revoke", "--idle", "--yes")
	calls := f.Calls(t)
	if !slices.Contains(calls, "auth session revoke aaaa1111-old") || slices.Contains(calls, "auth session revoke bbbb2222-new") {
		t.Fatalf("calls = %v", calls)
	}
	must(t, "t3", "revoke", "bbbb", "--yes")
	if !slices.Contains(f.Calls(t), "auth session revoke bbbb2222-new") {
		t.Fatalf("revoke by prefix: %v", f.Calls(t))
	}
	if _, err := execute(t, "t3", "revoke", "zzzz", "--yes"); err == nil {
		t.Fatal("an unknown id should fail")
	}
}
