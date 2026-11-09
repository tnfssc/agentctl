package cli

import (
	"context"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/tui/dash"
)

// overSSH reports whether this process runs in an SSH session.
func overSSH() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CLIENT") != ""
}

// runInteractive opens the dashboard; bare `agentctl` in a terminal.
func runInteractive(ctx context.Context, u *ui) error {
	_, err := tea.NewProgram(dash.New(ctx, dash.Options{Remote: overSSH(), Version: buildVersion()}),
		tea.WithContext(ctx),
		tea.WithInput(u.in),
		tea.WithOutput(u.out),
	).Run()
	return err
}
