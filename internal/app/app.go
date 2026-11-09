// Package app is what agentctl does, independent of how it is driven: the
// command line and the dashboard both call it.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/agent/opencode"
	"github.com/tnfssc/agentctl/internal/agent/pi"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/ocweb"
	"github.com/tnfssc/agentctl/internal/proxy"
)

// Agent describes one supported coding agent.
type Agent struct {
	Name, Title string
	status      func(*config.Config) (Status, error)
}

// Status is an agent's current route, as its own config describes it.
type Status struct {
	Route, URL, Model string
	Notes             []string
}

// Agents lists the supported agents in display order.
var Agents = []Agent{
	{claude.Name, "Claude Code", func(cfg *config.Config) (Status, error) {
		s, err := claude.Current(cfg)
		return Status(s), err
	}},
	{codex.Name, "Codex", func(cfg *config.Config) (Status, error) {
		s, err := codex.Current(cfg)
		return Status(s), err
	}},
	{opencode.Name, "OpenCode", func(cfg *config.Config) (Status, error) {
		s, err := opencode.Current(cfg)
		return Status(s), err
	}},
	{pi.Name, "Pi", func(cfg *config.Config) (Status, error) {
		s, err := pi.Current(cfg)
		return Status(s), err
	}},
}

// AgentNames returns the command-line names of all agents.
func AgentNames() []string {
	names := make([]string, len(Agents))
	for i, a := range Agents {
		names[i] = a.Name
	}
	return names
}

// FindAgent returns the agent with the given command-line name.
func FindAgent(name string) (Agent, error) {
	for _, a := range Agents {
		if a.Name == name {
			return a, nil
		}
	}
	return Agent{}, fmt.Errorf("unknown agent %q; choose one of %v", name, AgentNames())
}

// Status reads the agent's current route.
func (a Agent) Status(cfg *config.Config) (Status, error) { return a.status(cfg) }

// Request is one route change.
type Request struct {
	Agent, Route, Model string
}

// sendsClaudeToProxy reports whether a route puts Claude Code in front of a
// proxy: Claude Code itself, or OpenCode, whose claude-agent runs it.
func sendsClaudeToProxy(agentName, route string) bool {
	return (agentName == claude.Name || agentName == opencode.Name) && route != agent.Local && route != ""
}

// NeedsClaudeAck reports whether the request routes Claude Code through a
// proxy before the user has accepted claude.PolicyWarning.
func NeedsClaudeAck(st *config.State, r Request) bool {
	return sendsClaudeToProxy(r.Agent, r.Route) && !st.ClaudeProxyAcknowledged
}

// UsableModels is what agents other than Claude Code may list from a proxy:
// models proven not to be Anthropic's that a coding agent can talk to.
func UsableModels(list []proxy.Model, allow []string) (usable []proxy.Model, hidden []string) {
	for _, pm := range list {
		if models.Usable(models.Classify(pm.ID, pm.OwnedBy, allow)) && models.Chat(pm.ID) {
			usable = append(usable, pm)
		} else {
			hidden = append(hidden, pm.ID)
		}
	}
	return usable, hidden
}

