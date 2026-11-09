// Package cli wires the agentctl command tree.
package cli

import (
	"context"
	"os"
	"runtime/debug"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// Set through -ldflags at release time; see scripts/release-build.sh.
var (
	version = ""
	commit  = ""
)

// Execute runs the root command with fang's styled help, errors, version,
// completion and man page support.
func Execute(ctx context.Context) error {
	return fang.Execute(ctx, newRootCmd(),
		fang.WithVersion(buildVersion()),
		fang.WithCommit(commit),
		fang.WithNotifySignal(os.Interrupt),
	)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "agentctl",
		Short: "Switch coding agents between their own login and CLIProxyAPI servers",
		Long: skin.New(true).Logo() + "\n\n" +
			"agentctl routes Claude Code, Codex, OpenCode and Pi either to their own login or to a CLIProxyAPI\n" +
			"server, by editing the agents' own config files. Claude models only ever run through Claude Code.",
		Example: `  agentctl proxy add spark --url https://spark.example.ts.net:8445
  agentctl proxy test
  agentctl use codex spark
  agentctl use claude local
  agentctl tune claude
  agentctl t3 pair
  agentctl status`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			if u.interactive() {
				return runInteractive(cmd.Context(), u)
			}
			return printStatus(u)
		},
	}
	root.AddCommand(newProxyCmd(), newUseCmd(), newRefreshCmd(), newTuneCmd(), newT3Cmd(), newStatusCmd(), newTokenCmd())
	return root
}

// buildVersion prefers the ldflags value and falls back to the module
// version that `go install pkg@vX` records.
func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
