// Package codex switches Codex between its own ChatGPT login and a proxy by
// editing ~/.codex/config.toml. Proxy mode also writes a model catalog that
// lists only the proxy's non-Anthropic models, so Claude never shows up in
// Codex's model picker.
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/tomledit"
)

// Name is the agent's name on the command line.
const Name = "codex"

// providerID is the [model_providers.<id>] table agentctl owns.
const providerID = "agentctl"

// templateSlug is the bundled entry cloned for proxy models Codex does not
// know. It exposes tools directly rather than through code mode, which is
// the safer default for models built by other vendors.
const templateSlug = "gpt-5.5"

// Home is Codex's config directory, honoring CODEX_HOME.
func Home() (string, error) {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

// ConfigPath is Codex's user config file.
func ConfigPath() (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "config.toml"), nil
}

// CatalogPath is where the filtered model catalog for a proxy is kept.
func CatalogPath(proxyName string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codex", proxyName+".models.json"), nil
}

type codexConfig struct {
	Model            string `toml:"model"`
	ModelProvider    string `toml:"model_provider"`
	ModelCatalogJSON string `toml:"model_catalog_json"`
	ModelProviders   map[string]struct {
		BaseURL string `toml:"base_url"`
	} `toml:"model_providers"`
}

func readConfig() (path string, src []byte, cfg codexConfig, err error) {
	if path, err = ConfigPath(); err != nil {
		return path, nil, cfg, err
	}
	if src, err = agent.ReadFile(path); err != nil {
		return path, nil, cfg, err
	}
	if err = toml.Unmarshal(src, &cfg); err != nil {
		return path, src, cfg, fmt.Errorf("%s: %w", path, err)
	}
	return path, src, cfg, nil
}

// Status is Codex's current route as its config describes it.
type Status struct {
	Route string // "local", a proxy name, or "custom"
	URL   string
	Model string
	Notes []string
}

// Current reads config.toml and names the route.
func Current(cfg *config.Config) (Status, error) {
	_, _, cc, err := readConfig()
	if err != nil {
		return Status{}, err
	}
	s := Status{Route: agent.Local, Model: cc.Model}
	switch cc.ModelProvider {
	case "", "openai":
	case providerID:
		s.URL = cc.ModelProviders[providerID].BaseURL
		s.Route = "custom"
		if got, err := proxy.NormalizeURL(s.URL); err == nil {
			for _, name := range cfg.Names() {
				if want, err := proxy.NormalizeURL(cfg.Proxies[name].URL); err == nil && want == got {
					s.Route = name
				}
			}
		}
	default:
		s.Route = "custom"
		s.URL = cc.ModelProviders[cc.ModelProvider].BaseURL
		s.Notes = append(s.Notes, fmt.Sprintf("model_provider %q is not managed by agentctl", cc.ModelProvider))
	}
	return s, nil
}

// Catalog is a filtered Codex model catalog for one proxy.
type Catalog struct {
	JSON   []byte   // {"models":[...]} ready to write
	Slugs  []string // listed models, best first
	Hidden []string // proxy models left out (Anthropic or unproven)
}

// BuildCatalog keeps the proxy models agentctl can prove are not Anthropic's,
// reusing Codex's own bundled entry for each model it knows and cloning a
// template entry for the rest.
func BuildCatalog(ctx context.Context, proxyModels []proxy.Model, allow []string) (Catalog, error) {
	bundled, err := BundledCatalog(ctx)
	if err != nil {
		return Catalog{}, err
	}
	bySlug := map[string]map[string]any{}
	for _, m := range bundled {
		if slug, ok := m["slug"].(string); ok {
			bySlug[slug] = m
		}
	}
	template, ok := bySlug[templateSlug]
	if !ok && len(bundled) > 0 {
		template = bundled[len(bundled)-1]
	}

	var (
		cat     Catalog
		entries []map[string]any
	)
	for i, pm := range proxyModels {
		if !models.Usable(models.Classify(pm.ID, pm.OwnedBy, allow)) {
			cat.Hidden = append(cat.Hidden, pm.ID)
			continue
		}
		entry, known := bySlug[pm.ID]
		if !known {
			if template == nil {
				continue
			}
			entry = clone(template)
			entry["slug"] = pm.ID
			entry["display_name"] = pm.ID
			entry["description"] = "Served by your proxy (" + pm.OwnedBy + ")"
			entry["visibility"] = "list"
			entry["priority"] = 1000 + i
			delete(entry, "upgrade")
			delete(entry, "availability_nux")
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return cat, errors.New("the proxy serves no models agentctl can prove are non-Anthropic; allow some with `agentctl proxy allow`")
	}
	sort.SliceStable(entries, func(i, j int) bool { return priority(entries[i]) < priority(entries[j]) })
	for _, e := range entries {
		if slug, ok := e["slug"].(string); ok && e["visibility"] == "list" {
			cat.Slugs = append(cat.Slugs, slug)
		}
	}
	cat.JSON, err = json.MarshalIndent(map[string]any{"models": entries}, "", "  ")
	return cat, err
}

func priority(e map[string]any) float64 {
	switch p := e["priority"].(type) {
	case float64:
		return p
	case int:
		return float64(p)
	default:
		return 1 << 20
	}
}

func clone(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// BundledCatalog asks the installed Codex for the catalog it ships with, so
// entries always match the local Codex version. Tests replace it.
var BundledCatalog = bundledCatalog

func bundledCatalog(ctx context.Context) ([]map[string]any, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "codex", "debug", "models", "--bundled")
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("codex is not installed or not on PATH")
		}
		return nil, fmt.Errorf("codex debug models --bundled: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var doc struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("codex debug models --bundled: %w", err)
	}
	return doc.Models, nil
}

