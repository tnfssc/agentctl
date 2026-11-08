// Package config loads and saves agentctl's own settings: the proxies the
// user attached and what agentctl changed in each agent's files.
package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Proxy is one attached CLIProxyAPI server. Keys come either inline or from
// a command, so a key can stay on the machine that owns it (e.g. read over
// ssh) instead of being copied here.
type Proxy struct {
	URL              string `toml:"url"`
	Key              string `toml:"key,omitempty"`
	KeyCmd           string `toml:"key_cmd,omitempty"`
	ManagementKey    string `toml:"management_key,omitempty"`
	ManagementKeyCmd string `toml:"management_key_cmd,omitempty"`
	// AllowModels clears models agentctl cannot prove are non-Anthropic for
	// agents other than Claude Code. It never overrides a Claude match.
	AllowModels []string `toml:"allow_models,omitempty"`
}

// Config is the file at Path().
type Config struct {
	Proxies map[string]Proxy `toml:"proxies"`
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidName reports whether s can name a proxy. "local" is reserved because
// it means an agent's own login.
func ValidName(s string) error {
	if s == "local" {
		return errors.New(`"local" is reserved for the agents' own logins`)
	}
	if !nameRE.MatchString(s) {
		return fmt.Errorf("proxy name %q must be lowercase letters, digits, - or _", s)
	}
	return nil
}

// Dir is agentctl's config directory: $XDG_CONFIG_HOME/agentctl, falling
// back to ~/.config/agentctl on every OS so it is easy to find over ssh.
func Dir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "agentctl"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "agentctl"), nil
}

// Path is the config file path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads the config; a missing file is an empty config.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	cfg := &Config{Proxies: map[string]Proxy{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Proxies == nil {
		cfg.Proxies = map[string]Proxy{}
	}
	return cfg, nil
}

// Save writes the config with owner-only permissions; it holds keys.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("# Managed by agentctl. Keys live here, so keep this file private.\n\n")
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(c); err != nil {
		return err
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o600)
}

// Names returns proxy names sorted.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Proxies))
	for n := range c.Proxies {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Lookup returns the named proxy.
func (c *Config) Lookup(name string) (Proxy, error) {
	p, ok := c.Proxies[name]
	if !ok {
		return Proxy{}, fmt.Errorf("no proxy named %q; see `agentctl proxy ls`", name)
	}
	return p, nil
}

// ResolveKey returns the client API key, running KeyCmd if set.
func (p Proxy) ResolveKey(ctx context.Context) (string, error) {
	return resolve(ctx, p.Key, p.KeyCmd, "key")
}

// ResolveManagementKey returns the management key, or "" if none is set.
func (p Proxy) ResolveManagementKey(ctx context.Context) (string, error) {
	return resolve(ctx, p.ManagementKey, p.ManagementKeyCmd, "management key")
}

func resolve(ctx context.Context, inline, cmd, what string) (string, error) {
	if cmd == "" {
		return inline, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	c := exec.CommandContext(ctx, "/bin/sh", "-c", cmd) //nolint:gosec // the user's own configured command
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("%s command failed: %w: %s", what, err, strings.TrimSpace(stderr.String()))
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return "", fmt.Errorf("%s command printed nothing", what)
	}
	return key, nil
}

// WriteFileAtomic writes via a temp file and rename so a crash never leaves
// a half-written config behind. It keeps the existing file's mode when the
// file exists. Symlinks are followed so dotfile-managed configs stay links.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
