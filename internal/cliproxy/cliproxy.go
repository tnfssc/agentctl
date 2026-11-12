// Package cliproxy runs a CLIProxyAPI server on this machine. Upstream ships
// release archives but no installer or service manager of its own (its Linux
// installer script lags the v8 config layout), so agentctl owns the whole
// install: the binary from a checksum-verified GitHub release, a config
// seeded from the release's example, a launchd agent or systemd user unit,
// and the keys. Logins (OAuth accounts) live in CLIProxyAPI's own auth
// directory and are made by its own `--*-login` flows.
//
// CLIProxyAPI bcrypt-hashes the management key in its config on startup, so
// agentctl keeps the plaintext in its own keys directory; that copy is the
// only way to show it later.
package cliproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/semver"
)

// Title names the server in messages.
const Title = "CLIProxyAPI"

// DefaultPort is CLIProxyAPI's own default.
const DefaultPort = 8317

// DefaultProxy is the name the server is attached under.
const DefaultProxy = "cliproxy"

// Dir holds the binary, its config, logs and the installed version:
// $XDG_DATA_HOME/agentctl/cliproxyapi, else ~/.local/share/agentctl/cliproxyapi.
func Dir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "agentctl", "cliproxyapi"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "agentctl", "cliproxyapi"), nil
}

func inDir(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// Bin is the installed cli-proxy-api.
func Bin() (string, error) { return inDir("cli-proxy-api") }

// ConfigPath is the server's config.yaml.
func ConfigPath() (string, error) { return inDir("config.yaml") }

// LogPath is where the service writes the server's output.
func LogPath() (string, error) { return inDir("cliproxyapi.log") }

func versionPath() (string, error) { return inDir("version") }

// ManagementKeyPath is agentctl's plaintext copy of the management key. The
// dot keeps it apart from OpenCode's per-proxy key copies, whose names
// can't contain one.
func ManagementKeyPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "keys", "cliproxyapi.management"), nil
}

// Login is one account CLIProxyAPI holds in its auth directory.
type Login struct {
	Provider string // the auth file's type: codex, antigravity, xai, ...
	Account  string // its email or label, "" if none
	File     string
}

// Status is what agentctl shows about the server on this machine.
type Status struct {
	Installed bool   // agentctl's binary exists
	Version   string // the release agentctl installed, without a v

	Latest    string // newest release, "" when unknown
	LatestErr error

	Service bool // the launchd agent or systemd unit exists
	Enabled bool // the service manager runs it (loaded / active)
	Running bool // it answers /healthz
	// NoLinger is set on Linux when systemd stops user services at logout.
	NoLinger bool

	Host string // server.host; "" listens on every interface
	Port int
	TLS  bool

	APIKeys []string
	// ManagementKey is agentctl's copy, "" when it has none.
	ManagementKey string
	// ManagementSet reports whether the config has a management key at
	// all; without one the management API answers 404.
	ManagementSet bool

	AuthDir string
	Logins  []Login
}

// URL is where this machine reaches the server.
func (s Status) URL() string {
	scheme := "http"
	if s.TLS {
		scheme = "https"
	}
	return scheme + "://127.0.0.1:" + strconv.Itoa(s.Port)
}

// PanelURL is the management web panel.
func (s Status) PanelURL() string { return s.URL() + "/management.html" }

// AllInterfaces reports whether the server listens beyond this machine.
func (s Status) AllInterfaces() bool { return s.Host == "" || s.Host == "0.0.0.0" || s.Host == "::" }

// UpdateAvailable reports whether a newer release exists than the one
// installed.
func (s Status) UpdateAvailable() bool {
	return s.Installed && s.Latest != "" && s.Version != "" && semver.Compare(s.Latest, s.Version) > 0
}

// OnLatest reports whether the installed release is known to be the newest.
func (s Status) OnLatest() bool {
	return s.Installed && s.Latest != "" && s.Version != "" && !s.UpdateAvailable()
}

