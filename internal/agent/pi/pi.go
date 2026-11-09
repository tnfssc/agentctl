// Package pi switches Pi (earendil-works/pi) between its own logins and a
// proxy. Proxy mode adds an "agentctl" provider to ~/.pi/agent/models.json,
// listing only the proxy's non-Anthropic chat models, and makes it Pi's
// default in settings.json. Pi fetches the key per request by running
// `agentctl token <proxy>`, so it never sits in Pi's files.
package pi

import (
	"encoding/json"
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
const Name = "pi"

// providerID is the models.json provider agentctl owns.
const providerID = "agentctl"

// Dir is Pi's config directory, honoring PI_CODING_AGENT_DIR.
func Dir() (string, error) {
	if d := os.Getenv("PI_CODING_AGENT_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi", "agent"), nil
}

// ModelsPath is Pi's custom provider file.
func ModelsPath() (string, error) { return inDir("models.json") }

// SettingsPath is Pi's settings file.
func SettingsPath() (string, error) { return inDir("settings.json") }

func inDir(name string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name), nil
}

type settings struct {
	DefaultProvider string `json:"defaultProvider"`
	DefaultModel    string `json:"defaultModel"`
}

func readFiles() (modelsPath, settingsPath string, models, set []byte, s settings, err error) {
	if modelsPath, err = ModelsPath(); err != nil {
		return
	}
	if settingsPath, err = SettingsPath(); err != nil {
		return
	}
	if models, err = agent.ReadFile(modelsPath); err != nil {
		return
	}
	if set, err = agent.ReadFile(settingsPath); err != nil {
		return
	}
	if err = jsonc.Unmarshal(set, &s); err != nil {
		err = fmt.Errorf("%s: %w", settingsPath, err)
	}
	return
}

// Status is Pi's current route as its files describe it.
type Status struct {
	Route string // "local", a proxy name, or "custom"
	URL   string
	Model string
	Notes []string
}

// Current reads Pi's files and names the route.
func Current(cfg *config.Config) (Status, error) {
	_, _, models, _, s, err := readFiles()
	if err != nil {
		return Status{}, err
	}
	st := Status{Route: agent.Local, Model: s.DefaultModel}
	if s.DefaultProvider != providerID {
		if s.DefaultProvider != "" && s.DefaultModel != "" {
			st.Model = s.DefaultProvider + "/" + s.DefaultModel
		}
		return st, nil
	}
	st.Route, st.URL = "custom", strings.TrimSuffix(jsonc.GetString(models, "providers", providerID, "baseUrl"), "/v1")
	if got, err := proxy.NormalizeURL(st.URL); err == nil {
		for _, name := range cfg.Names() {
			if want, err := proxy.NormalizeURL(cfg.Proxies[name].URL); err == nil && want == got {
				st.Route = name
			}
		}
	}
	return st, nil
}

// Model is one entry in the agentctl provider. Fields Pi has defaults for
// are left out unless Pi's own catalog knows better.
type Model struct {
	ID               string          `json:"id"`
	Name             string          `json:"name,omitempty"`
	API              string          `json:"api,omitempty"`
	Reasoning        bool            `json:"reasoning,omitempty"`
	Input            []string        `json:"input,omitempty"`
	ContextWindow    int             `json:"contextWindow,omitempty"`
	MaxTokens        int             `json:"maxTokens,omitempty"`
	Cost             json.RawMessage `json:"cost,omitempty"`
	ThinkingLevelMap json.RawMessage `json:"thinkingLevelMap,omitempty"`
}

type provider struct {
	Name    string  `json:"name"`
	BaseURL string  `json:"baseUrl"`
	API     string  `json:"api"`
	APIKey  string  `json:"apiKey"`
	Models  []Model `json:"models"`
}

// BuildModels describes the given proxy models (already filtered to what Pi
// may list) for models.json. Metadata comes from Pi's own model catalog
// cache when it knows the id; OpenAI's models use the Responses API, the
// rest chat completions.
func BuildModels(list []proxy.Model) []Model {
	known := catalog()
	out := make([]Model, 0, len(list))
	for _, pm := range list {
		m, ok := known[pm.ID]
		if !ok {
			m = Model{Name: pm.ID}
		}
		m.ID, m.API = pm.ID, ""
		if strings.EqualFold(pm.OwnedBy, "openai") {
			m.API = "openai-responses"
		}
		out = append(out, m)
	}
	return out
}

