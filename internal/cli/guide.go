package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/guide"
)

func newGuideCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "guide [<topic>]",
		Short: "Print setup instructions for your coding agent to follow",
		Long: "Prints instructions written for a coding agent, for setups agentctl leaves to you: commit signing\n" +
			"keys, memory limits for agents and Docker, a Cloudflare Tunnel to T3 Code and OpenCode. Paste the\n" +
			"command into your agent (e.g. `run agentctl guide signing`) and it walks you through the steps.\n" +
			"Setups that change the machine have an undo guide, `<topic>-undo`.",
		Example:   "  agentctl guide memory\n  agentctl guide tunnel-undo",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: guide.Topics(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				for _, t := range guide.Topics() {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), t)
				}
				return nil
			}
			text, err := guide.Get(args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), text)
			return err
		},
	}
}
