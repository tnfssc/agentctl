package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

func newT3Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "t3",
		Short: "Run T3 Code on this machine: service, updates, pairing and devices",
		Long: "Manage the T3 Code server on this machine through T3 Code's own `t3` CLI. agentctl installs and\n" +
			"updates it on the nightly channel as a background service, so updates started from a desktop app\n" +
			"reach this server too.",
		Example: `  agentctl t3 install
  agentctl t3 pair --label phone
  agentctl t3 devices
  agentctl t3 revoke --idle`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return printT3Status(cmd) },
	}
	cmd.AddCommand(
		&cobra.Command{
			Use: "status", Short: "Show the server, its version and any update", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return printT3Status(cmd) },
		},
		newT3InstallCmd(), newT3UpdateCmd(), newT3RestartCmd(), newT3UninstallCmd(),
		newT3PairCmd(), newT3URLCmd(), newT3DevicesCmd(), newT3RevokeCmd(),
	)
	return cmd
}

func printT3Status(cmd *cobra.Command) error {
	u := newUI(cmd)
	st, err := t3.Inspect(cmd.Context())
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
		u.fail("T3 Code is not installed. Set it up with `agentctl t3 install`.")
	case !st.ServiceInstalled:
		state := "installed"
		if st.Running {
			state = "running" + labelSuffix(st.Label)
		}
		u.warn("T3 Code is " + state + ", but not as a background service: it won't start at boot and desktop-app " +
			"updates can't reach it. Set it up with `agentctl t3 install`.")
	case st.Running:
		u.ok("T3 Code is running" + labelSuffix(st.Label))
	default:
		u.fail("T3 Code's service is installed but the server is not answering. Try `agentctl t3 restart`.")
	}
	if st.Version != "" {
		row("version", st.Version)
	} else if st.ServiceVersion != "" {
		row("version", st.ServiceVersion+u.s.Subtle.Render(" (service)"))
	}
	switch {
	case st.UpdateAvailable():
		row("update", u.s.Accent.Render(st.Latest)+u.s.Subtle.Render(" · agentctl t3 update"))
	case st.LatestErr != nil:
		row("update", u.s.Subtle.Render(st.LatestErr.Error()))
	case st.OnLatest():
		row("update", u.s.Subtle.Render("on the newest nightly"))
	case st.Latest != "":
		row("nightly", st.Latest)
	}
	if st.Update != "" {
		row("last", st.Update+reasonSuffix(st.UpdateReason))
	}
	if base := t3.LinkBase(cfg.T3.URL, st); base != "" {
		row("pair at", base)
	}
	if st.Installed() {
		row("data", st.Home)
	}
	return nil
}

func labelSuffix(label string) string {
	if label == "" {
		return ""
	}
	return " as " + label
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}

func newT3InstallCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install T3 Code (nightly) and run it as a background service",
		Long: "Runs T3 Code's installer (https://t3.codes/install.sh) for the nightly channel if t3 is missing, then\n" +
			"`t3 service install`: a per-user systemd or launchd service that starts at boot. Running it again\n" +
			"repairs the service.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := t3.Inspect(cmd.Context())
			if err != nil {
				return err
			}
			if !st.ServiceSupported && st.Installed() {
				return errors.New("T3 Code's background service is not supported on this system")
			}
			what := "Install T3 Code (nightly) and run it as a background service?"
			if st.Installed() {
				what = "Install T3 Code's background service?"
				if st.ServiceInstalled {
					what = "Reinstall T3 Code's background service? This repairs it and restarts the server."
				}
			}
			if !yes {
				ok, err := u.confirm(what, "Running agent turns stop if the server restarts.")
				if err != nil || !ok {
					return err
				}
			}
			if err := t3.Install(cmd.Context(), u.err); err != nil {
				return err
			}
			u.ok("T3 Code is set up. Pair a device with `agentctl t3 pair`.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "install without asking")
	return cmd
}

func newT3UpdateCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update T3 Code to the newest nightly and restart its service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := t3.Inspect(cmd.Context())
			if err != nil {
				return err
			}
			if !st.Installed() {
				return t3.ErrNotInstalled
			}
			if st.OnLatest() {
				u.ok("Already on the newest nightly, " + st.Latest)
				return nil
			}
			if !yes {
				ok, err := u.confirm("Update T3 Code to "+orNewest(st.Latest)+"?", "The service restarts, which stops running agent turns.")
				if err != nil || !ok {
					return err
				}
			}
			if err := t3.Update(cmd.Context(), u.err); err != nil {
				return err
			}
			u.ok("T3 Code updated")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "update without asking")
	return cmd
}

