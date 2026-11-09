// Package opencode switches OpenCode (v2) between its own logins and a
// proxy by editing its global config. Proxy mode adds an "agentctl" provider
// listing only the proxy's non-Anthropic chat models, and points OpenCode's
// claude-agent provider at the same proxy: Claude models then run through
// the real Claude Code binary, the only place agentctl lets them run.
//
// OpenCode can't run a command for a key, so the proxy's key goes into a
// 0600 file under agentctl's config directory that the config reads with
// {file:...}; the config itself never holds it.
package opencode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/jsonc"
	"github.com/tnfssc/agentctl/internal/proxy"
)

// Name is the agent's name on the command line.
const Name = "opencode"

const (
	providerID = "agentctl"
	claudeID   = "claude-agent"
)

// ConfigDir is OpenCode's global config directory: OPENCODE_CONFIG_DIR,
// else $XDG_CONFIG_HOME/opencode, else ~/.config/opencode on every OS.
func ConfigDir() (string, error) {
	if d := os.Getenv("OPENCODE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "opencode"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "opencode"), nil
}

// ConfigPath is the global config file: opencode.jsonc when it exists
// (OpenCode reads it last, so it wins), else opencode.json.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	if p := filepath.Join(dir, "opencode.jsonc"); fileExists(p) {
		return p, nil
	}
	return filepath.Join(dir, "opencode.json"), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// KeyPath is where a proxy's key is kept for OpenCode.
func KeyPath(proxyName string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "keys", proxyName), nil
}

func readConfig() (path string, src []byte, err error) {
	if path, err = ConfigPath(); err != nil {
		return path, nil, err
	}
	src, err = agent.ReadFile(path)
	return path, src, err
}

// Status is OpenCode's current route as its config describes it.
type Status struct {
	Route string // "local", a proxy name, or "custom"
	URL   string
	Model string
	Notes []string
}

// Current reads the config and names the route.
func Current(cfg *config.Config) (Status, error) {
	path, src, err := readConfig()
	if err != nil {
		return Status{}, err
	}
	model := jsonc.GetString(src, "model")
	if _, _, err := jsonc.Get(src, "model"); err != nil {
		return Status{}, fmt.Errorf("%s: %w", path, err)
	}
	s := Status{Route: agent.Local, Model: model}
	if !strings.HasPrefix(model, providerID+"/") {
		return s, nil
	}
	s.Model = strings.TrimPrefix(model, providerID+"/")
	s.Route = "custom"
	s.URL = strings.TrimSuffix(jsonc.GetString(src, "providers", providerID, "settings", "baseURL"), "/v1")
	if got, err := proxy.NormalizeURL(s.URL); err == nil {
		for _, name := range cfg.Names() {
			if want, err := proxy.NormalizeURL(cfg.Proxies[name].URL); err == nil && want == got {
				s.Route = name
			}
		}
	}
	return s, nil
}

type model struct {
	Name string `json:"name"`
	// Package is the AI SDK package for this model when it differs from the
	// provider's: @ai-sdk/openai speaks the Responses API.
	Package string `json:"package,omitempty"`
}

type providerBlock struct {
	Name     string           `json:"name"`
	Package  string           `json:"package"`
	Settings map[string]any   `json:"settings"`
	Models   map[string]model `json:"models"`
}

// RememberLocal stores OpenCode's model before the first switch to a proxy,
// so `use opencode local` can restore it.
func RememberLocal(st *config.State) error {
	_, src, err := readConfig()
	if err != nil {
		return err
	}
	if m := jsonc.GetString(src, "model"); !strings.HasPrefix(m, providerID+"/") {
		st.OpenCodeLocalModel = m
	}
	return nil
}

