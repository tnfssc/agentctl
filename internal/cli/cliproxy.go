package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/cliproxy"
	"github.com/tnfssc/agentctl/internal/config"
)

func newCLIProxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cliproxy",
		Short: "Run a CLIProxyAPI server on this machine: install, keys, logins and updates",
		Long: "Installs CLIProxyAPI from its GitHub releases, runs it at login under launchd or systemd, and\n" +
			"attaches it as a proxy agents can route through. agentctl keeps the API keys and the management key,\n" +
			"which CLIProxyAPI hashes once it starts, so `agentctl cliproxy keys` can show them later.",
		Example: `  agentctl cliproxy install
  agentctl cliproxy login codex
  agentctl cliproxy keys
  agentctl cliproxy keys --management | pbcopy`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return printCLIProxyStatus(cmd) },
	}
	cmd.AddCommand(
		&cobra.Command{
			Use: "status", Short: "Show the server, its address, version and logins", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return printCLIProxyStatus(cmd) },
		},
		newCLIProxyInstallCmd(),
		newCLIProxyStepCmd("start", "Start the server and run it at login", "", "", "CLIProxyAPI is running", cliproxy.Start),
		newCLIProxyStepCmd("restart", "Restart the server", "Restart CLIProxyAPI?", "Requests in flight fail.",
			"CLIProxyAPI restarted", cliproxy.Restart),
		newCLIProxyStepCmd("stop", "Stop the server until started again", "Stop CLIProxyAPI?",
			"Agents routed through it fail until it starts again; it stays stopped after a reboot.", "CLIProxyAPI stopped", cliproxy.Stop),
		newCLIProxyUpdateCmd(), newCLIProxyKeysCmd(), newCLIProxyLoginCmd(), newCLIProxyListenCmd(), newCLIProxyUninstallCmd(),
	)
	return cmd
}

func printCLIProxyStatus(cmd *cobra.Command) error {
	u := newUI(cmd)
	st, err := cliproxy.Inspect(cmd.Context(), true)
	if err != nil {
		return err
	}
	row := func(k, v string) { u.println(u.s.Subtle.Render(fmt.Sprintf("%-10s", k)) + v) }
	switch {
	case !st.Installed:
		u.warn("CLIProxyAPI is not installed here. Install it with `agentctl cliproxy install`.")
		return nil
	case st.Running:
		u.ok("CLIProxyAPI is running at " + st.URL())
	case st.Enabled:
		u.warn("CLIProxyAPI is starting, or failing to: see " + logPath())
	default:
		u.warn("CLIProxyAPI is stopped. Start it with `agentctl cliproxy start`.")
	}
	row("version", st.Version)
	switch {
	case st.UpdateAvailable():
		row("update", u.s.Accent.Render(st.Latest)+u.s.Subtle.Render(" · agentctl cliproxy update"))
	case st.LatestErr != nil:
		row("update", u.s.Subtle.Render(st.LatestErr.Error()))
	case st.OnLatest():
		row("update", u.s.Subtle.Render("on the newest release"))
	}
	row("listen", listenText(st))
	row("api keys", strconv.Itoa(len(st.APIKeys))+u.s.Subtle.Render(" · agentctl cliproxy keys"))
	row("panel", st.PanelURL()+u.s.Subtle.Render(" · management key: agentctl cliproxy keys --management"))
	if len(st.Logins) == 0 {
		row("logins", u.s.Subtle.Render("none yet · agentctl cliproxy login"))
	}
	for i, l := range st.Logins {
		k := ""
		if i == 0 {
			k = "logins"
		}
		row(k, loginText(l))
	}
	for _, w := range cliproxyWarnings(st) {
		u.warn(w)
	}
	return nil
}

func logPath() string {
	p, _ := cliproxy.LogPath()
	return tildeDir(p)
}

func listenText(st cliproxy.Status) string {
	if st.AllInterfaces() {
		return "every interface, port " + strconv.Itoa(st.Port)
	}
	return st.Host + ":" + strconv.Itoa(st.Port) + " (this machine only)"
}

func loginText(l cliproxy.Login) string {
	if l.Account == "" {
		return l.Provider
	}
	return l.Provider + " · " + l.Account
}

// cliproxyWarnings are problems worth a line in the status.
func cliproxyWarnings(st cliproxy.Status) []string {
	var out []string
	if st.NoLinger {
		out = append(out, "systemd stops this user's services at logout, taking CLIProxyAPI down with the last SSH session; "+
			"`loginctl enable-linger` keeps it running")
	}
	if !st.ManagementSet {
		out = append(out, "no management key: the management API and panel are off; `agentctl cliproxy keys rotate-management` sets one")
	} else if st.ManagementKey == "" {
		out = append(out, "agentctl has no copy of the management key (CLIProxyAPI keeps only a hash); "+
			"`agentctl cliproxy keys rotate-management` sets a new one")
	}
	for _, l := range st.Logins {
		if l.Provider == "claude" {
			out = append(out, "it holds a Claude login, which Anthropic's terms forbid and which can get the account banned; "+
				"delete "+tildeDir(l.File)+" to remove it")
			break
		}
	}
	return out
}

