package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/proxy"
)

// routeStatus is one agent's current route, as its own config describes it.
type routeStatus struct {
	Route, URL, Model string
	Notes             []string
}

type agentDef struct {
	name, title string
	status      func(*config.Config) (routeStatus, error)
}

var agents = []agentDef{
	{claude.Name, "Claude Code", func(cfg *config.Config) (routeStatus, error) {
		s, err := claude.Current(cfg)
		return routeStatus(s), err
	}},
	{codex.Name, "Codex", func(cfg *config.Config) (routeStatus, error) {
		s, err := codex.Current(cfg)
		return routeStatus(s), err
	}},
}

func agentNames() []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.name
	}
	return names
}

func newUseCmd() *cobra.Command {
	var (
		model       string
		yes, dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "use <agent> <local|proxy>",
		Short: "Route an agent to its own login or to a proxy",
		Long: "Route Claude Code or Codex to its own login (local) or to an attached proxy.\n\n" +
			"local uses whatever the agent itself is logged in with; nothing is copied. A proxy uses the\n" +
			"accounts logged in on that proxy. Codex only ever sees the proxy's non-Claude models.",
		Example: `  agentctl use codex spark --model gpt-6.1-sol
  agentctl use claude local
  agentctl use codex local --dry-run`,
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return agentNames(), cobra.ShellCompDirectiveNoFileComp
			case 1:
				cfg, err := config.Load()
				if err != nil {
					return nil, cobra.ShellCompDirectiveError
				}
				return append([]string{agent.Local}, cfg.Names()...), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUse(cmd.Context(), newUI(cmd), useOpts{agent: args[0], route: args[1], model: model, yes: yes, dryRun: dryRun})
		},
	}
	f := cmd.Flags()
	f.StringVar(&model, "model", "", "model to use on the proxy (default: keep the current one if the proxy has it)")
	f.BoolVarP(&yes, "yes", "y", false, "apply without asking")
	f.BoolVar(&dryRun, "dry-run", false, "show the changes without applying them")
	return cmd
}

func describeRoute(route string) string {
	if route == agent.Local {
		return "its own login"
	}
	return "proxy " + route
}

type useOpts struct {
	agent, route, model string
	yes, dryRun         bool
}

// runUse plans, shows, confirms and applies one route change.
func runUse(ctx context.Context, u *ui, o useOpts) error {
	name, route := o.agent, o.route
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := config.LoadState()
	if err != nil {
		return err
	}
	var p config.Proxy
	if route != agent.Local {
		if p, err = cfg.Lookup(route); err != nil {
			return err
		}
	}

	var plan agent.Plan
	switch name {
	case claude.Name:
		if route != agent.Local {
			list, err := fetchModels(ctx, p)
			if err != nil {
				return fmt.Errorf("%s: %w", route, err)
			}
			if o.model != "" && !slices.ContainsFunc(list, func(m proxy.Model) bool { return m.ID == o.model }) {
				return fmt.Errorf("%s does not serve %q; see `agentctl proxy models %s`", route, o.model, route)
			}
			if !st.ClaudeProxyAcknowledged {
				u.warn(claude.PolicyWarning)
				if !o.yes && !o.dryRun {
					ok, err := u.confirm("Route Claude Code through "+route+" anyway?", "You will not be asked again.")
					if err != nil || !ok {
						return errors.Join(errors.New("cancelled"), err)
					}
				}
				st.ClaudeProxyAcknowledged = !o.dryRun
			}
		}
		plan, err = claude.PlanUse(cfg, route, o.model)
	case codex.Name:
		var cat codex.Catalog
		if route != agent.Local {
			if o.model != "" && models.Classify(o.model, "", nil) == models.Anthropic {
				return fmt.Errorf("%q is a Claude model; Claude models are only allowed in Claude Code", o.model)
			}
			list, err := fetchModels(ctx, p)
			if err != nil {
				return fmt.Errorf("%s: %w", route, err)
			}
			if cat, err = codex.BuildCatalog(ctx, list, p.AllowModels); err != nil {
				return err
			}
			if err := codex.RememberLocalModel(st); err != nil {
				return err
			}
		}
		plan, err = codex.PlanUse(cfg, st, route, o.model, cat)
	default:
		return fmt.Errorf("unknown agent %q; choose one of %v", name, agentNames())
	}
	if err != nil {
		return err
	}

	for _, w := range plan.Warnings {
		u.warn(w)
	}
	if plan.Empty() {
		u.ok(fmt.Sprintf("%s already uses %s", name, route))
		return st.Save()
	}
	u.diff(plan.Diff())
	if o.dryRun {
		return nil
	}
	if !o.yes {
		ok, err := u.confirm("Apply these changes?", "Backups go to ~/.config/agentctl/backups.")
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("cancelled")
		}
	}
	if err := plan.Apply(); err != nil {
		return err
	}
	if err := st.Save(); err != nil {
		return err
	}
	u.ok(fmt.Sprintf("%s now uses %s. Restart running %s sessions to pick it up.", name, describeRoute(route), name))
	return nil
}