// catalog reads Pi's cached model catalog (models-store.json), keyed by id.
// It is a cache Pi refreshes on its own; a missing or unfamiliar file only
// means plainer entries.
func catalog() map[string]Model {
	path, err := inDir("models-store.json")
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var store map[string]struct {
		Models []Model `json:"models"`
	}
	if json.Unmarshal(b, &store) != nil {
		return nil
	}
	out := map[string]Model{}
	// Vendors' own entries win over aggregators' copies.
	for _, p := range []string{"openrouter", "opencode", "google", "openai", "openai-codex"} {
		for _, m := range store[p].Models {
			id := m.ID
			if p == "openrouter" {
				_, id, _ = strings.Cut(id, "/")
			}
			out[id] = m
		}
	}
	return out
}

// RememberLocal stores Pi's default before the first switch to a proxy, so
// `use pi local` can restore it.
func RememberLocal(st *config.State) error {
	_, _, _, _, s, err := readFiles()
	if err != nil {
		return err
	}
	if s.DefaultProvider != providerID {
		st.PiLocalProvider, st.PiLocalModel = s.DefaultProvider, s.DefaultModel
	}
	return nil
}

// PlanUse prepares the change that routes Pi to route ("local" or a proxy
// name). For a proxy, models must come from BuildModels and model, if set,
// must be one of them.
func PlanUse(cfg *config.Config, st *config.State, route, model string, models []Model) (agent.Plan, error) {
	modelsPath, settingsPath, modelsBefore, setBefore, s, err := readFiles()
	if err != nil {
		return agent.Plan{}, err
	}
	plan := agent.Plan{Agent: Name, Route: route}
	var modelsAfter, setAfter []byte

	if route == agent.Local {
		setAfter = setBefore
		if modelsAfter, err = jsonc.Delete(modelsBefore, "providers", providerID); err != nil {
			return agent.Plan{}, fmt.Errorf("%s: %w", modelsPath, err)
		}
		if modelsBefore != nil && strings.TrimSpace(string(modelsAfter)) == "{}" {
			modelsAfter = nil // agentctl created the file
		}
		if s.DefaultProvider == providerID {
			if setAfter, err = setDefault(setAfter, st.PiLocalProvider, st.PiLocalModel); err != nil {
				return agent.Plan{}, fmt.Errorf("%s: %w", settingsPath, err)
			}
			if strings.TrimSpace(string(setAfter)) == "{}" {
				setAfter = nil // agentctl created the file
			}
		}
		plan.Changes = changes(modelsPath, modelsBefore, modelsAfter, settingsPath, setBefore, setAfter)
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
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	switch {
	case len(models) == 0:
		return agent.Plan{}, fmt.Errorf("%s serves no models Pi may use", route)
	case model != "" && !slices.Contains(ids, model):
		return agent.Plan{}, fmt.Errorf("%q is not available to Pi on %s; choose one of: %s", model, route, strings.Join(ids, ", "))
	case model == "" && slices.Contains(ids, s.DefaultModel):
		model = s.DefaultModel
	case model == "":
		model = ids[0]
	}
	block := provider{
		Name:    "agentctl: " + route,
		BaseURL: base + "/v1",
		API:     "openai-completions",
		// Pi runs this through /bin/sh on every request.
		APIKey: "!" + shellQuote(exe) + " token " + shellQuote(route),
		Models: models,
	}
	if modelsAfter, err = jsonc.Set(modelsBefore, block, "providers", providerID); err != nil {
		return agent.Plan{}, fmt.Errorf("%s: %w", modelsPath, err)
	}
	if setAfter, err = setDefault(setBefore, providerID, model); err != nil {
		return agent.Plan{}, fmt.Errorf("%s: %w", settingsPath, err)
	}
	plan.Changes = changes(modelsPath, modelsBefore, modelsAfter, settingsPath, setBefore, setAfter)
	return plan, nil
}

// setDefault sets Pi's default provider and model, or removes them when
// provider is empty.
func setDefault(src []byte, provider, model string) ([]byte, error) {
	var err error
	for _, kv := range [][2]string{{"defaultProvider", provider}, {"defaultModel", model}} {
		if kv[1] == "" {
			src, err = jsonc.Delete(src, kv[0])
		} else {
			src, err = jsonc.Set(src, kv[1], kv[0])
		}
		if err != nil {
			return nil, err
		}
	}
	return src, nil
}

func changes(modelsPath string, mb, ma []byte, settingsPath string, sb, sa []byte) []agent.Change {
	return []agent.Change{
		{Path: modelsPath, Before: mb, After: ma, Perm: 0o600},
		{Path: settingsPath, Before: sb, After: sa, Perm: 0o600},
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