// PlanUse prepares the change that routes OpenCode to route ("local" or a
// proxy name). For a proxy, list is the proxy's models OpenCode may use
// and model, if set, one of them.
func PlanUse(ctx context.Context, cfg *config.Config, st *config.State, route, chosen string, list []proxy.Model) (agent.Plan, error) {
	path, before, err := readConfig()
	if err != nil {
		return agent.Plan{}, err
	}
	plan := agent.Plan{Agent: Name, Route: route}
	src := before

	if route == agent.Local {
		if src, err = removeOurs(before); err != nil {
			return agent.Plan{}, fmt.Errorf("%s: %w", path, err)
		}
		if strings.HasPrefix(jsonc.GetString(before, "model"), providerID+"/") {
			if st.OpenCodeLocalModel != "" {
				src, err = jsonc.Set(src, st.OpenCodeLocalModel, "model")
			} else {
				src, err = jsonc.Delete(src, "model")
			}
			if err != nil {
				return agent.Plan{}, fmt.Errorf("%s: %w", path, err)
			}
		}
		if before != nil && strings.TrimSpace(string(src)) == "{}" {
			src = nil // agentctl created the file
		}
		plan.Changes = []agent.Change{{Path: path, Before: before, After: src, Perm: 0o600}}
		return plan, nil
	}

	p, err := cfg.Lookup(route)
	if err != nil {
		return agent.Plan{}, err
	}
	base, err := proxy.NormalizeURL(p.URL)
	if err != nil {
		return agent.Plan{}, err
	}
	key, err := p.ResolveKey(ctx)
	if err != nil {
		return agent.Plan{}, err
	}
	if key == "" {
		key = "agentctl-no-key" // the provider wants one; the proxy ignores it
	}
	keyPath, err := KeyPath(route)
	if err != nil {
		return agent.Plan{}, err
	}
	ids := make([]string, len(list))
	block := providerBlock{
		Name:     "agentctl: " + route,
		Package:  "@ai-sdk/openai-compatible",
		Settings: map[string]any{"baseURL": base + "/v1", "apiKey": "{file:" + keyPath + "}"},
		Models:   map[string]model{},
	}
	for i, pm := range list {
		ids[i] = pm.ID
		m := model{Name: pm.ID}
		if strings.EqualFold(pm.OwnedBy, "openai") {
			m.Package = "@ai-sdk/openai"
		}
		block.Models[pm.ID] = m
	}
	current := strings.TrimPrefix(jsonc.GetString(before, "model"), providerID+"/")
	switch {
	case len(list) == 0:
		return agent.Plan{}, fmt.Errorf("%s serves no models OpenCode may use", route)
	case chosen != "" && !slices.Contains(ids, chosen):
		return agent.Plan{}, fmt.Errorf("%q is not available to OpenCode on %s; choose one of: %s", chosen, route, strings.Join(ids, ", "))
	case chosen == "" && slices.Contains(ids, current):
		chosen = current
	case chosen == "":
		chosen = ids[0]
	}
	for _, edit := range []struct {
		v    any
		path []string
	}{
		{block, []string{"providers", providerID}},
		{base, []string{"providers", claudeID, "settings", "baseURL"}},
		{"{file:" + keyPath + "}", []string{"providers", claudeID, "settings", "authToken"}},
		{providerID + "/" + chosen, []string{"model"}},
	} {
		if src, err = jsonc.Set(src, edit.v, edit.path...); err != nil {
			return agent.Plan{}, fmt.Errorf("%s: %w", path, err)
		}
	}
	keyBefore, err := agent.ReadFile(keyPath)
	if err != nil {
		return agent.Plan{}, err
	}
	plan.Changes = []agent.Change{
		{Path: path, Before: before, After: src, Perm: 0o600},
		{Path: keyPath, Before: keyBefore, After: []byte(key + "\n"), Perm: 0o600, Quiet: true},
	}
	if p.KeyCmd != "" {
		plan.Warnings = append(plan.Warnings, "OpenCode reads a copy of the key; run `agentctl refresh` after it changes")
	}
	return plan, nil
}

// removeOurs takes out everything agentctl adds: its provider, and the
// claude-agent settings that point at one of its key files.
func removeOurs(src []byte) ([]byte, error) {
	src, err := jsonc.Delete(src, "providers", providerID)
	if err != nil {
		return nil, err
	}
	dir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	token := jsonc.GetString(src, "providers", claudeID, "settings", "authToken")
	if !strings.HasPrefix(token, "{file:"+filepath.Join(dir, "keys")+string(filepath.Separator)) {
		return src, nil
	}
	for _, k := range []string{"authToken", "baseURL"} {
		if src, err = jsonc.Delete(src, "providers", claudeID, "settings", k); err != nil {
			return nil, err
		}
	}
	return src, nil
}