func newCLIProxyInstallCmd() *cobra.Command {
	var opts cliproxy.InstallOptions
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the newest release, start it at login and attach it as a proxy",
		Long: "Downloads CLIProxyAPI for this machine from GitHub, checks it against the release's checksums and\n" +
			"writes a config from the release's example: listening on 127.0.0.1 only, with a new API key and\n" +
			"management key. It then runs at login under launchd (macOS) or systemd --user (Linux) and is attached\n" +
			"as a proxy. A config left by an earlier install is kept.",
		Example: `  agentctl cliproxy install
  agentctl cliproxy install --port 8318 --name box`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			if err := cliproxy.Install(cmd.Context(), u.err, opts); err != nil {
				return err
			}
			u.ok("CLIProxyAPI is running. Next: `agentctl cliproxy login` to add an account.")
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&opts.Port, "port", 0, "port to listen on (default 8317)")
	f.StringVar(&opts.Proxy, "name", cliproxy.DefaultProxy, "proxy name to attach it under")
	return cmd
}

// newCLIProxyStepCmd runs one server step, asking first when ask is set.
func newCLIProxyStepCmd(use, short, ask, note, done string, step func(context.Context, io.Writer) error) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
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

func newCLIProxyUpdateCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Install the newest release and restart the server on it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := cliproxy.Inspect(cmd.Context(), true)
			if err != nil {
				return err
			}
			if !st.Installed {
				return cliproxy.ErrNotInstalled
			}
			if st.LatestErr != nil {
				return st.LatestErr
			}
			if st.OnLatest() {
				u.ok("Already on the newest release, " + st.Latest)
				return nil
			}
			if !yes {
				ok, err := u.confirm("Update CLIProxyAPI to "+st.Latest+"?", "The server restarts on it; requests in flight fail.")
				if err != nil || !ok {
					return err
				}
			}
			v, err := cliproxy.Update(cmd.Context(), u.err)
			if err != nil {
				return err
			}
			u.ok("CLIProxyAPI updated to " + v)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "update without asking")
	return cmd
}

func newCLIProxyKeysCmd() *cobra.Command {
	var api, mgmt bool
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Show the API keys and the management key",
		Long: "API keys are what agents and other clients send; the management key opens the management API and\n" +
			"its web panel. --api and --management print one key alone, for pipes.",
		Example: `  agentctl cliproxy keys
  agentctl cliproxy keys --api
  agentctl cliproxy keys add
  agentctl cliproxy keys rm sk-...`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := cliproxy.Inspect(cmd.Context(), false)
			if err != nil {
				return err
			}
			if !st.Installed {
				return cliproxy.ErrNotInstalled
			}
			switch {
			case api && mgmt:
				return errors.New("use --api or --management, not both")
			case api:
				if len(st.APIKeys) == 0 {
					return errors.New("the config has no API keys")
				}
				_, err := fmt.Fprintln(cmd.OutOrStdout(), st.APIKeys[0])
				return err
			case mgmt:
				if st.ManagementKey == "" {
					return errors.New("agentctl has no copy of the management key; `agentctl cliproxy keys rotate-management` sets a new one")
				}
				_, err := fmt.Fprintln(cmd.OutOrStdout(), st.ManagementKey)
				return err
			}
			for i, k := range st.APIKeys {
				label := ""
				if i == 0 {
					label = "api key"
				}
				u.println(u.s.Subtle.Render(fmt.Sprintf("%-16s", label)) + k)
			}
			mk := st.ManagementKey
			if mk == "" {
				mk = u.s.Subtle.Render("unknown · agentctl cliproxy keys rotate-management")
			}
			u.println(u.s.Subtle.Render(fmt.Sprintf("%-16s", "management key")) + mk)
			u.println(u.s.Subtle.Render(fmt.Sprintf("%-16s", "url")) + st.URL() + u.s.Subtle.Render(" · panel "+st.PanelURL()))
			return nil
		},
	}
	cmd.Flags().BoolVar(&api, "api", false, "print only the first API key")
	cmd.Flags().BoolVar(&mgmt, "management", false, "print only the management key")
	cmd.AddCommand(
		&cobra.Command{
			Use: "add", Short: "Add a new API key and print it", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				k, err := cliproxy.AddKey()
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), k)
				return err
			},
		},
		&cobra.Command{
			Use: "rm <key>", Short: "Remove an API key; clients using it stop working", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := cliproxy.RemoveKey(args[0]); err != nil {
					return err
				}
				newUI(cmd).ok("removed " + cliproxy.Mask(args[0]))
				return nil
			},
		},
		newCLIProxyRotateCmd(),
	)
	return cmd
}

func newCLIProxyRotateCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rotate-management",
		Short: "Set a new management key and print it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			if !yes {
				ok, err := u.confirm("Set a new management key?", "The old key stops working; a running server restarts to apply it.")
				if err != nil || !ok {
					return err
				}
			}
			k, err := cliproxy.RotateManagement(cmd.Context(), u.err)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), k)
			return err
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "rotate without asking")
	return cmd
}

