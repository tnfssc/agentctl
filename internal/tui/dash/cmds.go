package dash

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/proxy"
)

// Results of background work. Every slow call (network, `codex debug
// models`, file writes) runs in a command and reports back with one of these.
type (
	loadedMsg struct {
		cfg      *config.Config
		st       *config.State
		statuses []agentStatus
		err      error
	}
	probedMsg struct {
		name  string
		probe app.Probe
	}
	modelsMsg struct {
		route string
		list  []proxy.Model
		err   error
	}
	plannedMsg struct {
		plan    agent.Plan
		err     error
		refresh bool
	}
	appliedMsg struct {
		plan agent.Plan
		err  error
	}
	savedMsg struct {
		name  string
		probe app.Probe
		err   error
	}
	removedMsg struct {
		name string
		err  error
	}
)

type agentStatus struct {
	agent  app.Agent
	status app.Status
	err    error
}

func load() tea.Msg {
	cfg, err := config.Load()
	if err != nil {
		return loadedMsg{err: err}
	}
	st, err := config.LoadState()
	if err != nil {
		return loadedMsg{err: err}
	}
	msg := loadedMsg{cfg: cfg, st: st}
	for _, a := range app.Agents {
		s, err := a.Status(cfg)
		msg.statuses = append(msg.statuses, agentStatus{agent: a, status: s, err: err})
	}
	return msg
}

func probe(ctx context.Context, name string, p config.Proxy) tea.Cmd {
	return func() tea.Msg { return probedMsg{name: name, probe: app.ProbeProxy(ctx, p)} }
}

func fetchModels(ctx context.Context, route string, p config.Proxy) tea.Cmd {
	return func() tea.Msg {
		list, err := app.FetchModels(ctx, p)
		return modelsMsg{route: route, list: list, err: err}
	}
}

func plan(ctx context.Context, cfg *config.Config, st *config.State, req app.Request) tea.Cmd {
	return func() tea.Msg {
		p, err := app.Plan(ctx, cfg, st, req)
		return plannedMsg{plan: p, err: err}
	}
}

func refresh(ctx context.Context, cfg *config.Config, st *config.State) tea.Cmd {
	return func() tea.Msg {
		p, err := app.RefreshCodex(ctx, cfg, st)
		return plannedMsg{plan: p, err: err, refresh: true}
	}
}

func apply(p agent.Plan, st *config.State) tea.Cmd {
	return func() tea.Msg { return appliedMsg{plan: p, err: app.Apply(p, st)} }
}

// saveProxy checks a new proxy and saves it only if it works.
func saveProxy(ctx context.Context, name string, p config.Proxy) tea.Cmd {
	return func() tea.Msg {
		r := app.ProbeProxy(ctx, p)
		if err := r.Err(); err != nil {
			return savedMsg{name: name, probe: r, err: err}
		}
		cfg, err := config.Load()
		if err == nil {
			cfg.Proxies[name] = p
			err = cfg.Save()
		}
		return savedMsg{name: name, probe: r, err: err}
	}
}

func removeProxy(name string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err == nil {
			delete(cfg.Proxies, name)
			err = cfg.Save()
		}
		return removedMsg{name: name, err: err}
	}
}
