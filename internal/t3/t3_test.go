package t3_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/testutil"
)

func fake(t *testing.T) *testutil.FakeT3 {
	t.Helper()
	f := testutil.T3(t, testutil.Home(t))
	prev := t3.DistTagsURL
	t3.DistTagsURL = f.Server.URL + "/dist-tags"
	t.Cleanup(func() { t3.DistTagsURL = prev })
	return f
}

func TestInspectRunningService(t *testing.T) {
	f := fake(t)
	st, err := t3.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed() || !st.ServiceInstalled || !st.Running || st.Version != testutil.T3Version || st.Label != "testbox" {
		t.Fatalf("status = %+v", st)
	}
	if st.Latest != testutil.T3Nightly || !st.UpdateAvailable() {
		t.Fatalf("update check: latest %q available %v", st.Latest, st.UpdateAvailable())
	}
	if !strings.HasPrefix(st.CLI, f.Home+"/runtime/versions/") {
		t.Fatalf("with no t3 on PATH the service runtime should be used, got %s", st.CLI)
	}
}

func TestInspectIgnoresAnotherHomesService(t *testing.T) {
	f := fake(t)
	// What t3 prints for a T3 home whose machine runs a service for another
	// home: installed, with no version here.
	f.Write(t, "status.json", `{"service":{"supported":true,"installed":true,"installedVersion":null}}`)
	st, err := t3.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.ServiceInstalled {
		t.Fatal("a service with no version for this home is not this home's service")
	}
	if st.Version == "" || st.OnLatest() {
		t.Fatalf("running %q, so it's behind %q: %+v", st.Version, st.Latest, st)
	}
}