// Plan prepares a route change without writing anything. It may record
// facts in st (the model Codex used before a proxy) that Apply persists.
func Plan(ctx context.Context, cfg *config.Config, st *config.State, r Request) (agent.Plan, error) {
	var p config.Proxy
	if r.Route != agent.Local {
		var err error
		if p, err = cfg.Lookup(r.Route); err != nil {
			return agent.Plan{}, err
		}
	}
	switch r.Agent {
	case claude.Name:
		if r.Model != "" {
			return agent.Plan{}, errors.New("choose Claude Code's model with /model, not --model; agentctl only switches its route")
		}
		if r.Route != agent.Local {
			if _, err := FetchModels(ctx, p); err != nil {
				return agent.Plan{}, fmt.Errorf("%s: %w", r.Route, err)
			}
		}
		return claude.PlanUse(cfg, r.Route)
	case codex.Name:
		var cat codex.Catalog
		if r.Route != agent.Local {
			if r.Model != "" && models.Classify(r.Model, "", nil) == models.Anthropic {
				return agent.Plan{}, fmt.Errorf("%q is a Claude model; Claude models are only allowed in Claude Code", r.Model)
			}
			list, err := FetchModels(ctx, p)
			if err != nil {
				return agent.Plan{}, fmt.Errorf("%s: %w", r.Route, err)
			}
			if cat, err = codex.BuildCatalog(ctx, list, p.AllowModels); err != nil {
				return agent.Plan{}, err
			}
			if err := codex.RememberLocalModel(st); err != nil {
				return agent.Plan{}, err
			}
		}
		return codex.PlanUse(cfg, st, r.Route, r.Model, cat)
	case pi.Name, opencode.Name:
		var usable []proxy.Model
		if r.Route != agent.Local {
			if r.Model != "" && models.Classify(r.Model, "", nil) == models.Anthropic {
				return agent.Plan{}, fmt.Errorf("%q is a Claude model; Claude models are only allowed in Claude Code", r.Model)
			}
			list, err := FetchModels(ctx, p)
			if err != nil {
				return agent.Plan{}, fmt.Errorf("%s: %w", r.Route, err)
			}
			usable, _ = UsableModels(list, p.AllowModels)
		}
		if r.Agent == pi.Name {
			if r.Route != agent.Local {
				if err := pi.RememberLocal(st); err != nil {
					return agent.Plan{}, err
				}
			}
			return pi.PlanUse(cfg, st, r.Route, r.Model, pi.BuildModels(usable))
		}
		if r.Route != agent.Local {
			if err := opencode.RememberLocal(st); err != nil {
				return agent.Plan{}, err
			}
		}
		return opencode.PlanUse(ctx, cfg, st, r.Route, r.Model, usable)
	default:
		_, err := FindAgent(r.Agent)
		return agent.Plan{}, err
	}
}

// Apply writes a plan and saves the state that came with it. A Claude Code
// proxy route counts as the user accepting the policy warning.
func Apply(plan agent.Plan, st *config.State) error {
	if err := plan.Apply(); err != nil {
		return err
	}
	if sendsClaudeToProxy(plan.Agent, plan.Route) {
		st.ClaudeProxyAcknowledged = true
	}
	return st.Save()
}

// ConnectOpenCodeClaude turns on OpenCode's claude-agent provider for an
// OpenCode route, applied or already in place: it is how OpenCode reaches
// Claude models, through Claude Code, and it lists none until connected. It
// reports whether it connected it just now.
func ConnectOpenCodeClaude(ctx context.Context, plan agent.Plan) (bool, error) {
	if plan.Agent != opencode.Name || plan.Route == "" {
		return false, nil
	}
	return ocweb.ConnectClaude(ctx)
}

// ClaudeConnected is what to tell the user after ConnectOpenCodeClaude
// connected claude-agent.
const ClaudeConnected = "Turned on OpenCode's claude-agent: Claude models come through Claude Code and show up in " +
	"OpenCode in a minute."

// ErrNothingToRefresh means no agent with a model list is routed through a
// proxy.
var ErrNothingToRefresh = errors.New("no agent is routed through a proxy; nothing to refresh")

// ModelLists names the plan Refresh returns.
const ModelLists = "model lists"

// Refresh rebuilds the model list of every agent routed through a proxy
// (Codex, OpenCode and Pi; Claude Code asks the proxy itself), keeping each
// agent's model when the proxy still serves it. OpenCode's key copy is
// rewritten too. The result is one plan, reviewed at once.
func Refresh(ctx context.Context, cfg *config.Config, st *config.State) (agent.Plan, error) {
	merged := agent.Plan{Agent: ModelLists}
	for _, name := range []string{codex.Name, opencode.Name, pi.Name} {
		a, err := FindAgent(name)
		if err != nil {
			return agent.Plan{}, err
		}
		s, err := a.Status(cfg)
		if err != nil {
			return agent.Plan{}, fmt.Errorf("%s: %w", a.Title, err)
		}
		if s.Route == agent.Local || s.Route == "custom" {
			continue
		}
		p, err := Plan(ctx, cfg, st, Request{Agent: name, Route: s.Route})
		if err != nil {
			return agent.Plan{}, fmt.Errorf("%s: %w", a.Title, err)
		}
		for _, c := range p.Changes {
			c.Agent = name
			merged.Changes = append(merged.Changes, c)
		}
		for _, w := range p.Warnings {
			merged.Warnings = append(merged.Warnings, a.Title+": "+w)
		}
	}
	if merged.Changes == nil {
		return agent.Plan{}, ErrNothingToRefresh
	}
	return merged, nil
}

