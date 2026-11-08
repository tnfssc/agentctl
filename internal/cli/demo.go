package cli

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/tnfssc/cliproxy-cli/internal/tui/demo"
)

func newDemoCmd() *cobra.Command {
	var speed float64
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Play an animated tour of the terminal UI",
		Long:  "Play a fake proxy boot sequence: gradient banner, spinners, a spring-animated progress bar and live status rows. Nothing touches the network.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if speed <= 0 {
				return fmt.Errorf("--speed must be positive, got %v", speed)
			}
			if !isTerminal(cmd.InOrStdin()) || !isTerminal(cmd.OutOrStdout()) {
				return errors.New("demo needs an interactive terminal")
			}
			p := tea.NewProgram(demo.New(speed),
				tea.WithContext(cmd.Context()),
				tea.WithInput(cmd.InOrStdin()),
				tea.WithOutput(cmd.OutOrStdout()),
			)
			_, err := p.Run()
			return err
		},
	}
	cmd.Flags().Float64Var(&speed, "speed", 1, "animation speed multiplier")
	return cmd
}

// isTerminal reports whether v is a real TTY. A ModeCharDevice check is not
// enough: /dev/null is a character device too.
func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}
