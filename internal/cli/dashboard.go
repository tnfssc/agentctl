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
//
// ctx ends on SIGINT. In the dashboard ctrl+c is a key, so SIGINT comes from
// outside, or from ctrl+c while the terminal is handed to another program
// (a CLIProxyAPI login): that one is meant for the program, not for agentctl.
func runInteractive(ctx context.Context, u *ui) error {
	dctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			if !dash.HandedOver() {
				cancel()
			}
		case <-dctx.Done():
		}
	}()
	ctx = dctx
	_, err := tea.NewProgram(dash.New(ctx, dash.Options{Remote: overSSH(), Version: buildVersion()}),
		tea.WithContext(ctx),
		tea.WithInput(u.in),
		tea.WithOutput(u.out),
	).Run()
	return err
}
