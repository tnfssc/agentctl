package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/tools"
)

func newUseCmd() *cobra.Command {
	var (
		model       string
		yes, dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "use <agent> <local|proxy>",
		Short: "Route an agent to its own login or to a proxy",
		Long: "Route Claude Code, Codex, OpenCode or Pi to its own login (local) or to an attached proxy.\n\n" +
			"local uses whatever the agent itself is logged in with; nothing is copied. A proxy uses the\n" +
			"accounts logged in on that proxy. Only Claude Code sees the proxy's Claude models; OpenCode\n" +
			"reaches them through its claude-agent provider, which runs Claude Code.",
		Example: `  agentctl use codex spark --model gpt-6.1-sol
  agentctl use opencode spark
  agentctl use pi spark --model gpt-6.1-sol
  agentctl use claude local
  agentctl use codex local --dry-run`,
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return app.AgentNames(), cobra.ShellCompDirectiveNoFileComp
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
			req := app.Request{Agent: args[0], Route: args[1], Model: model}
			return runUse(cmd.Context(), newUI(cmd), req, yes, dryRun)
		},
	}
	f := cmd.Flags()
	f.StringVar(&model, "model", "", "default model on the proxy for Codex, OpenCode or Pi (Claude Code uses /model)")
	f.BoolVarP(&yes, "yes", "y", false, "apply without asking")
	f.BoolVar(&dryRun, "dry-run", false, "show the changes without applying them")
	return cmd
}

func newRefreshCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Rebuild the model lists of agents routed through a proxy",
		Long: "Codex, OpenCode and Pi get a snapshot of the proxy's models when they are routed to it. Run this\n" +
			"after the proxy gains or drops models, or its key changes. Claude Code needs no refresh; it asks\n" +
			"the proxy at startup.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			cfg, st, err := loadAll()
			if err != nil {
				return err
			}
			plan, err := app.Refresh(cmd.Context(), cfg, st)
			if errors.Is(err, app.ErrNothingToRefresh) {
				u.ok(err.Error())
				return nil
			}
			if err != nil {
				return err
			}
			return confirmAndApply(cmd.Context(), u, plan, st, yes, false)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without asking")
	return cmd
}

func loadAll() (*config.Config, *config.State, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	st, err := config.LoadState()
	return cfg, st, err
}

// runUse plans, shows, confirms and applies one route change.
func runUse(ctx context.Context, u *ui, req app.Request, yes, dryRun bool) error {
	cfg, st, err := loadAll()
	if err != nil {
		return err
	}
	if _, err := app.FindAgent(req.Agent); err != nil {
		return err
	}
	if app.NeedsClaudeAck(st, req) {
		u.warn(claude.PolicyWarning)
		if !yes && !dryRun {
			ok, err := u.confirm("Route Claude Code through "+req.Route+" anyway?", "You will not be asked again.")
			if err != nil || !ok {
				return errors.Join(errors.New("cancelled"), err)
			}
		}
	}
	plan, err := app.Plan(ctx, cfg, st, req)
	if err != nil {
		return err
	}
	return confirmAndApply(ctx, u, plan, st, yes, dryRun)
}

func confirmAndApply(ctx context.Context, u *ui, plan agent.Plan, st *config.State, yes, dryRun bool) error {
	for _, w := range plan.Warnings {
		u.warn(w)
	}
	if plan.Empty() {
		if plan.Agent == app.ModelLists {
			u.ok("model lists are already up to date")
			return nil
		}
		if plan.Route == "" {
			u.ok(plan.Agent + " settings already match; nothing to change")
		} else {
			u.ok(fmt.Sprintf("%s already uses %s; nothing to change", plan.Agent, describeRoute(plan.Route)))
		}
		if !dryRun {
			connectClaude(ctx, u, plan)
		}
		return nil
	}
	u.diff(plan.Diff())
	if dryRun {
		return nil
	}
	if !yes {
		ok, err := u.confirm("Apply these changes?", "Backups go to ~/.config/agentctl/backups.")
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("cancelled")
		}
	}
	if err := app.Apply(ctx, u.err, plan, st); err != nil {
		return err
	}
	if plan.Agent == app.ModelLists {
		u.ok("Model lists updated. Restart running sessions to pick them up.")
		return nil
	}
	if plan.Agent == tools.Name {
		u.ok("Web search and MCP tools installed. `tvly login` lifts Tavily's rate cap; new sessions see the skills.")
		return nil
	}
	if plan.Route == "" {
		u.ok(fmt.Sprintf("%s settings updated. Restart running %s sessions to pick them up.", plan.Agent, plan.Agent))
	} else {
		u.ok(fmt.Sprintf("%s now uses %s. Restart running %s sessions to pick it up.", plan.Agent, describeRoute(plan.Route), plan.Agent))
	}
	connectClaude(ctx, u, plan)
	return nil
}

// connectClaude turns on OpenCode's claude-agent after an OpenCode route.
// A failure is a warning: the route itself is in place.
func connectClaude(ctx context.Context, u *ui, plan agent.Plan) {
	switch on, err := app.ConnectOpenCodeClaude(ctx, plan); {
	case err != nil:
		u.warn("couldn't turn on OpenCode's claude-agent, so it lists no Claude models: " + err.Error() +
			"; run `opencode auth login claude-agent --method claude-login`")
	case on:
		u.ok(app.ClaudeConnected)
	}
}

func describeRoute(route string) string {
	if route == agent.Local {
		return "its own login"
	}
	return "proxy " + route
}
