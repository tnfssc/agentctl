package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show where each agent is routed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printStatus(newUI(cmd))
		},
	}
}

func printStatus(u *ui) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, a := range agents {
		st, err := a.status(cfg)
		if err != nil {
			u.fail(fmt.Sprintf("%-12s %v", a.title, err))
			continue
		}
		route := u.s.Success.Render("own login")
		if st.Route != agent.Local {
			route = u.s.Accent.Render("proxy " + st.Route)
		}
		var detail []string
		if st.Model != "" {
			detail = append(detail, st.Model)
		}
		if st.Route == "custom" && st.URL != "" {
			detail = append(detail, st.URL)
		}
		u.println(u.s.Text.Bold(true).Render(fmt.Sprintf("%-12s", a.title)) + " " + route + "  " + u.s.Subtle.Render(strings.Join(detail, "  ")))
		for _, n := range st.Notes {
			u.warn(n)
		}
	}
	return nil
}