func newCLIProxyLoginCmd() *cobra.Command {
	var names []string
	for _, p := range cliproxy.Providers {
		names = append(names, p.Name)
	}
	cmd := &cobra.Command{
		Use:   "login [<provider>]",
		Short: "Add an account through CLIProxyAPI's own sign-in",
		Long: "Runs CLIProxyAPI's login for a provider: open the link it prints and sign in. Over SSH the browser\n" +
			"can't reach this machine; after a few seconds it asks for the URL the browser ended on, so paste that.\n" +
			"Providers: " + strings.Join(names, ", ") + ". Claude asks first: routing a Claude\n" +
			"subscription through a proxy breaks Anthropic's terms and can get the account banned.",
		Example: `  agentctl cliproxy login
  agentctl cliproxy login codex-device`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: names,
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			if !u.interactive() {
				return errors.New("logins need a terminal: they print a link and may ask for the URL you end on")
			}
			var name string
			if len(args) == 1 {
				name = args[0]
			} else {
				var opts []huh.Option[string]
				for _, p := range cliproxy.Providers {
					opts = append(opts, huh.NewOption(p.Title+" · "+p.Note, p.Name))
				}
				if err := ask(huh.NewSelect[string]().Title("Sign in to").Options(opts...).Value(&name)); err != nil {
					return err
				}
			}
			p, ok := cliproxy.FindProvider(name)
			if !ok {
				return fmt.Errorf("unknown provider %q; one of %s", name, strings.Join(names, ", "))
			}
			if p.Risk != "" {
				u.warn(p.Risk)
				ok, err := u.confirm("Sign in to "+p.Title+" anyway?", "Your account, your call.")
				if err != nil || !ok {
					return err
				}
			}
			c, err := cliproxy.LoginCmd(cmd.Context(), p)
			if err != nil {
				return err
			}
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("%s login: %w", p.Title, err)
			}
			u.ok(p.Title + " login saved; a running server picks it up on its own")
			return nil
		},
	}
	return cmd
}

func newCLIProxyListenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "listen <local|all>",
		Short: "Listen on this machine only, or on every interface",
		Long: "local (the default) answers only on 127.0.0.1; put Tailscale Serve or a tunnel in front to reach it\n" +
			"from elsewhere. all answers on every interface, so anyone on your networks can try the API keys.\n" +
			"The management API stays local-only either way. A running server restarts.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"local", "all"},
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			if args[0] != "local" && args[0] != "all" {
				return errors.New("listen local or listen all")
			}
			if err := cliproxy.Listen(cmd.Context(), u.err, args[0] == "all"); err != nil {
				return err
			}
			st, err := cliproxy.Inspect(cmd.Context(), false)
			if err != nil {
				return err
			}
			u.ok("CLIProxyAPI listens on " + listenText(st))
			return nil
		},
	}
	return cmd
}

// routedThrough names the agents that route through proxy.
func routedThrough(proxy string) []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range app.Agents {
		if st, err := a.Status(cfg); err == nil && st.Route == proxy && st.Route != agent.Local {
			out = append(out, a.Title)
		}
	}
	return out
}

func newCLIProxyUninstallCmd() *cobra.Command {
	var yes, all bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the server and remove it; --all also deletes its config and keys",
		Long: "Removes the service and the binary, keeping the config so a later install has the same keys. --all\n" +
			"also deletes the config, agentctl's copy of the management key, and the proxy entry. Logins stay in\n" +
			"CLIProxyAPI's auth directory either way; delete it yourself to sign them out.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			st, err := cliproxy.Inspect(cmd.Context(), false)
			if err != nil {
				return err
			}
			if !st.Installed && !st.Service && !all {
				u.ok("CLIProxyAPI is not installed")
				return nil
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if who := routedThrough(cfg.CLIProxy.Proxy); cfg.CLIProxy.Proxy != "" && len(who) > 0 {
				return fmt.Errorf("%s route through %s; switch them first (`agentctl use <agent> local`)", strings.Join(who, ", "), cfg.CLIProxy.Proxy)
			}
			if !yes {
				what := "Agents can't reach it until it's installed again; the config and keys stay."
				if all {
					what = "Its config, API keys and management key are deleted, and agentctl forgets the proxy."
				}
				ok, err := u.confirm("Uninstall CLIProxyAPI?", what)
				if err != nil || !ok {
					return err
				}
			}
			if err := cliproxy.Uninstall(cmd.Context(), all); err != nil {
				return err
			}
			u.ok("CLIProxyAPI uninstalled")
			if st.AuthDir != "" {
				u.println(u.s.Subtle.Render("Logins kept in " + tildeDir(st.AuthDir)))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "uninstall without asking")
	cmd.Flags().BoolVar(&all, "all", false, "also delete the config, its keys and the proxy entry")
	return cmd
}
