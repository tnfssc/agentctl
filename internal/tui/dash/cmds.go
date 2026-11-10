package dash

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/guide"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/tune"
)

// Results of background work. Every slow call (network, `codex debug
// models`, file writes) runs in a command and reports back with one of these.
type (
	loadedMsg struct {
		cfg        *config.Config
		st         *config.State
		statuses   []agentStatus
		signed     bool
		signingKey string
		applied    []string // guides whose setup is on record
		err        error
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
		saved   string // what was saved before planning, as the start of a toast
	}
	appliedMsg struct {
		plan      agent.Plan
		err       error
		claude    string // what turning on OpenCode's claude-agent did, if anything
		claudeErr bool
	}
	// claudeMsg reports turning on OpenCode's claude-agent for a route that
	// was already in place.
	claudeMsg struct {
		toast string
		err   bool
	}
	tuneLoadedMsg struct {
		agent string
		items []tune.Status
		err   error
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
	msg.signed, msg.signingKey = tune.SigningStatus()
	msg.applied = guide.Applied()
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
		p, err := app.Refresh(ctx, cfg, st)
		return plannedMsg{plan: p, err: err, refresh: true}
	}
}

func inspectTune(agentName string) tea.Cmd {
	return func() tea.Msg {
		items, err := tune.Inspect(agentName)
		return tuneLoadedMsg{agent: agentName, items: items, err: err}
	}
}

func planTune(agentName string, want map[string]bool) tea.Cmd {
	return func() tea.Msg {
		p, err := tune.Plan(agentName, want)
		return plannedMsg{plan: p, err: err}
	}
}

func apply(ctx context.Context, p agent.Plan, st *config.State) tea.Cmd {
	return func() tea.Msg {
		if err := app.Apply(p, st); err != nil {
			return appliedMsg{plan: p, err: err}
		}
		msg, failed := connectClaude(ctx, p)
		return appliedMsg{plan: p, claude: msg, claudeErr: failed}
	}
}

// connectClaude turns on OpenCode's claude-agent for an OpenCode route and
// says what happened: "" when nothing needed doing.
func connectClaude(ctx context.Context, p agent.Plan) (msg string, failed bool) {
	switch on, err := app.ConnectOpenCodeClaude(ctx, p); {
	case err != nil:
		return "Couldn't turn on OpenCode's claude-agent, so it lists no Claude models: " + err.Error(), true
	case on:
		return app.ClaudeConnected, false
	}
	return "", false
}

func connectClaudeCmd(ctx context.Context, p agent.Plan, done string) tea.Cmd {
	return func() tea.Msg {
		msg, failed := connectClaude(ctx, p)
		if msg == "" {
			msg = done
		}
		return claudeMsg{toast: msg, err: failed}
	}
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
