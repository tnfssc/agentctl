package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/ocweb"
	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

func newOpenCodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "opencode",
		Short: "Run OpenCode's web server on this machine: start, pairing and updates",
		Long: "Manage OpenCode's background server, which also serves its web app, through OpenCode's own `opencode`\n" +
			"CLI (v2). It is not a system service: any opencode command starts it, and it stays down after a\n" +
			"reboot until something does.",
		Example: `  agentctl opencode start
  agentctl opencode url https://box.tailnet.ts.net
  agentctl opencode pair`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return printOpenCodeStatus(cmd) },
	}
	cmd.AddCommand(
		&cobra.Command{
			Use: "status", Short: "Show the server, its address and version", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return printOpenCodeStatus(cmd) },
		},
		newOpenCodeStepCmd("start", "Start the server", "", "", "OpenCode's server is running", ocweb.Start),
		newOpenCodeStepCmd("restart", "Restart the server", "Restart OpenCode's server?", "Running agent turns stop.",
			"OpenCode's server restarted", ocweb.Restart),
		newOpenCodeStepCmd("stop", "Stop the server", "Stop OpenCode's server?",
			"Running agent turns stop. The next opencode command starts it again.", "OpenCode's server stopped", ocweb.Stop),
		newOpenCodeUpdateCmd(),
		newOpenCodeStepCmd("signout", "Sign out every paired browser and app",
			"Sign out every paired browser and app?", signOutNote, "Signed out every paired device", ocweb.SignOutAll),
		newOpenCodePairCmd(), newOpenCodeURLCmd(), newOpenCodeRemoteCmd(),
	)
	return cmd
}

const signOutNote = "OpenCode keeps no list of devices: this gives the server a new password, which ends every " +
	"paired session. Devices need a new link; the server restarts, which stops running agent turns."

func printOpenCodeStatus(cmd *cobra.Command) error {
	u := newUI(cmd)
	st, err := ocweb.Inspect(cmd.Context())
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	row := func(k, v string) { u.println(u.s.Subtle.Render(fmt.Sprintf("%-9s", k)) + v) }
	switch {
	case !st.Installed():
		u.fail("OpenCode is not installed.")
		return nil
	case !st.Supported():
		u.fail("OpenCode " + st.Version + " has no background server; it came with v2.")
		return nil
	case st.Running:
		u.ok("OpenCode's server is running at " + st.URL)
	default:
		u.warn("OpenCode's server is stopped. Start it with `agentctl opencode start`.")
	}
	row("version", st.Version)
	switch {
	case st.UpdateAvailable():
		row("update", u.s.Accent.Render(st.Latest)+u.s.Subtle.Render(" · agentctl opencode update"))
	case st.LatestErr != nil:
		row("update", u.s.Subtle.Render(st.LatestErr.Error()))
	case st.OnLatest():
		row("update", u.s.Subtle.Render("on the newest release"))
	}
	if st.Serving != "" {
		row("serving", st.Serving+u.s.Subtle.Render(" · restart to run "+st.Version))
	}
	if st.Remote {
		row("remote", "on · reachable from the internet over OpenTunnel")
	}
	switch base := ocweb.LinkBase(cfg.OpenCode.URL, st); {
	case base != "":
		row("pair at", base)
	case st.Remote:
		row("pair at", "the OpenTunnel address")
	}
	return nil
}

// newOpenCodeStepCmd is a command that runs one server step, asking first
// when ask is set.
func newOpenCodeStepCmd(use, short, ask, note, done string, step func(context.Context, io.Writer) error) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := ocweb.Inspect(cmd.Context())
			if err != nil {
				return err
			}
			if err := openCodeUsable(st); err != nil {
				return err
			}
			if ask != "" && !yes {
				ok, err := u.confirm(ask, note)
				if err != nil || !ok {
					return err
				}
			}
			if err := step(cmd.Context(), u.err); err != nil {
				return err
			}
			u.ok(done)
			return nil
		},
	}
	if ask != "" {
		cmd.Flags().BoolVarP(&yes, "yes", "y", false, use+" without asking")
	}
	return cmd
}

func newOpenCodeUpdateCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Upgrade opencode to the newest release and restart the server on it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := ocweb.Inspect(cmd.Context())
			if err != nil {
				return err
			}
			if err := openCodeUsable(st); err != nil {
				return err
			}
			if st.OnLatest() {
				u.ok("Already on the newest release, " + st.Latest)
				return nil
			}
			if !yes {
				what := "Upgrade OpenCode to " + st.Latest + "?"
				if st.Latest == "" {
					what = "Upgrade OpenCode?"
				}
				ok, err := u.confirm(what, "Runs `opencode upgrade`, then restarts the server, which stops running agent turns.")
				if err != nil || !ok {
					return err
				}
			}
			if err := ocweb.Update(cmd.Context(), u.err); err != nil {
				return err
			}
			u.ok("OpenCode updated")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "update without asking")
	return cmd
}

func openCodeUsable(st ocweb.Status) error {
	switch {
	case !st.Installed():
		return ocweb.ErrNotInstalled
	case !st.Supported():
		return errors.New("OpenCode " + st.Version + " has no background server; agentctl needs v2")
	}
	return nil
}

