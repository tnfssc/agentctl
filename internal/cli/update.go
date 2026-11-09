package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/selfupdate"
)

func newUpdateCmd() *cobra.Command {
	var check, yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update agentctl to the latest release",
		Long: "Downloads the latest release for this machine, checks it against the release's SHA256SUMS and\n" +
			"replaces this agentctl in place, so agents that call it keep working.",
		Example: `  agentctl update
  agentctl update --check`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			current := buildVersion()
			if !strings.HasPrefix(current, "v") {
				return errors.New("this agentctl was built from source (" + current + "); rebuild it, or install a release with the install script")
			}
			latest, err := selfupdate.Latest(cmd.Context())
			if err != nil {
				return err
			}
			if !selfupdate.Newer(current, latest) {
				u.ok("agentctl " + current + " is the latest release")
				return nil
			}
			if check {
				u.println(fmt.Sprintf("agentctl %s is out (you have %s); update with `agentctl update`", latest, current))
				return nil
			}
			if !yes {
				ok, err := u.confirm(fmt.Sprintf("Update agentctl %s → %s?", current, latest), "Running agentctl sessions keep the old version until restarted.")
				if err != nil || !ok {
					return err
				}
			}
			path, err := selfupdate.Install(cmd.Context(), latest)
			if err != nil {
				return err
			}
			u.ok(fmt.Sprintf("updated %s to %s", path, latest))
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only say whether a newer release exists")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "update without asking")
	return cmd
}
