package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/config"
)

// noKey is what agents send to a proxy that needs no key.
const noKey = "agentctl-no-key"

// newTokenCmd prints a proxy's client key. Claude Code (apiKeyHelper) and
// Codex (provider auth command) call it, so keys never sit in their configs.
func newTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "token <proxy>",
		Short:  "Print a proxy's API key (used by the agents, not by you)",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			p, err := cfg.Lookup(args[0])
			if err != nil {
				return err
			}
			key, err := p.ResolveKey(cmd.Context())
			if err != nil {
				return err
			}
			// Agents insist on some credential; a proxy without keys ignores it.
			if key == "" {
				key = noKey
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), key)
			return err
		},
	}
}
