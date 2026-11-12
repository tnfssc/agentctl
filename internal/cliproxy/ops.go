package cliproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/tnfssc/agentctl/internal/config"
)

// ErrNotInstalled means agentctl hasn't installed the server here.
var ErrNotInstalled = errors.New(Title + " is not installed here; run `agentctl cliproxy install`")

// InstallOptions are the choices made once, at install.
type InstallOptions struct {
	Port  int    // 0 means DefaultPort
	Proxy string // name to attach it under; "" means DefaultProxy
}

// Install downloads the newest release, seeds its config with fresh keys
// unless one is already there, installs the service, starts it and
// attaches it as a proxy. Progress goes to out.
func Install(ctx context.Context, out io.Writer, opts InstallOptions) error {
	if st, err := Inspect(ctx, false); err != nil {
		return err
	} else if st.Installed {
		return errors.New(Title + " is already installed; `agentctl cliproxy update` gets the newest release")
	}
	if opts.Proxy == "" {
		opts.Proxy = DefaultProxy
	}
	if err := config.ValidName(opts.Proxy); err != nil {
		return err
	}
	cfgPath, err := ConfigPath()
	if err != nil {
		return err
	}
	seeding := !fileExists(cfgPath)
	port := opts.Port
	if port == 0 {
		port = DefaultPort
	}
	if seeding || opts.Port != 0 {
		if err := portFree(ctx, port); err != nil {
			return err
		}
	}

	tag, err := Latest(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Downloading %s %s\n", Title, tag)
	rel, err := download(ctx, tag)
	if err != nil {
		return err
	}
	if err := putBinary(rel); err != nil {
		return err
	}
	if seeding {
		if rel.example == nil {
			return errors.New("the release has no config.example.yaml to start from")
		}
		c, mgmt, err := seed(rel.example, cfgPath, port)
		if err != nil {
			return err
		}
		if err := c.save(); err != nil {
			return err
		}
		if err := saveManagementKey(mgmt); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Wrote %s with a new API key and management key\n", cfgPath)
	} else {
		_, _ = fmt.Fprintf(out, "Keeping %s\n", cfgPath)
		if opts.Port != 0 {
			if err := edit(func(c *serverConfig) error { c.setPort(port); return nil }); err != nil {
				return err
			}
		}
	}
	if err := startService(ctx, out); err != nil {
		return err
	}
	return attach(out, opts.Proxy)
}

// portFree checks nothing else listens on port, such as another
// CLIProxyAPI.
func portFree(ctx context.Context, port int) error {
	l, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return fmt.Errorf("port %d is taken (another %s?); pick another with --port", port, Title)
	}
	return l.Close()
}

