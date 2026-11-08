// Package claude switches Claude Code between its own login and a proxy by
// editing ~/.claude/settings.json. Nothing is copied: local mode removes the
// proxy settings and Claude Code falls back to the login it already has.
package claude

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/proxy"
)

// Name is the agent's name on the command line.
const Name = "claude"

// PolicyWarning is shown, and must be acknowledged once, before Claude Code
// is first routed through a proxy.
const PolicyWarning = `Anthropic's terms forbid serving Claude Pro/Max/Team logins through a proxy:
"developers may not collect, store, or intermediate Claude.ai credentials or session tokens"
(code.claude.com/docs/en/legal-and-compliance). If this proxy holds Claude subscription logins,
the accounts on it can be banned without notice, even when only Claude Code uses them.
Routing Claude Code to non-Claude models on the proxy, or to Claude via an API key upstream, is fine.`

// Environment keys agentctl owns in settings.json. Local mode removes all of
// them so the stored login is used again.
const (
	envBaseURL   = "ANTHROPIC_BASE_URL"
	envAuthToken = "ANTHROPIC_AUTH_TOKEN"
	envAPIKey    = "ANTHROPIC_API_KEY"
	envModel     = "ANTHROPIC_MODEL"
	envDiscovery = "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"
)

var managedEnv = []string{envBaseURL, envAuthToken, envAPIKey, envModel, envDiscovery}

// SettingsPath is the user settings file, honoring CLAUDE_CONFIG_DIR.
func SettingsPath() (string, error) {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "settings.json"), nil
}

// Status is Claude Code's current route as its settings describe it.
type Status struct {
	Route string // "local", a proxy name, or "custom"
	URL   string
	Model string
	Notes []string
}

// Current reads the settings and names the route.
func Current(cfg *config.Config) (Status, error) {
	path, err := SettingsPath()
	if err != nil {
		return Status{}, err
	}
	src, err := agent.ReadFile(path)
	if err != nil {
		return Status{}, err
	}
	if len(src) > 0 && !gjson.ValidBytes(src) {
		return Status{}, fmt.Errorf("%s is not valid JSON", path)
	}
	s := Status{Route: agent.Local}
	env := gjson.GetBytes(src, "env")
	s.URL = env.Get(envBaseURL).String()
	s.Model = env.Get(envModel).String()
	if s.URL != "" {
		s.Route = matchProxy(cfg, s.URL)
	}
	if s.Route == agent.Local && (gjson.GetBytes(src, "apiKeyHelper").Exists() || env.Get(envAuthToken).Exists() || env.Get(envAPIKey).Exists()) {
		s.Notes = append(s.Notes, "settings.json supplies an API key, which overrides your Claude login")
	}
	s.Notes = append(s.Notes, shellNotes(s.Route)...)
	return s, nil
}

func matchProxy(cfg *config.Config, rawURL string) string {
	got, err := proxy.NormalizeURL(rawURL)
	if err != nil {
		return "custom"
	}
	for _, name := range cfg.Names() {
		if want, err := proxy.NormalizeURL(cfg.Proxies[name].URL); err == nil && want == got {
			return name
		}
	}
	return "custom"
}

// shellNotes flags variables exported in this shell that would fight the
// settings file. Claude Code may be started from a different environment, so
// these are hints, not errors.
func shellNotes(route string) []string {
	var notes []string
	for _, k := range []string{envBaseURL, envAuthToken, envAPIKey} {
		if os.Getenv(k) != "" {
			if route == agent.Local {
				notes = append(notes, fmt.Sprintf("$%s is exported in your shell and overrides your Claude login there", k))
			} else if k != envBaseURL {
				notes = append(notes, fmt.Sprintf("$%s is exported in your shell; unset it so the proxy key is used", k))
			}
		}
	}
	return notes
}

// PlanUse prepares the settings change that routes Claude Code to route
// ("local" or a proxy name). Model selection stays with Claude Code; any
// ANTHROPIC_MODEL override is removed so /model controls future sessions.
func PlanUse(cfg *config.Config, route string) (agent.Plan, error) {
	path, err := SettingsPath()
	if err != nil {
		return agent.Plan{}, err
	}
	before, err := agent.ReadFile(path)
	if err != nil {
		return agent.Plan{}, err
	}
	src := before
	if len(src) == 0 {
		src = []byte("{}")
	}
	if !gjson.ValidBytes(src) {
		return agent.Plan{}, fmt.Errorf("%s is not valid JSON; fix it first", path)
	}

	plan := agent.Plan{Agent: Name, Route: route}
	if helper := gjson.GetBytes(src, "apiKeyHelper").String(); helper != "" && !strings.Contains(helper, " token ") {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("replacing your apiKeyHelper %q (a backup is kept)", helper))
	}
	for _, k := range []string{envAuthToken, envAPIKey} {
		if gjson.GetBytes(src, "env."+k).Exists() {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("removing %s from settings.json env (a backup is kept)", k))
		}
	}

	for _, k := range managedEnv {
		if src, err = sjson.DeleteBytes(src, "env."+k); err != nil {
			return agent.Plan{}, err
		}
	}
	if src, err = sjson.DeleteBytes(src, "apiKeyHelper"); err != nil {
		return agent.Plan{}, err
	}

	if route != agent.Local {
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
		sets := []struct {
			path  string
			value string
		}{
			{"env." + envBaseURL, base},
			{"env." + envDiscovery, "1"},
			{"apiKeyHelper", shellQuote(exe) + " token " + route},
		}
		for _, s := range sets {
			if src, err = sjson.SetBytes(src, s.path, s.value); err != nil {
				return agent.Plan{}, err
			}
		}
	}

	if env := gjson.GetBytes(src, "env"); env.IsObject() && len(env.Map()) == 0 {
		if src, err = sjson.DeleteBytes(src, "env"); err != nil {
			return agent.Plan{}, err
		}
	}
	after := Format(src, before)
	if before == nil && string(after) == "{}\n" {
		after = nil
	}
	plan.Changes = []agent.Change{{Path: path, Before: before, After: after, Perm: 0o600}}
	plan.Warnings = append(plan.Warnings, shellNotes(route)...)
	return plan, nil
}

// Format pretty-prints like Claude Code does (two spaces, one item per line,
// key order kept) and keeps the original's trailing-newline choice.
func Format(src, original []byte) []byte {
	out := pretty.PrettyOptions(src, &pretty.Options{Width: 1, Indent: "  "})
	if len(original) > 0 && original[len(original)-1] != '\n' {
		out = []byte(strings.TrimRight(string(out), "\n"))
	}
	return out
}

// shellQuote quotes s for /bin/sh unless it is plainly safe as is.
func shellQuote(s string) string {
	safe := func(r rune) bool {
		return strings.ContainsRune("/.-_", r) || unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !safe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ErrNoLogin is returned when local mode is chosen but Claude Code has no
// stored login to fall back to.
var ErrNoLogin = errors.New("no Claude login found; run `claude` and /login first")
