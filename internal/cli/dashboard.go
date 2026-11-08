package cli

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/tui/dash"
)

// runInteractive opens the dashboard; bare `agentctl` in a terminal.
func runInteractive(ctx context.Context, u *ui) error {
	_, err := tea.NewProgram(dash.New(ctx),
		tea.WithContext(ctx),
		tea.WithInput(u.in),
		tea.WithOutput(u.out),
	).Run()
	return err
}
