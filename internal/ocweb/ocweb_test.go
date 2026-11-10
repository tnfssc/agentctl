package ocweb_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tnfssc/agentctl/internal/ocweb"
	"github.com/tnfssc/agentctl/internal/testutil"
)

func TestInspect(t *testing.T) {
	ctx := context.Background()
	f := testutil.OpenCode(t, testutil.Home(t))
	st, err := ocweb.Inspect(ctx)
	if err != nil || !st.Running || st.URL != "http://127.0.0.1:49374" || st.Version != testutil.OpenCodeVersion || st.Serving != "" {
		t.Fatalf("running: %+v %v", st, err)
	}
	if st.Latest != testutil.OpenCodeLatest || !st.UpdateAvailable() {
		t.Fatalf("latest = %q, err %v", st.Latest, st.LatestErr)
	}

	// Upgraded without a restart: the server still runs the old version.
	f.Register(t, "2.2.5")
	if st, _ = ocweb.Inspect(ctx); st.Serving != "2.2.5" {
		t.Fatalf("serving = %q", st.Serving)
	}

	f.Write(t, "url", "")
	if st, _ = ocweb.Inspect(ctx); st.Running || st.URL != "" || st.Serving != "" {
		t.Fatalf("stopped: %+v", st)
	}

	f.Write(t, "version", "1.1.4")
	calls := len(f.Calls(t))
	st, err = ocweb.Inspect(ctx)
	if err != nil || !st.Installed() || st.Supported() {
		t.Fatalf("v1 has no server: %+v %v", st, err)
	}
	if len(f.Calls(t)) != calls {
		t.Fatalf("v1 should not be asked about a server: %v", f.Calls(t))
	}

	t.Setenv("PATH", "/nonexistent")
	if st, err = ocweb.Inspect(ctx); err != nil || st.Installed() {
		t.Fatalf("not installed: %+v %v", st, err)
	}
}

func TestPairReadsTheLink(t *testing.T) {
	testutil.OpenCode(t, testutil.Home(t))
	before := time.Now()
	p, err := ocweb.Pair(context.Background(), "https://box.example.ts.net")
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != "https://box.example.ts.net/auth/connect/CODE22CHARSxxxxxxxxxxx" {
		t.Fatalf("url = %q", p.URL)
	}
	if d := p.Expires.Sub(before); d < 5*time.Minute || d > 5*time.Minute+time.Minute {
		t.Fatalf("expires in %v", d)
	}
}

func TestStepsAndErrors(t *testing.T) {
	ctx := context.Background()
	f := testutil.OpenCode(t, testutil.Home(t))
	var out strings.Builder
	if err := ocweb.SignOutAll(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if err := ocweb.Update(ctx, &out); err != nil {
		t.Fatal(err)
	}
	want := []string{"service unset password", "service start", "service status", "upgrade", "service restart"}
	if got := f.Calls(t); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), "Upgrade complete") {
		t.Fatalf("output = %q", out.String())
	}

	// A stopped server stays stopped through an update.
	if err := ocweb.Stop(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if err := ocweb.Update(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if calls := f.Calls(t); calls[len(calls)-1] != "upgrade" {
		t.Fatalf("calls = %v", calls)
	}

	f.Write(t, "fail", "Port must be between 1 and 65535")
	err := ocweb.Restart(ctx, &out)
	if err == nil || err.Error() != "opencode service restart: Port must be between 1 and 65535" {
		t.Fatalf("err = %v", err)
	}

	t.Setenv("PATH", "/nonexistent")
	if err := ocweb.Start(ctx, &out); !errors.Is(err, ocweb.ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
}

func TestConnectClaude(t *testing.T) {
	ctx := context.Background()
	f := testutil.OpenCode(t, testutil.Home(t))
	if on, err := ocweb.ConnectClaude(ctx); !on || err != nil {
		t.Fatalf("first connect: %v %v", on, err)
	}
	if on, err := ocweb.ConnectClaude(ctx); on || err != nil {
		t.Fatalf("already connected: %v %v", on, err)
	}
	f.Write(t, "version", "1.1.4")
	if on, err := ocweb.ConnectClaude(ctx); on || err != nil {
		t.Fatalf("v1 has no claude-agent: %v %v", on, err)
	}
}
