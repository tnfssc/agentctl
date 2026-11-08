package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/tui/theme"
)

// ui prints styled output for one command. Styling goes through lipgloss so
// colors are downsampled to the terminal and dropped when piped.
type ui struct {
	out, err io.Writer
	in       io.Reader
	s        theme.Styles
}

func newUI(cmd *cobra.Command) *ui {
	return &ui{out: cmd.OutOrStdout(), err: cmd.ErrOrStderr(), in: cmd.InOrStdin(), s: theme.New(true)}
}

// Terminal write errors are not actionable mid-command, so they are dropped.

func (u *ui) println(a ...any) { _, _ = lipgloss.Fprintln(u.out, a...) }

func (u *ui) warn(msg string) { _, _ = lipgloss.Fprintln(u.err, u.s.Warning.Render("! ")+msg) }

func (u *ui) ok(msg string) { u.println(u.s.Success.Render("✓ ") + msg) }

func (u *ui) fail(msg string) { u.println(u.s.Error.Render("✗ ") + msg) }

// diff prints a unified diff with added and removed lines colored.
func (u *ui) diff(d string) {
	for line := range strings.Lines(d) {
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			u.println(u.s.Title.Render(line))
		case strings.HasPrefix(line, "+"):
			u.println(u.s.Success.Render(line))
		case strings.HasPrefix(line, "-"):
			u.println(u.s.Error.Render(line))
		case strings.HasPrefix(line, "@@"):
			u.println(u.s.Accent.Render(line))
		default:
			u.println(u.s.Subtle.Render(line))
		}
	}
}

// interactive reports whether we can prompt: both ends must be a terminal.
func (u *ui) interactive() bool { return isTerminal(u.in) && isTerminal(u.out) }

var errNeedsYes = errors.New("not a terminal; re-run with --yes to apply")

// confirm asks a yes/no question, or returns errNeedsYes when it cannot.
func (u *ui) confirm(title, description string) (bool, error) {
	if !u.interactive() {
		return false, errNeedsYes
	}
	ok := false
	err := ask(huh.NewConfirm().
		Title(title).
		Description(description).
		Affirmative("Yes").
		Negative("No").
		Value(&ok))
	if errors.Is(err, huh.ErrUserAborted) {
		return false, nil
	}
	return ok, err
}

// secret prompts for a value without echoing it.
func (u *ui) secret(title string) (string, error) {
	if !u.interactive() {
		return "", fmt.Errorf("%s: not a terminal; pass it as a flag", title)
	}
	var v string
	err := ask(huh.NewInput().
		Title(title).
		EchoMode(huh.EchoModePassword).
		Value(&v))
	return strings.TrimSpace(v), err
}

// ask runs one field as a form on stderr, so stdout stays clean for output.
func ask(field huh.Field) error {
	return huh.NewForm(huh.NewGroup(field)).
		WithAccessible(os.Getenv("ACCESSIBLE") != "").
		WithOutput(os.Stderr).
		Run()
}

// isTerminal reports whether v is a real TTY. A ModeCharDevice check is not
// enough: /dev/null is a character device too.
func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}
