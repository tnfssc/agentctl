package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/tools"
)

func newToolsCmd() *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Install the web search and MCP tools agents use from the shell",
		Long: "Installs Tavily's CLI (uv or pipx) and mcpc (Homebrew or npm), and their skills into\n" +
			"~/.agents/skills, linked for Claude Code: what the shared Web and MCP instructions point agents at,\n" +
			"since built-in web search fails through a proxy. Only what's missing, never the vendors' init\n" +
			"scripts. `agentctl tune all` does this too. `tvly login` lifts Tavily's rate cap; Firecrawl and\n" +
			"other setups: `agentctl guide tools`; removing it: `agentctl guide tools-undo`.",
		Example: "  agentctl tools --dry-run\n  agentctl tools --yes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			if !yes && !dryRun && !u.interactive() {
				return errNeedsYes
			}
			runs, warnings := tools.Plan()
			if len(runs) == 0 && len(warnings) > 0 {
				return errors.New(strings.Join(warnings, "; "))
			}
			if len(runs) == 0 {
				u.ok("web search and MCP tools are installed; nothing to do")
				return nil
			}
			st, err := config.LoadState()
			if err != nil {
				return err
			}
			plan := agent.Plan{Agent: tools.Name, Runs: runs, Warnings: warnings}
			return confirmAndApply(cmd.Context(), u, plan, st, yes, dryRun)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "install without asking")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be installed")
	return cmd
}
