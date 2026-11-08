// Package app is what agentctl does, independent of how it is driven: the
// command line and the dashboard both call it.
package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
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

// NeedsClaudeAck reports whether the request routes Claude Code through a
// proxy before the user has accepted claude.PolicyWarning.
func NeedsClaudeAck(st *config.State, r Request) bool {
	return r.Agent == claude.Name && r.Route != agent.Local && !st.ClaudeProxyAcknowledged
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
		if r.Route != agent.Local {
			list, err := FetchModels(ctx, p)
			if err != nil {
				return agent.Plan{}, fmt.Errorf("%s: %w", r.Route, err)
			}
			if r.Model != "" && !slices.ContainsFunc(list, func(m proxy.Model) bool { return m.ID == r.Model }) {
				return agent.Plan{}, fmt.Errorf("%s does not serve %q", r.Route, r.Model)
			}
		}
		return claude.PlanUse(cfg, r.Route, r.Model)
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
	if plan.Agent == claude.Name && plan.Route != agent.Local {
		st.ClaudeProxyAcknowledged = true
	}
	return st.Save()
}

// ErrNothingToRefresh means Codex is not routed through an agentctl proxy.
var ErrNothingToRefresh = errors.New("codex is not routed through a proxy; nothing to refresh")

// RefreshCodex rebuilds Codex's model list from its current proxy, keeping
// the current model if the proxy still serves it.
func RefreshCodex(ctx context.Context, cfg *config.Config, st *config.State) (agent.Plan, error) {
	s, err := codex.Current(cfg)
	if err != nil {
		return agent.Plan{}, err
	}
	if s.Route == agent.Local || s.Route == "custom" {
		return agent.Plan{}, ErrNothingToRefresh
	}
	plan, err := Plan(ctx, cfg, st, Request{Agent: codex.Name, Route: s.Route})
	return plan, err
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
	Counts    struct{ Claude, Other, Unknown int }

	// What Claude Code's model discovery would see.
	DiscoveryErr     error
	DiscoveryLatency time.Duration
	Disguised        []string // ids Claude Code would see that the plain list does not have
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
	plain := map[string]bool{}
	for _, m := range r.Models {
		plain[m.ID] = true
		switch models.Classify(m.ID, m.OwnedBy, p.AllowModels) {
		case models.Anthropic:
			r.Counts.Claude++
		case models.Other:
			r.Counts.Other++
		default:
			r.Counts.Unknown++
		}
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