// FetchModels lists the models a proxy's key can reach.
func FetchModels(ctx context.Context, p config.Proxy) ([]proxy.Model, error) {
	c, err := proxy.New(p.URL)
	if err != nil {
		return nil, err
	}
	key, err := p.ResolveKey(ctx)
	if err != nil {
		return nil, err
	}
	list, err := c.Models(ctx, key)
	if proxy.IsUnauthorized(err) {
		return nil, errors.New("the proxy rejected the API key")
	}
	return list, err
}

// Probe is what a check of one proxy found.
type Probe struct {
	HealthErr error
	Latency   time.Duration
	Models    []proxy.Model
	ModelsErr error
	Counts    Counts

	// What Claude Code's model discovery would see.
	DiscoveryErr     error
	DiscoveryLatency time.Duration
	Disguised        []string // ids Claude Code would see that the plain list does not have
}

// Counts sorts a proxy's models by who may use them.
type Counts struct {
	Claude  int // Claude Code only
	Other   int // every agent
	Unknown int // Claude Code only until allowed
	NotChat int // image, audio and other models no agent drives
}

// String summarizes the counts, e.g. "3 Claude · 4 all agents · 0 unverified".
func (c Counts) String() string {
	s := fmt.Sprintf("%d Claude · %d all agents · %d unverified", c.Claude, c.Other, c.Unknown)
	if c.NotChat > 0 {
		s += fmt.Sprintf(" · %d not chat", c.NotChat)
	}
	return s
}

// CountModels sorts models by who may use them, given a proxy's allow list.
func CountModels(list []proxy.Model, allow []string) Counts {
	var c Counts
	for _, m := range list {
		class := models.Classify(m.ID, m.OwnedBy, allow)
		switch {
		case class == models.Anthropic:
			c.Claude++
		case !models.Chat(m.ID):
			c.NotChat++
		case class == models.Other:
			c.Other++
		default:
			c.Unknown++
		}
	}
	return c
}

// Err reports whether the proxy is unusable.
func (p Probe) Err() error { return errors.Join(p.HealthErr, p.ModelsErr) }

// discoveryTimeout is Claude Code's default for /v1/models discovery.
const discoveryTimeout = 3 * time.Second

// SlowDiscovery reports whether Claude Code's discovery would time out.
func (p Probe) SlowDiscovery() bool { return p.DiscoveryLatency > discoveryTimeout }

// ProbeProxy checks health, key and models, and asks for the model list the
// way Claude Code does to spot names the proxy would disguise.
func ProbeProxy(ctx context.Context, p config.Proxy) Probe {
	var r Probe
	c, err := proxy.New(p.URL)
	if err != nil {
		r.HealthErr = err
		return r
	}
	start := time.Now()
	if r.HealthErr = c.Health(ctx); r.HealthErr != nil {
		return r
	}
	r.Latency = time.Since(start)
	key, err := p.ResolveKey(ctx)
	if err != nil {
		r.ModelsErr = err
		return r
	}
	if r.Models, r.ModelsErr = c.Models(ctx, key); r.ModelsErr != nil {
		if proxy.IsUnauthorized(r.ModelsErr) {
			r.ModelsErr = errors.New("the proxy rejected the API key")
		}
		return r
	}
	r.Counts = CountModels(r.Models, p.AllowModels)
	plain := map[string]bool{}
	for _, m := range r.Models {
		plain[m.ID] = true
	}
	start = time.Now()
	seen, err := c.ModelsAsClaudeCode(ctx, key)
	r.DiscoveryLatency, r.DiscoveryErr = time.Since(start), err
	for _, id := range seen {
		if !plain[id] {
			r.Disguised = append(r.Disguised, id)
		}
	}
	return r
}