func orNewest(v string) string {
	if v == "" {
		return "the newest nightly"
	}
	return v
}

func newT3RestartCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart T3 Code's background service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := t3.Inspect(cmd.Context())
			if err != nil {
				return err
			}
			if !st.ServiceInstalled {
				return errors.New("T3 Code is not running as a background service here; set it up with `agentctl t3 install`")
			}
			if !yes {
				ok, err := u.confirm("Restart T3 Code?", "Running agent turns stop.")
				if err != nil || !ok {
					return err
				}
			}
			if err := t3.Restart(cmd.Context(), u.err); err != nil {
				return err
			}
			u.ok("T3 Code restarted")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "restart without asking")
	return cmd
}

func newT3UninstallCmd() *cobra.Command {
	var all, yes bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove T3 Code's background service, or with --all t3 itself",
		Long: "Stops and removes T3 Code's background service; t3, your projects and threads stay, and\n" +
			"`agentctl t3 install` brings the service back. With --all, `t3 uninstall` also removes the launcher and\n" +
			"every downloaded version; projects and threads still stay.",
		Example: `  agentctl t3 uninstall
  agentctl t3 uninstall --all`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			what, step := "Uninstall T3 Code's background service?", t3.Uninstall
			if all {
				what, step = "Remove T3 Code from this machine (service, launcher and every downloaded version)?", t3.UninstallAll
			}
			if !yes {
				ok, err := u.confirm(what, "The server stops. Projects and threads stay.")
				if err != nil || !ok {
					return err
				}
			}
			if err := step(cmd.Context(), u.err); err != nil {
				return err
			}
			if all {
				u.ok("T3 Code removed; projects and threads are still in its data folder")
			} else {
				u.ok("T3 Code's background service removed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "also remove t3: the launcher and every downloaded version")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "uninstall without asking")
	return cmd
}

func newT3PairCmd() *cobra.Command {
	var (
		label, base string
		ttl         time.Duration
	)
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Make a one-time link that pairs a new device",
		Long: "Prints a one-time pairing link and its QR code. Open it on the new device, or paste it into a\n" +
			"T3 Code app's Add environment. Each device needs its own link.",
		Example: `  agentctl t3 pair --label phone
  agentctl t3 pair --url https://box.tailnet.ts.net --ttl 15m`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := t3.Inspect(cmd.Context())
			if err != nil {
				return err
			}
			if !st.Installed() {
				return t3.ErrNotInstalled
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if base == "" {
				base = t3.LinkBase(cfg.T3.URL, st)
			}
			if base == "" {
				return errors.New("no address for the link: the server isn't running; start it with `agentctl t3 restart`, or pass --url")
			}
			if base, err = t3.NormalizeURL(base); err != nil {
				return err
			}
			p, err := t3.Pair(cmd.Context(), label, base, ttl)
			if err != nil {
				return err
			}
			if !st.Running {
				u.warn("the server isn't answering; the link works once it runs")
			}
			if t3.LocalOnly(base) {
				u.warn("this link only works on this machine; set the address devices use with `agentctl t3 url <url>`")
			}
			// The code is drawn in color; without color it would come out
			// inverted on dark terminals, which many scanners can't read.
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
	f := cmd.Flags()
	f.StringVar(&label, "label", "", "name shown for the device in `agentctl t3 devices`")
	f.StringVar(&base, "url", "", "address devices use to reach the server (default: `agentctl t3 url`)")
	f.DurationVar(&ttl, "ttl", t3.DefaultTTL, "how long the link works")
	return cmd
}

func newT3URLCmd() *cobra.Command {
	var forget bool
	cmd := &cobra.Command{
		Use:   "url [<url>]",
		Short: "Show or set the address pairing links use",
		Long: "Pairing links point at the address the server listens on, which is often only this machine.\n" +
			"Set the address other devices use instead, e.g. a Tailscale Serve URL.",
		Example: `  agentctl t3 url https://box.tailnet.ts.net
  agentctl t3 url --clear`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			switch {
			case forget:
				cfg.T3.URL = ""
			case len(args) == 1:
				if cfg.T3.URL, err = t3.NormalizeURL(args[0]); err != nil {
					return err
				}
			default:
				if cfg.T3.URL == "" {
					u.println(u.s.Subtle.Render("not set; links use the address the server listens on"))
				} else {
					u.println(cfg.T3.URL)
				}
				return nil
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if cfg.T3.URL == "" {
				u.ok("pairing links use the address the server listens on")
			} else {
				u.ok("pairing links use " + cfg.T3.URL)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&forget, "clear", false, "forget the address")
	return cmd
}

func newT3DevicesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "devices",
		Aliases: []string{"sessions"},
		Short:   "List paired devices and unused pairing links",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			all, err := t3.ListAccess(cmd.Context())
			if err != nil {
				return err
			}
			if len(all) == 0 {
				u.println(u.s.Subtle.Render("no devices yet; pair one with `agentctl t3 pair`"))
				return nil
			}
			now := time.Now()
			idle := 0
			for _, a := range all {
				what, when := a.Device, "connected "+t3.Ago(now, a.LastSeen)
				if a.Link {
					what, when = "pairing link", "expires "+t3.Ago(now, a.Expires)
				}
				line := fmt.Sprintf("%-8s  %-20s  %-22s  %s", a.ID[:min(8, len(a.ID))], cut(orDash(a.Label), 20), cut(orDash(what), 22), when)
				if a.Idle(now) {
					idle++
					line += u.s.Warning.Render("  idle")
				}
				u.println(line)
			}
			if idle > 0 {
				u.println(u.s.Subtle.Render(fmt.Sprintf("\n%d idle: paired devices that last connected over 7 days ago. One connected the whole\n"+
					"time looks the same; revoke them with `agentctl t3 revoke --idle`", idle)))
			}
			return nil
		},
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func newT3RevokeCmd() *cobra.Command {
	var idle, yes bool
	cmd := &cobra.Command{
		Use:   "revoke [<id>...]",
		Short: "Sign out devices or cancel unused pairing links",
		Long: "Revoke devices by the id (or its first characters) shown in `agentctl t3 devices`, or with --idle\n" +
			"every paired device that last connected over 7 days ago. A revoked device needs a new pairing link to\n" +
			"come back; one connected right now may keep working until it reconnects or T3 Code restarts.",
		Example: `  agentctl t3 revoke 66140145
  agentctl t3 revoke --idle`,
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			if idle == (len(args) > 0) {
				return errors.New("name devices to revoke, or pass --idle")
			}
			all, err := t3.ListAccess(cmd.Context())
			if err != nil {
				return err
			}
			var pick []t3.Access
			if idle {
				now := time.Now()
				for _, a := range all {
					if a.Idle(now) {
						pick = append(pick, a)
					}
				}
				if len(pick) == 0 {
					u.ok("no idle devices")
					return nil
				}
			} else {
				for _, id := range args {
					a, err := findAccess(all, id)
					if err != nil {
						return err
					}
					pick = append(pick, a)
				}
			}
			if !yes {
				ok, err := u.confirm(fmt.Sprintf("Revoke %d?", len(pick)), revokeNote)
				if err != nil || !ok {
					return err
				}
			}
			for _, a := range pick {
				if err := t3.Revoke(cmd.Context(), a); err != nil {
					return err
				}
				u.ok("revoked " + orDash(a.Label) + " " + u.s.Subtle.Render(a.ID))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&idle, "idle", false, "revoke every paired device that last connected over 7 days ago")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "revoke without asking")
	return cmd
}

const revokeNote = "Revoked devices need a new pairing link to come back. One connected right now may keep " +
	"working until it reconnects or T3 Code restarts."

// findAccess matches an id or a unique prefix of one.
func findAccess(all []t3.Access, id string) (t3.Access, error) {
	var found []t3.Access
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
		if strings.HasPrefix(a.ID, id) {
			found = append(found, a)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return t3.Access{}, fmt.Errorf("no device or link %q; see `agentctl t3 devices`", id)
	}
	return t3.Access{}, fmt.Errorf("%q matches %d devices; give more of the id", id, len(found))
}