func TestNewestCLIForSetupServiceVersionForAuth(t *testing.T) {
	f := fake(t)
	// A Mac with a desktop app on a newer nightly than its service.
	newer := filepath.Join(f.Home, "bin", "t3")
	f.AddCLI(t, newer, testutil.T3Nightly, "desktop")
	st, err := t3.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.CLI != newer {
		t.Fatalf("status should use the newest t3, got %s", st.CLI)
	}
	if _, err := t3.ListAccess(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := t3.Update(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls(t)
	for _, want := range []string{"desktop: status --json", "auth session list --json", "desktop: update --channel nightly --yes"} {
		if !slices.Contains(calls, want) {
			t.Errorf("missing call %q in %v", want, calls)
		}
	}
	if slices.Contains(calls, "desktop: auth session list --json") {
		t.Error("auth commands must use the service's version: a newer t3 would migrate its database")
	}
}

func TestInspectWithoutStatusCommand(t *testing.T) {
	f := fake(t)
	f.Write(t, "nostatus", "")
	st, err := t3.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.ServiceInstalled || st.ServiceVersion != testutil.T3Version || !st.Running {
		t.Fatalf("an old t3 without status should still read as installed and running: %+v", st)
	}
}

func TestInspectNotInstalled(t *testing.T) {
	testutil.Home(t)
	t.Setenv("PATH", "/usr/bin:/bin")
	prev := t3.DistTagsURL
	t3.DistTagsURL = "http://127.0.0.1:1/dist-tags"
	t.Cleanup(func() { t3.DistTagsURL = prev })
	st, err := t3.Inspect(context.Background())
	if err != nil || st.Installed() || st.Running {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if st.LatestErr == nil {
		t.Fatal("an unreachable registry should be reported, not hidden")
	}
}

func TestAccessListOrderAndIdle(t *testing.T) {
	f := fake(t)
	now := time.Now().UTC()
	ago := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }
	f.Write(t, "links.json", `[{"id":"l1","label":"phone","createdAt":"`+ago(time.Minute)+`","expiresAt":"`+ago(-4*time.Minute)+`"}]`)
	f.Write(t, "sessions.json", `[
	{"sessionId":"old","subject":"one-time-token","client":{"label":"laptop","os":"macOS","browser":"Chrome"},"issuedAt":"`+ago(40*24*time.Hour)+`","expiresAt":"`+ago(-time.Hour)+`","lastConnectedAt":"`+ago(9*24*time.Hour)+`"},
	{"sessionId":"fresh","subject":"one-time-token","client":{"label":"bot","deviceType":"bot"},"issuedAt":"`+ago(time.Hour)+`","expiresAt":"`+ago(-time.Hour)+`","lastConnectedAt":null},
	{"sessionId":"recent","subject":"one-time-token","client":{"label":"desk"},"issuedAt":"`+ago(20*24*time.Hour)+`","expiresAt":"`+ago(-time.Hour)+`","lastConnectedAt":"`+ago(2*time.Hour)+`"},
	{"sessionId":"desktop","subject":"desktop-bootstrap","client":{"label":"T3 Code desktop"},"issuedAt":"`+ago(50*24*time.Hour)+`","expiresAt":"`+ago(-time.Hour)+`","lastConnectedAt":"`+ago(30*24*time.Hour)+`"}
	]`)
	all, err := t3.ListAccess(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	var idle []string
	for _, a := range all {
		ids = append(ids, a.ID)
		if a.Idle(now) {
			idle = append(idle, a.ID)
		}
	}
	if !slices.Equal(ids, []string{"l1", "fresh", "recent", "old", "desktop"}) {
		t.Fatalf("order = %v", ids)
	}
	// The desktop app's own session is older but never idle: it is not a
	// paired device, and revoking it would cut the desktop app off.
	if !slices.Equal(idle, []string{"old"}) {
		t.Fatalf("idle = %v", idle)
	}
	if all[3].Device != "Chrome · macOS" || all[1].Device != "bot" {
		t.Fatalf("devices: %q %q", all[3].Device, all[1].Device)
	}

	if err := t3.Revoke(context.Background(), all[0]); err != nil {
		t.Fatal(err)
	}
	if err := t3.Revoke(context.Background(), all[3]); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls(t)
	if !slices.Contains(calls, "auth pairing revoke l1") || !slices.Contains(calls, "auth session revoke old") {
		t.Fatalf("calls = %v", calls)
	}
}

func TestPair(t *testing.T) {
	f := fake(t)
	p, err := t3.Pair(context.Background(), "phone", "https://box.example.ts.net", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != "https://box.example.ts.net/pair#token=TOKEN12CHARS" || p.Expires.IsZero() {
		t.Fatalf("pairing = %+v", p)
	}
	want := "auth pairing create --json --label phone --ttl 10m --base-url https://box.example.ts.net"
	if !slices.Contains(f.Calls(t), want) {
		t.Fatalf("calls = %v", f.Calls(t))
	}
}

func TestUpdateFollowsNightly(t *testing.T) {
	f := fake(t)
	if err := t3.Update(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := t3.Restart(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if calls := f.Calls(t); !slices.Contains(calls, "update --channel nightly --yes") || !slices.Contains(calls, "service restart") {
		t.Fatalf("calls = %v", calls)
	}
}

func TestErrorsCarryT3sMessage(t *testing.T) {
	f := fake(t)
	f.Write(t, "fail", "T3 Code service is not installed.")
	if err := t3.Restart(context.Background(), io.Discard); err == nil || !strings.HasSuffix(err.Error(), ": T3 Code service is not installed.") {
		t.Fatalf("restart error = %v", err)
	}
	if _, err := t3.ListAccess(context.Background()); err == nil || !strings.HasSuffix(err.Error(), ": T3 Code service is not installed.") {
		t.Fatalf("list error = %v", err)
	}
}

func TestLinkURLs(t *testing.T) {
	for _, c := range []struct{ raw, want string }{
		{"https://box.ts.net/", "https://box.ts.net"},
		{"\thttp://10.0.0.2:3773\n", "http://10.0.0.2:3773"},
	} {
		if got, err := t3.NormalizeURL(c.raw); err != nil || got != c.want {
			t.Errorf("NormalizeURL(%q) = %q, %v", c.raw, got, err)
		}
	}
	for _, bad := range []string{"box.ts.net", "https://box.ts.net/t3", "https://box.ts.net/?x=1"} {
		if _, err := t3.NormalizeURL(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if !t3.LocalOnly("http://127.0.0.1:3773") || t3.LocalOnly("https://box.ts.net") {
		t.Error("LocalOnly misjudged a URL")
	}
}

func TestUninstallChecksTheServiceIsGone(t *testing.T) {
	f := fake(t)
	f.Write(t, "stuck", "")
	if err := t3.Uninstall(context.Background(), io.Discard); err == nil || !strings.Contains(err.Error(), "left the background service installed") {
		t.Fatalf("t3 exits 0 without removing anything; that must not read as success: %v", err)
	}
	if err := os.Remove(filepath.Join(f.Dir, "stuck")); err != nil {
		t.Fatal(err)
	}
	if err := t3.Uninstall(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.Calls(t), "service uninstall") {
		t.Fatalf("calls = %v", f.Calls(t))
	}
	if err := t3.Uninstall(context.Background(), io.Discard); err == nil || !strings.Contains(err.Error(), "no T3 Code background service") {
		t.Fatalf("uninstalling twice should say there is nothing to remove: %v", err)
	}
}