func newOpenCodePairCmd() *cobra.Command {
	var (
		base   string
		remote bool
	)
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Make a one-time link that signs in a browser or app",
		Long: "Prints a one-time link and its QR code; open it on the new device to use OpenCode's web app.\n" +
			"Each device needs its own link, and stays signed in for 30 days. Starts the server if it is stopped.",
		Example: `  agentctl opencode pair
  agentctl opencode pair --url https://box.tailnet.ts.net
  agentctl opencode pair --remote`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := ocweb.Inspect(cmd.Context())
			if err != nil {
				return err
			}
			if err := openCodeUsable(st); err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if remote && base != "" {
				return errors.New("--remote cannot be combined with --url")
			}
			if base == "" && !remote {
				base = ocweb.LinkBase(cfg.OpenCode.URL, st)
				remote = base == "" && st.Remote
			}
			var p ocweb.Pairing
			switch {
			case remote:
				if !st.Remote {
					return errors.New("remote access is off; turn it on with `agentctl opencode remote on`")
				}
				p, err = ocweb.PairRemote(cmd.Context())
			case base == "":
				return errors.New("no address for the link: the server isn't running; start it with `agentctl opencode start`, or pass --url")
			default:
				if base, err = t3.NormalizeURL(base); err == nil {
					p, err = ocweb.Pair(cmd.Context(), base)
				}
			}
			if err != nil {
				return err
			}
			if !remote && t3.LocalOnly(base) {
				u.warn("this link only works on this machine; set the address devices use with `agentctl opencode url <url>`")
			}
			// Drawn in color only; see `agentctl t3 pair`.
			if isTerminal(u.out) && colorprofile.Detect(u.out, os.Environ()) >= colorprofile.ANSI {
				if rows, err := skin.QR(p.URL); err == nil {
					u.println(strings.Join(rows, "\n"))
				}
			}
			u.println(u.s.Text.Bold(true).Render(p.URL))
			u.println(u.s.Subtle.Render("one device, expires " + t3.Ago(time.Now(), p.Expires)))
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "url", "", "address devices use to reach the server (default: `agentctl opencode url`)")
	cmd.Flags().BoolVar(&remote, "remote", false, "link through the OpenTunnel address (needs `agentctl opencode remote on`)")
	return cmd
}

func newOpenCodeRemoteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remote [on|off]",
		Short: "Show, or turn on or off, access from the internet over OpenTunnel",
		Long: "OpenCode can reach the server from anywhere through OpenTunnel, at an unguessable address; links from\n" +
			"`agentctl opencode pair` then use it unless `agentctl opencode url` is set. Turning it on or off restarts\n" +
			"the server; the first time on, OpenCode sets up this machine's tunnel, which can take a minute.",
		Example: `  agentctl opencode remote on
  agentctl opencode remote off`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			st, err := ocweb.Inspect(cmd.Context())
			if err != nil {
				return err
			}
			if err := openCodeUsable(st); err != nil {
				return err
			}
			if len(args) == 0 {
				if st.Remote {
					u.println("on · reachable from the internet over OpenTunnel")
				} else {
					u.println(u.s.Subtle.Render("off · only addresses that reach this machine work"))
				}
				return nil
			}
			on := args[0] == "on"
			if !on && args[0] != "off" {
				return fmt.Errorf("want on or off, not %q", args[0])
			}
			if on == st.Remote {
				u.ok("remote access is already " + args[0])
				return nil
			}
			if !yes {
				what, note := "Turn on remote access?", ocweb.RemoteWarning+" The server restarts, which stops running agent turns."
				if !on {
					what, note = "Turn off remote access?", "Devices paired over OpenTunnel lose access. The server restarts, which stops running agent turns."
				}
				ok, err := u.confirm(what, note)
				if err != nil || !ok {
					return err
				}
			}
			if err := ocweb.SetRemote(cmd.Context(), u.err, on); err != nil {
				return err
			}
			u.ok("remote access is " + args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "change it without asking")
	return cmd
}

func newOpenCodeURLCmd() *cobra.Command {
	var forget bool
	cmd := &cobra.Command{
		Use:   "url [<url>]",
		Short: "Show or set the address pairing links use",
		Long: "OpenCode's server listens on 127.0.0.1, which only this machine reaches. Set the address other\n" +
			"devices use instead, e.g. a Tailscale Serve URL in front of it.",
		Example: `  agentctl opencode url https://box.tailnet.ts.net
  agentctl opencode url --clear`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			switch {
			case forget:
				cfg.OpenCode.URL = ""
			case len(args) == 1:
				if cfg.OpenCode.URL, err = t3.NormalizeURL(args[0]); err != nil {
					return err
				}
			default:
				if cfg.OpenCode.URL == "" {
					u.println(u.s.Subtle.Render("not set; links use the address the server listens on"))
				} else {
					u.println(cfg.OpenCode.URL)
				}
				return nil
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if cfg.OpenCode.URL == "" {
				u.ok("pairing links use the address the server listens on")
			} else {
				u.ok("pairing links use " + cfg.OpenCode.URL)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&forget, "clear", false, "forget the address")
	return cmd
}