// Inspect reads the server's state; latest also asks GitHub for the newest
// release. It changes nothing.
func Inspect(ctx context.Context, latest bool) (Status, error) {
	var s Status
	bin, err := Bin()
	if err != nil {
		return s, err
	}
	if _, err := os.Stat(bin); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil // not installed is a state
		}
		return s, err
	}
	s.Installed = true
	if p, err := versionPath(); err == nil {
		if b, err := os.ReadFile(p); err == nil { //nolint:gosec // agentctl's own file
			s.Version = strings.TrimPrefix(strings.TrimSpace(string(b)), "v")
		}
	}
	if err := s.readConfig(); err != nil {
		return s, err
	}
	if p, err := ManagementKeyPath(); err == nil {
		if b, err := os.ReadFile(p); err == nil { //nolint:gosec // agentctl's own key copy
			s.ManagementKey = strings.TrimSpace(string(b))
		}
	}
	svc := manager()
	s.Service = svc.installed()
	if s.Service {
		s.Enabled = svc.enabled(ctx)
		s.NoLinger = svc.noLinger(ctx)
	}
	s.Running = healthy(ctx, s.URL())
	if latest {
		if tag, err := Latest(ctx); err != nil {
			s.LatestErr = err
		} else {
			s.Latest = strings.TrimPrefix(tag, "v")
		}
	}
	return s, nil
}

func (s *Status) readConfig() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	c, err := loadConfig(path)
	if err != nil {
		return err
	}
	s.Host, s.Port, s.TLS = c.host(), c.port(), c.tls()
	s.APIKeys = c.apiKeys()
	s.ManagementSet = c.managementKey() != ""
	s.AuthDir = c.authDir()
	s.Logins = logins(s.AuthDir)
	return nil
}

// logins lists the auth files in dir, reading only their type and account.
func logins(dir string) []Login {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Login
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path) //nolint:gosec // CLIProxyAPI's auth directory
		if err != nil {
			continue
		}
		var f struct {
			Type, Email, Label string
		}
		if json.Unmarshal(b, &f) != nil || f.Type == "" {
			continue
		}
		acct := f.Email
		if acct == "" {
			acct = f.Label
		}
		out = append(out, Login{Provider: f.Type, Account: acct, File: path})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Account < out[j].Account
	})
	return out
}

// healthy reports whether the server answers /healthz; tests replace it.
var healthy = Healthz

// SetHealthy replaces the health check (for tests) and returns the old one.
func SetHealthy(f func(context.Context, string) bool) func(context.Context, string) bool {
	prev := healthy
	healthy = f
	return prev
}

// Healthz reports whether a server answers /healthz at base.
func Healthz(ctx context.Context, base string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// waitHealthy waits for the server to answer after a start, which the
// service managers return before.
func waitHealthy(ctx context.Context, base string) error {
	deadline := time.Now().Add(20 * time.Second)
	for {
		if healthy(ctx, base) {
			return nil
		}
		if time.Now().After(deadline) {
			log, _ := LogPath()
			return fmt.Errorf("%s did not answer at %s within 20s; see %s", Title, base, log)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// newKey is a random client API key in the OpenAI style CLIProxyAPI's docs
// use.
func newKey() string { return "sk-" + randomHex(24) }

// newManagementKey is a random management key.
func newManagementKey() string { return randomHex(24) }

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // never fails on supported platforms
	return hex.EncodeToString(b)
}

func saveManagementKey(key string) error {
	p, err := ManagementKeyPath()
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(p, []byte(key+"\n"), 0o600)
}

// Mask shortens a key for lists: its prefix and last four characters.
func Mask(key string) string {
	if len(key) <= 12 {
		return strings.Repeat("•", len(key))
	}
	head := 3
	if !strings.HasPrefix(key, "sk-") {
		head = 0
	}
	return key[:head] + "…" + key[len(key)-4:]
}