// putBinary stages the binary beside the old one and renames it over, so
// a running server keeps its file until it restarts.
func putBinary(rel release) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	bin, _ := Bin()
	staged, err := os.CreateTemp(dir, ".cli-proxy-api-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(staged.Name()) }()
	if _, err := staged.Write(rel.bin); err != nil {
		_ = staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := os.Chmod(staged.Name(), 0o755); err != nil { //nolint:gosec // an executable
		return err
	}
	if err := os.Rename(staged.Name(), bin); err != nil {
		return err
	}
	vp, _ := versionPath()
	return config.WriteFileAtomic(vp, []byte(rel.version+"\n"), 0o600)
}

// attach points a proxy entry at the server with its first key, unless that
// name already means another proxy.
func attach(out io.Writer, name string) error {
	st, err := Inspect(context.Background(), false)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if p, ok := cfg.Proxies[name]; ok && p.URL != st.URL() {
		_, _ = fmt.Fprintf(out, "Not attached: a proxy named %s already points at %s; add this one with `agentctl proxy add <name> --url %s`\n", name, p.URL, st.URL())
		return nil
	}
	p := config.Proxy{URL: st.URL()}
	if len(st.APIKeys) > 0 {
		p.Key = st.APIKeys[0]
	}
	if old, ok := cfg.Proxies[name]; ok {
		p.AllowModels = old.AllowModels
	}
	cfg.Proxies[name] = p
	cfg.CLIProxy.Proxy = name
	if err := cfg.Save(); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Attached as proxy %s; route agents with `agentctl use <agent> %s`\n", name, name)
	return nil
}

func startService(ctx context.Context, out io.Writer) error {
	if err := writeService(); err != nil {
		return err
	}
	if err := manager().start(ctx); err != nil {
		return err
	}
	return waitAnswering(ctx, out)
}

func waitAnswering(ctx context.Context, out io.Writer) error {
	st, err := Inspect(ctx, false)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Waiting for %s at %s\n", Title, st.URL())
	return waitHealthy(ctx, st.URL())
}

func needInstalled(ctx context.Context) (Status, error) {
	st, err := Inspect(ctx, false)
	if err == nil && !st.Installed {
		err = ErrNotInstalled
	}
	return st, err
}

// Start starts the server and keeps it starting at login.
func Start(ctx context.Context, out io.Writer) error {
	if _, err := needInstalled(ctx); err != nil {
		return err
	}
	return startService(ctx, out)
}

// Restart restarts the server, which drops requests in flight.
func Restart(ctx context.Context, out io.Writer) error {
	if _, err := needInstalled(ctx); err != nil {
		return err
	}
	if err := writeService(); err != nil {
		return err
	}
	if err := manager().restart(ctx); err != nil {
		return err
	}
	return waitAnswering(ctx, out)
}

// Stop stops the server; it stays stopped across logins until Start. It
// returns once the server no longer answers, or after 10s.
func Stop(ctx context.Context, _ io.Writer) error {
	st, err := needInstalled(ctx)
	if err != nil || !st.Service {
		return err
	}
	if err := manager().stop(ctx); err != nil {
		return err
	}
	for deadline := time.Now().Add(10 * time.Second); healthy(ctx, st.URL()) && time.Now().Before(deadline); {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return nil
}

// restartIfRunning applies a config change that needs a restart.
func restartIfRunning(ctx context.Context, out io.Writer) error {
	st, err := Inspect(ctx, false)
	if err != nil || !st.Service || !st.Enabled {
		return err
	}
	return Restart(ctx, out)
}

// Update installs the newest release over the old one and restarts the
// server on it. It reports the version now installed.
func Update(ctx context.Context, out io.Writer) (string, error) {
	st, err := needInstalled(ctx)
	if err != nil {
		return "", err
	}
	tag, err := Latest(ctx)
	if err != nil {
		return "", err
	}
	_, _ = fmt.Fprintf(out, "Downloading %s %s\n", Title, tag)
	rel, err := download(ctx, tag)
	if err != nil {
		return "", err
	}
	if err := putBinary(rel); err != nil {
		return "", err
	}
	if st.Service && st.Enabled {
		if err := Restart(ctx, out); err != nil {
			return "", err
		}
	}
	return rel.version, nil
}

// Uninstall stops and removes the service and the binary. With all it also
// deletes the config (client keys, settings), agentctl's copy of the
// management key and the proxy entry. Logins in the auth directory are always kept.
func Uninstall(ctx context.Context, all bool) error {
	if err := removeService(ctx); err != nil {
		return err
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if all {
		if p, err := ManagementKeyPath(); err == nil {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		return forgetProxy()
	}
	for _, name := range []string{"cli-proxy-api", "version", "cliproxyapi.log"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// forgetProxy drops the proxy entry that pointed at the server.
func forgetProxy() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.CLIProxy.Proxy == "" {
		return nil
	}
	delete(cfg.Proxies, cfg.CLIProxy.Proxy)
	cfg.CLIProxy = config.CLIProxy{}
	return cfg.Save()
}

// edit loads the config, applies f and saves it. CLIProxyAPI watches the
// file and reloads client keys on its own.
func edit(f func(*serverConfig) error) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	c, err := loadConfig(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotInstalled
	}
	if err != nil {
		return err
	}
	if err := f(c); err != nil {
		return err
	}
	return c.save()
}

// AddKey adds a new client API key and returns it.
func AddKey() (string, error) {
	key := newKey()
	err := edit(func(c *serverConfig) error {
		c.setAPIKeys(append(c.apiKeys(), key))
		return nil
	})
	return key, err
}

// RemoveKey removes a client API key. The last one stays: without keys
// the server would refuse every agent. When the attached proxy used it,
// the proxy moves to the first remaining key.
func RemoveKey(key string) error {
	var left []string
	err := edit(func(c *serverConfig) error {
		keys := c.apiKeys()
		if !slices.Contains(keys, key) {
			return errors.New("no such API key; see `agentctl cliproxy keys`")
		}
		if len(keys) == 1 {
			return errors.New("that is the only API key; add another first (`agentctl cliproxy keys add`)")
		}
		left = slices.DeleteFunc(keys, func(k string) bool { return k == key })
		c.setAPIKeys(left)
		return nil
	})
	if err != nil {
		return err
	}
	return syncProxy(key, left[0])
}

// syncProxy moves the attached proxy from an old key to a new one,
// including OpenCode's copy of it. Other agents ask `agentctl token`.
func syncProxy(old, key string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	name := cfg.CLIProxy.Proxy
	p, ok := cfg.Proxies[name]
	if !ok || p.Key != old {
		return nil
	}
	p.Key = key
	cfg.Proxies[name] = p
	if err := cfg.Save(); err != nil {
		return err
	}
	// opencode.KeyPath, which this package can't import: testutil, which
	// OpenCode's tests use, imports this package.
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	kp := filepath.Join(dir, "keys", name)
	if !fileExists(kp) {
		return nil
	}
	return config.WriteFileAtomic(kp, []byte(key), 0o600)
}

// RotateManagement sets a new management key and restarts a running server,
// which hashes it on start. It returns the key.
func RotateManagement(ctx context.Context, out io.Writer) (string, error) {
	key := newManagementKey()
	if err := edit(func(c *serverConfig) error { c.setManagementKey(key); return nil }); err != nil {
		return "", err
	}
	if err := saveManagementKey(key); err != nil {
		return "", err
	}
	return key, restartIfRunning(ctx, out)
}

// Listen sets where the server listens: every interface, or only this
// machine. The management API stays local-only either way. A running
// server restarts.
func Listen(ctx context.Context, out io.Writer, all bool) error {
	host := "127.0.0.1"
	if all {
		host = ""
	}
	if err := edit(func(c *serverConfig) error { c.setHost(host); return nil }); err != nil {
		return err
	}
	return restartIfRunning(ctx, out)
}

// Provider is an account CLIProxyAPI can log in to.
type Provider struct {
	Name, Title, Flag, Note string
	// Risk, when set, is shown before signing in, which then needs an
	// explicit yes.
	Risk string
}

// ClaudeRisk is why signing in to Claude through the proxy is the user's
// call, not a default (see
// .agents/notes/claude-lock/anthropic-subscription-policy.md).
const ClaudeRisk = "Anthropic's terms forbid routing a Claude subscription through a proxy, even for Claude Code. " +
	"Anthropic blocks such logins and can ban the account."

// Providers are the logins agentctl offers, Claude last and behind its
// warning.
var Providers = []Provider{
	{"codex", "Codex", "codex-login", "ChatGPT account, browser sign-in", ""},
	{"codex-device", "Codex (device code)", "codex-device-login", "ChatGPT account, code on another device", ""},
	{"antigravity", "Antigravity", "antigravity-login", "Google account", ""},
	{"xai", "xAI", "xai-login", "Grok account", ""},
	{"kimi", "Kimi", "kimi-login", "kimi.com account", ""},
	{"kimi-ai", "Kimi.ai", "kimi-ai-login", "kimi.ai account", ""},
	{"devin", "Devin", "devin-login", "Devin account", ""},
	{"meta", "Meta", "meta-login", "Meta account", ""},
	{"claude", "Claude", "claude-login", "Claude subscription · against Anthropic's terms", ClaudeRisk},
}

// FindProvider returns the provider called name.
func FindProvider(name string) (Provider, bool) {
	for _, p := range Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Provider{}, false
}

// LoginCmd is CLIProxyAPI's own login flow for p, to run attached to the
// terminal. It prints a link to open; over SSH the browser's callback can't
// reach this machine, so after a few seconds it also asks for the URL the
// browser ended on. The new account lands in the auth directory, which a
// running server watches.
func LoginCmd(ctx context.Context, p Provider) (*exec.Cmd, error) {
	if _, err := needInstalled(ctx); err != nil {
		return nil, err
	}
	bin, _ := Bin()
	cfg, _ := ConfigPath()
	dir, _ := Dir()
	cmd := exec.CommandContext(ctx, bin, "-config", cfg, "-"+p.Flag, "-no-browser") //nolint:gosec // agentctl's own install, fixed flags
	cmd.Dir = dir
	return cmd, nil
}
