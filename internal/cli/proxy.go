package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/proxy"
)

func newProxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Attach, test and inspect CLIProxyAPI servers",
	}
	cmd.AddCommand(newProxyAddCmd(), newProxyListCmd(), newProxyRemoveCmd(), newProxyTestCmd(), newProxyModelsCmd(), newProxyAllowCmd())
	return cmd
}

func newProxyAddCmd() *cobra.Command {
	var (
		p     config.Proxy
		force bool
	)
	cmd := &cobra.Command{
		Use:   "add <name> --url <url>",
		Short: "Attach a proxy and check that it works",
		Example: `  # Key typed at a prompt, never stored in shell history
  agentctl proxy add home --url http://127.0.0.1:8317

  # Key read on demand from the machine that owns it
  agentctl proxy add spark --url https://spark.tailnet.ts.net:8445 \
    --key-cmd "ssh spark 'grep ^CLIPROXY_API_KEY= ~/cliproxyapi/.env | cut -d= -f2'"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, name := newUI(cmd), args[0]
			if err := config.ValidName(name); err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if _, exists := cfg.Proxies[name]; exists && !force {
				return fmt.Errorf("proxy %q exists; use --force to replace it", name)
			}
			if p.URL, err = proxy.NormalizeURL(p.URL); err != nil {
				return err
			}
			if p.Key != "" && p.KeyCmd != "" {
				return errors.New("use --key or --key-cmd, not both")
			}
			if p.Key == "" && p.KeyCmd == "" && u.interactive() {
				if p.Key, err = u.secret("API key for " + name + " (empty if the proxy has none)"); err != nil {
					return err
				}
			}
			r := app.ProbeProxy(cmd.Context(), p)
			report(u, name, p, r)
			if r.Err() != nil && !force {
				return fmt.Errorf("not saved: %w (use --force to save anyway)", r.Err())
			}
			cfg.Proxies[name] = p
			if err := cfg.Save(); err != nil {
				return err
			}
			path, _ := config.Path()
			u.ok(fmt.Sprintf("saved %s to %s", name, path))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&p.URL, "url", "", "proxy base URL, e.g. http://127.0.0.1:8317")
	f.StringVar(&p.Key, "key", "", "client API key, if the proxy needs one (prefer --key-cmd or the prompt)")
	f.StringVar(&p.KeyCmd, "key-cmd", "", "shell command that prints the client API key")
	f.BoolVar(&force, "force", false, "replace an existing proxy, or save one that fails its check")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func newProxyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List attached proxies",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if len(cfg.Proxies) == 0 {
				u.println(u.s.Subtle.Render("No proxies yet. Add one with `agentctl proxy add <name> --url <url>`."))
				return nil
			}
			for _, name := range cfg.Names() {
				p := cfg.Proxies[name]
				key := "key inline"
				if p.KeyCmd != "" {
					key = "key via command"
				}
				u.println(u.s.Text.Bold(true).Render(name) + "  " + p.URL + u.s.Subtle.Render("  ("+key+")"))
			}
			return nil
		},
	}
}

func newProxyRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rm <name>",
		Aliases: []string{"remove"},
		Short:   "Detach a proxy",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if _, err := cfg.Lookup(args[0]); err != nil {
				return err
			}
			for _, a := range app.Agents {
				if st, err := a.Status(cfg); err == nil && st.Route == args[0] {
					return fmt.Errorf("%s is routed to %s; run `agentctl use %s local` first", a.Name, args[0], a.Name)
				}
			}
			delete(cfg.Proxies, args[0])
			if err := cfg.Save(); err != nil {
				return err
			}
			u.ok("removed " + args[0])
			return nil
		},
	}
}

func newProxyTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test [name...]",
		Short: "Check health, key and models, and what Claude Code would see",
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			names := args
			if len(names) == 0 {
				names = cfg.Names()
			}
			if len(names) == 0 {
				return errors.New("no proxies yet; add one with `agentctl proxy add`")
			}
			var failed []string
			for i, name := range names {
				p, err := cfg.Lookup(name)
				if err != nil {
					return err
				}
				if i > 0 {
					u.println()
				}
				r := app.ProbeProxy(cmd.Context(), p)
				report(u, name, p, r)
				if r.Err() != nil {
					failed = append(failed, name)
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("failed: %s", strings.Join(failed, ", "))
			}
			return nil
		},
	}
}

func newProxyModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models <name>",
		Short: "List a proxy's models and which agents may use them",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			p, err := cfg.Lookup(args[0])
			if err != nil {
				return err
			}
			list, err := app.FetchModels(cmd.Context(), p)
			if err != nil {
				return err
			}
			width := 0
			for _, m := range list {
				width = max(width, len(m.ID))
			}
			for _, m := range list {
				var who string
				switch c := models.Classify(m.ID, m.OwnedBy, p.AllowModels); {
				case c == models.Anthropic:
					who = u.s.Accent.Render("Claude Code only")
				case !models.Chat(m.ID):
					who = u.s.Subtle.Render("not a chat model")
				case c == models.Other:
					who = u.s.Success.Render("all agents")
				default:
					who = u.s.Warning.Render("Claude Code only (unverified; `agentctl proxy allow " + args[0] + " " + m.ID + "`)")
				}
				u.println(fmt.Sprintf("%-*s  ", width, m.ID) + u.s.Subtle.Render(fmt.Sprintf("%-14s", m.OwnedBy)) + who)
			}
			return nil
		},
	}
}

func newProxyAllowCmd() *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:   "allow <name> <model...>",
		Short: "Let non-Claude agents use models agentctl cannot verify",
		Long: "Models whose vendor agentctl cannot prove are hidden from every agent except Claude Code.\n" +
			"Allow them here once you know they are not Anthropic's. Anything that looks like Claude stays blocked.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			p, err := cfg.Lookup(args[0])
			if err != nil {
				return err
			}
			for _, m := range args[1:] {
				if remove {
					p.AllowModels = slices.DeleteFunc(p.AllowModels, func(a string) bool { return a == m })
					continue
				}
				if models.Classify(m, "", nil) == models.Anthropic {
					return fmt.Errorf("%q looks like a Claude model; Claude models are only allowed in Claude Code", m)
				}
				if !slices.Contains(p.AllowModels, m) {
					p.AllowModels = append(p.AllowModels, m)
				}
			}
			cfg.Proxies[args[0]] = p
			if err := cfg.Save(); err != nil {
				return err
			}
			u.ok(fmt.Sprintf("%s allow list: %s", args[0], strings.Join(p.AllowModels, ", ")))
			u.println(u.s.Subtle.Render("Run `agentctl refresh` to update the model lists of agents routed to " + args[0] + "."))
			return nil
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "remove the models from the allow list instead")
	return cmd
}

func report(u *ui, name string, p config.Proxy, r app.Probe) {
	u.println(u.s.Title.Render(name) + "  " + u.s.Subtle.Render(p.URL))
	if r.HealthErr != nil {
		u.fail("unreachable: " + r.HealthErr.Error())
		return
	}
	u.ok(fmt.Sprintf("healthy (%s)", r.Latency.Round(time.Millisecond)))
	if r.ModelsErr != nil {
		u.fail("models: " + r.ModelsErr.Error())
		return
	}
	u.ok(fmt.Sprintf("key accepted, %d models: %s", len(r.Models), r.Counts))
	switch {
	case r.DiscoveryErr != nil:
		u.warn("Claude Code's /model list will not show proxy models: " + r.DiscoveryErr.Error())
	case r.SlowDiscovery():
		u.warn(fmt.Sprintf("the proxy took %s to list models; Claude Code gives up after 3s and shows only built-in models",
			r.DiscoveryLatency.Round(time.Millisecond)))
	case len(r.Disguised) > 0:
		u.ok(fmt.Sprintf("Claude Code's /model lists the %d non-Claude models as claude-fable-5-dd-<name reversed> (e.g. %s)",
			len(r.Disguised), r.Disguised[0]))
	case r.Counts.Other+r.Counts.Unknown > 0:
		u.ok("Claude Code's /model lists only the Claude models; it drops ids that don't start with claude")
	default:
		u.ok("Claude Code's /model lists the proxy's models")
	}
}