// PlanUse prepares the change that routes Codex to route ("local" or a
// proxy name). For a proxy, cat must come from BuildCatalog and model, if
// set, must be one of cat.Slugs.
func PlanUse(cfg *config.Config, st *config.State, route, model string, cat Catalog) (agent.Plan, error) {
	path, before, cc, err := readConfig()
	if err != nil {
		return agent.Plan{}, err
	}
	plan := agent.Plan{Agent: Name, Route: route}
	src := before
	if src, err = tomledit.RemoveTable(src, "model_providers", providerID); err != nil {
		return agent.Plan{}, err
	}
	if cc.ModelProvider != "" && cc.ModelProvider != providerID && cc.ModelProvider != "openai" {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("replacing your model_provider %q (a backup is kept)", cc.ModelProvider))
	}

	if route == agent.Local {
		for _, k := range []string{"model_provider", "model_catalog_json"} {
			if src, err = tomledit.DeleteTopLevel(src, k); err != nil {
				return agent.Plan{}, err
			}
		}
		// The proxy's model may not exist for the ChatGPT login: restore the
		// one used before, or fall back to Codex's default.
		if cc.ModelProvider == providerID {
			if st.CodexLocalModel != "" {
				src, err = tomledit.SetTopLevel(src, "model", tomledit.String(st.CodexLocalModel))
			} else {
				src, err = tomledit.DeleteTopLevel(src, "model")
			}
			if err != nil {
				return agent.Plan{}, err
			}
		}
		plan.Changes = []agent.Change{{Path: path, Before: before, After: src}}
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
	exe, err := agent.HelperCommand()
	if err != nil {
		return agent.Plan{}, err
	}
	catPath, err := CatalogPath(route)
	if err != nil {
		return agent.Plan{}, err
	}
	switch {
	case model != "" && !slices.Contains(cat.Slugs, model):
		return agent.Plan{}, fmt.Errorf("%q is not available to Codex on %s; choose one of: %s", model, route, strings.Join(cat.Slugs, ", "))
	case model == "" && slices.Contains(cat.Slugs, cc.Model):
		model = cc.Model
	case model == "" && len(cat.Slugs) > 0:
		model = cat.Slugs[0]
	}

	for _, kv := range [][2]string{
		{"model_provider", tomledit.String(providerID)},
		{"model_catalog_json", tomledit.String(catPath)},
		{"model", tomledit.String(model)},
	} {
		if src, err = tomledit.SetTopLevel(src, kv[0], kv[1]); err != nil {
			return agent.Plan{}, err
		}
	}
	block := fmt.Sprintf(`[model_providers.%[1]s]
# Managed by agentctl; `+"`agentctl use codex local`"+` removes it.
name = %[2]s
base_url = %[3]s
wire_api = "responses"

[model_providers.%[1]s.auth]
command = %[4]s
args = %[5]s
timeout_ms = 30000
`, providerID, tomledit.String("agentctl: "+route), tomledit.String(base+"/v1"),
		tomledit.String(exe), tomledit.Strings("token", route))
	if src, err = tomledit.AppendBlock(src, block); err != nil {
		return agent.Plan{}, err
	}

	catBefore, err := agent.ReadFile(catPath)
	if err != nil {
		return agent.Plan{}, err
	}
	plan.Changes = []agent.Change{
		{Path: path, Before: before, After: src},
		{Path: catPath, Before: catBefore, After: append(slices.Clip(cat.JSON), '\n'), Perm: 0o600, Quiet: true},
	}
	if len(cat.Hidden) > 0 {
		plan.Warnings = append(plan.Warnings, "hidden from Codex (Claude or unverified): "+strings.Join(cat.Hidden, ", "))
	}
	return plan, nil
}

// RememberLocalModel stores the model Codex uses on its own login before the
// first switch to a proxy, so `use codex local` can restore it.
func RememberLocalModel(st *config.State) error {
	_, _, cc, err := readConfig()
	if err != nil {
		return err
	}
	if cc.ModelProvider != providerID {
		st.CodexLocalModel = cc.Model
	}
	return nil
}
