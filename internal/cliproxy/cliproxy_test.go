package cliproxy_test

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/cliproxy"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/testutil"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func install(t *testing.T) (*testutil.FakeCLIProxy, cliproxy.Status) {
	t.Helper()
	testutil.Home(t)
	f := testutil.CLIProxy(t)
	ctx := context.Background()
	if err := cliproxy.Install(ctx, io.Discard, cliproxy.InstallOptions{Port: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	st, err := cliproxy.Inspect(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return f, st
}

func TestInstallSeedsKeysStartsAndAttaches(t *testing.T) {
	f, st := install(t)
	if !st.Installed || st.Version != "8.0.67" || !st.Service || !st.Enabled || !st.Running || !f.Running() {
		t.Fatalf("status = %+v", st)
	}
	if st.Host != "127.0.0.1" || st.AllInterfaces() {
		t.Fatalf("a new server must listen on this machine only, host = %q", st.Host)
	}
	if len(st.APIKeys) != 1 || !strings.HasPrefix(st.APIKeys[0], "sk-") || len(st.APIKeys[0]) != 51 {
		t.Fatalf("api keys = %v; the example's placeholders must go", st.APIKeys)
	}
	if st.ManagementKey == "" || !st.ManagementSet {
		t.Fatalf("management key = %q, set = %v", st.ManagementKey, st.ManagementSet)
	}
	if !st.OnLatest() {
		t.Fatalf("latest = %q, version = %q", st.Latest, st.Version)
	}
	mp, _ := cliproxy.ManagementKeyPath()
	if fi, err := os.Stat(mp); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("management key copy: %v %v", fi, err)
	}
	cp, _ := cliproxy.ConfigPath()
	if fi, _ := os.Stat(cp); fi.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v", fi.Mode().Perm())
	}
	if b, _ := os.ReadFile(cp); !strings.Contains(string(b), "# Management key. If a plaintext value") {
		t.Fatalf("edits must keep the example's comments:\n%s", b)
	}
	svc, _ := cliproxy.ServicePath()
	b, err := os.ReadFile(svc)
	if err != nil {
		t.Fatal(err)
	}
	bin, _ := cliproxy.Bin()
	if !strings.Contains(string(b), bin) || !strings.Contains(string(b), cp) {
		t.Fatalf("service file doesn't run the binary with its config:\n%s", b)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Proxies[cliproxy.DefaultProxy]
	if p.URL != st.URL() || p.Key != st.APIKeys[0] || cfg.CLIProxy.Proxy != cliproxy.DefaultProxy {
		t.Fatalf("proxy = %+v, cliproxy = %+v", p, cfg.CLIProxy)
	}
	if err := cliproxy.Install(context.Background(), io.Discard, cliproxy.InstallOptions{}); err == nil {
		t.Fatal("a second install must point at update")
	}
}

func TestTakenPortStopsInstall(t *testing.T) {
	testutil.Home(t)
	testutil.CLIProxy(t)
	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	err = cliproxy.Install(context.Background(), io.Discard, cliproxy.InstallOptions{Port: port})
	if err == nil || !strings.Contains(err.Error(), "--port") {
		t.Fatalf("err = %v", err)
	}
	if st, _ := cliproxy.Inspect(context.Background(), false); st.Installed {
		t.Fatal("nothing should be installed")
	}
}

func TestKeys(t *testing.T) {
	_, st := install(t)
	first := st.APIKeys[0]
	if err := cliproxy.RemoveKey(first); err == nil {
		t.Fatal("the only key must stay")
	}
	second, err := cliproxy.AddKey()
	if err != nil {
		t.Fatal(err)
	}
	// OpenCode keeps its own copy of the proxy's key.
	kp := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "agentctl", "keys", cliproxy.DefaultProxy)
	if err := os.MkdirAll(filepath.Dir(kp), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kp, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cliproxy.RemoveKey(first); err != nil {
		t.Fatal(err)
	}
	st, _ = cliproxy.Inspect(context.Background(), false)
	if !slices.Equal(st.APIKeys, []string{second}) {
		t.Fatalf("keys = %v", st.APIKeys)
	}
	cfg, _ := config.Load()
	if cfg.Proxies[cliproxy.DefaultProxy].Key != second {
		t.Fatal("the attached proxy must move to a remaining key")
	}
	if b, _ := os.ReadFile(kp); string(b) != second {
		t.Fatalf("OpenCode's key copy = %q", b)
	}
}

func TestRotateManagementAndListen(t *testing.T) {
	f, st := install(t)
	old := st.ManagementKey
	key, err := cliproxy.RotateManagement(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	st, _ = cliproxy.Inspect(context.Background(), false)
	if key == old || st.ManagementKey != key {
		t.Fatalf("rotated %q -> %q, copy %q", old, key, st.ManagementKey)
	}
	if err := cliproxy.Listen(context.Background(), io.Discard, true); err != nil {
		t.Fatal(err)
	}
	st, _ = cliproxy.Inspect(context.Background(), false)
	if !st.AllInterfaces() {
		t.Fatalf("host = %q", st.Host)
	}
	restarts := 0
	for _, c := range f.Calls(t) {
		if strings.Contains(c, "kickstart -k") || strings.Contains(c, "--user restart") {
			restarts++
		}
	}
	if restarts != 2 {
		t.Fatalf("both changes need a restart; calls = %v", f.Calls(t))
	}
}

func TestStopStartUninstall(t *testing.T) {
	f, _ := install(t)
	ctx := context.Background()
	if err := cliproxy.Stop(ctx, io.Discard); err != nil {
		t.Fatal(err)
	}
	st, _ := cliproxy.Inspect(ctx, false)
	if st.Enabled || st.Running || !st.Service {
		t.Fatalf("after stop: %+v", st)
	}
	if err := cliproxy.Start(ctx, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !f.Running() {
		t.Fatal("start didn't start")
	}
	if err := cliproxy.Uninstall(ctx, false); err != nil {
		t.Fatal(err)
	}
	st, _ = cliproxy.Inspect(ctx, false)
	cp, _ := cliproxy.ConfigPath()
	if st.Installed || st.Service || f.Running() || !fileExists(cp) {
		t.Fatalf("uninstall should keep only the config: %+v", st)
	}
	if err := cliproxy.Uninstall(ctx, true); err != nil {
		t.Fatal(err)
	}
	mp, _ := cliproxy.ManagementKeyPath()
	if fileExists(cp) || fileExists(mp) {
		t.Fatal("--all removes the config and the management key copy")
	}
}

func TestReinstallKeepsConfig(t *testing.T) {
	_, st := install(t)
	ctx := context.Background()
	if err := cliproxy.Uninstall(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := cliproxy.Install(ctx, io.Discard, cliproxy.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	again, _ := cliproxy.Inspect(ctx, false)
	if !slices.Equal(again.APIKeys, st.APIKeys) || again.ManagementKey != st.ManagementKey || again.Port != st.Port {
		t.Fatalf("reinstall changed keys or port: %+v -> %+v", st, again)
	}
}

func TestLoginsAndLoginCmd(t *testing.T) {
	f, st := install(t)
	if err := os.MkdirAll(st.AuthDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"codex-a.json": `{"type":"codex","email":"a@example.com","access_token":"x"}`,
		"xai.json":     `{"type":"xai","label":"grok"}`,
		"junk.json":    `not json`,
		"notes.txt":    `{"type":"codex"}`,
	} {
		if err := os.WriteFile(filepath.Join(st.AuthDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	st, _ = cliproxy.Inspect(context.Background(), false)
	if len(st.Logins) != 2 || st.Logins[0].Account != "a@example.com" || st.Logins[1].Account != "grok" {
		t.Fatalf("logins = %+v", st.Logins)
	}
	p, _ := cliproxy.FindProvider("codex-device")
	cmd, err := cliproxy.LoginCmd(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "https://auth.example/login") {
		t.Fatalf("%s %v", out, err)
	}
	cp, _ := cliproxy.ConfigPath()
	if !slices.Contains(f.Calls(t), "cli-proxy-api -config "+cp+" -codex-device-login -no-browser") {
		t.Fatalf("calls = %v", f.Calls(t))
	}
	if p, ok := cliproxy.FindProvider("claude"); !ok || p.Risk == "" {
		t.Fatal("Claude is offered, behind its warning")
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
