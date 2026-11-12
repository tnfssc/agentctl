package cli

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/cliproxy"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/testutil"
)

func setupCLIProxy(t *testing.T) *testutil.FakeCLIProxy {
	t.Helper()
	testutil.Home(t)
	f := testutil.CLIProxy(t)
	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	must(t, "cliproxy", "install", "--port", strconv.Itoa(port))
	return f
}

func TestCLIProxyNotInstalled(t *testing.T) {
	testutil.Home(t)
	testutil.CLIProxy(t)
	if out := must(t, "cliproxy"); !strings.Contains(out, "agentctl cliproxy install") {
		t.Fatalf("status:\n%s", out)
	}
	if _, err := execute(t, "cliproxy", "keys"); err == nil {
		t.Fatal("keys without an install must fail")
	}
}

func TestCLIProxyStatusAndKeys(t *testing.T) {
	setupCLIProxy(t)
	st, _ := cliproxy.Inspect(t.Context(), false)
	out := must(t, "cliproxy")
	for _, want := range []string{"running at " + st.URL(), "8.0.67", "on the newest release", "this machine only", "logins    none yet"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}

	out = must(t, "cliproxy", "keys")
	if !strings.Contains(out, st.APIKeys[0]) || !strings.Contains(out, st.ManagementKey) {
		t.Fatalf("keys:\n%s", out)
	}
	if out := must(t, "cliproxy", "keys", "--api"); out != st.APIKeys[0]+"\n" {
		t.Fatalf("--api = %q", out)
	}
	if out := must(t, "cliproxy", "keys", "--management"); out != st.ManagementKey+"\n" {
		t.Fatalf("--management = %q", out)
	}

	added := strings.TrimSpace(must(t, "cliproxy", "keys", "add"))
	must(t, "cliproxy", "keys", "rm", st.APIKeys[0])
	if out := must(t, "cliproxy", "keys", "--api"); strings.TrimSpace(out) != added {
		t.Fatalf("after rm, first key = %q, want %q", out, added)
	}
	if _, err := execute(t, "cliproxy", "keys", "rotate-management"); err == nil {
		t.Fatal("rotating without a terminal needs --yes")
	}
	// Progress goes to stderr, which tests capture with stdout; the key is last.
	lines := strings.Split(strings.TrimSpace(must(t, "cliproxy", "keys", "rotate-management", "--yes")), "\n")
	rotated := lines[len(lines)-1]
	if rotated == st.ManagementKey || strings.TrimSpace(must(t, "cliproxy", "keys", "--management")) != rotated {
		t.Fatal("rotate-management didn't take")
	}
}

func TestCLIProxyListenAndUninstall(t *testing.T) {
	f := setupCLIProxy(t)
	if out := must(t, "cliproxy", "listen", "all"); !strings.Contains(out, "every interface") {
		t.Fatalf("listen all:\n%s", out)
	}
	if _, err := execute(t, "cliproxy", "listen", "lan"); err == nil {
		t.Fatal("only local or all")
	}
	if _, err := execute(t, "cliproxy", "login", "codex"); err == nil {
		t.Fatal("logins need a terminal")
	}
	if _, err := execute(t, "cliproxy", "login", "nope"); err == nil {
		t.Fatal("unknown providers fail")
	}
	must(t, "cliproxy", "uninstall", "--yes", "--all")
	cfg, _ := config.Load()
	if _, ok := cfg.Proxies[cliproxy.DefaultProxy]; ok || cfg.CLIProxy.Proxy != "" || f.Running() {
		t.Fatalf("--all must forget the proxy and stop the server: %+v", cfg)
	}
}
