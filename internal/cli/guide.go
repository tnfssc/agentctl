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
		Long: "Prints instructions written for a coding agent, for setups agentctl leaves to you, such as commit\n" +
			"signing keys. Paste the command into your agent (e.g. `run agentctl guide signing`) and it walks you\n" +
			"through the steps.",
		Example:   "  agentctl guide signing",
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
