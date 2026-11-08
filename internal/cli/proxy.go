package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

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
			if p.Key == "" && p.KeyCmd == "" {
				if p.Key, err = u.secret("API key for " + name); err != nil {
					return err
				}
			}
			r := probe(cmd.Context(), p)
			report(u, name, p, r)
			if r.err() != nil && !force {
				return fmt.Errorf("not saved: %w (use --force to save anyway)", r.err())
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
	f.StringVar(&p.Key, "key", "", "client API key (prefer --key-cmd or the prompt)")
	f.StringVar(&p.KeyCmd, "key-cmd", "", "shell command that prints the client API key")
	f.StringVar(&p.ManagementKey, "management-key", "", "management key, enables login status in `proxy test`")
	f.StringVar(&p.ManagementKeyCmd, "management-key-cmd", "", "shell command that prints the management key")
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
				extra := ""
				if p.ManagementKey != "" || p.ManagementKeyCmd != "" {
					extra = ", management key"
				}
				u.println(u.s.Text.Bold(true).Render(name) + "  " + p.URL + u.s.Subtle.Render("  ("+key+extra+")"))
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
			for _, a := range agents {
				if st, err := a.status(cfg); err == nil && st.Route == args[0] {
					return fmt.Errorf("%s is routed to %s; run `agentctl use %s local` first", a.name, args[0], a.name)
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
		Short: "Check health, key, models and (with a management key) upstream logins",
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
				r := probe(cmd.Context(), p)
				report(u, name, p, r)
				if r.err() != nil {
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
			list, err := fetchModels(cmd.Context(), p)
			if err != nil {
				return err
			}
			width := 0
			for _, m := range list {
				width = max(width, len(m.ID))
			}
			for _, m := range list {
				var who string
				switch c := models.Classify(m.ID, m.OwnedBy, p.AllowModels); c {
				case models.Anthropic:
					who = u.s.Accent.Render("Claude Code only")
				case models.Other:
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
			u.println(u.s.Subtle.Render("Re-run `agentctl use codex " + args[0] + "` to refresh Codex's model list."))
			return nil
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "remove the models from the allow list instead")
	return cmd
}

// probeResult is everything `proxy test` learns about one proxy.
type probeResult struct {
	healthErr error
	latency   time.Duration
	models    []proxy.Model
	modelsErr error
	logins    []proxy.Login
	loginsErr error
	noMgmt    bool
}

func (r probeResult) err() error {
	return errors.Join(r.healthErr, r.modelsErr)
}

func probe(ctx context.Context, p config.Proxy) probeResult {
	var r probeResult
	c, err := proxy.New(p.URL)
	if err != nil {
		r.healthErr = err
		return r
	}
	start := time.Now()
	if r.healthErr = c.Health(ctx); r.healthErr != nil {
		return r
	}
	r.latency = time.Since(start)
	r.models, r.modelsErr = fetchModels(ctx, p)
	mk, err := p.ResolveManagementKey(ctx)
	switch {
	case err != nil:
		r.loginsErr = err
	case mk == "":
		r.noMgmt = true
	default:
		r.logins, r.loginsErr = c.Logins(ctx, mk)
	}
	return r
}

func fetchModels(ctx context.Context, p config.Proxy) ([]proxy.Model, error) {
	c, err := proxy.New(p.URL)
	if err != nil {
		return nil, err
	}
	key, err := p.ResolveKey(ctx)
	if err != nil {
		return nil, err
	}
	list, err := c.Models(ctx, key)
	if proxy.IsUnauthorized(err) {
		return nil, errors.New("the proxy rejected the API key")
	}
	return list, err
}

func report(u *ui, name string, p config.Proxy, r probeResult) {
	u.println(u.s.Title.Render(name) + "  " + u.s.Subtle.Render(p.URL))
	if r.healthErr != nil {
		u.fail("unreachable: " + r.healthErr.Error())
		return
	}
	u.ok(fmt.Sprintf("healthy (%s)", r.latency.Round(time.Millisecond)))
	if r.modelsErr != nil {
		u.fail("models: " + r.modelsErr.Error())
	} else {
		var claude, other, unknown int
		for _, m := range r.models {
			switch models.Classify(m.ID, m.OwnedBy, p.AllowModels) {
			case models.Anthropic:
				claude++
			case models.Other:
				other++
			default:
				unknown++
			}
		}
		u.ok(fmt.Sprintf("key accepted, %d models: %d Claude (Claude Code only), %d for all agents, %d unverified", len(r.models), claude, other, unknown))
	}
	switch {
	case r.noMgmt:
		u.println(u.s.Subtle.Render("  add --management-key to see upstream login health"))
	case r.loginsErr != nil:
		u.fail("logins: " + r.loginsErr.Error())
	default:
		for _, l := range r.logins {
			loginLine(u, l)
		}
	}
}

func loginLine(u *ui, l proxy.Login) {
	who := l.Provider + "  " + l.Email
	switch {
	case l.Disabled:
		u.println(u.s.Subtle.Render("  - " + who + "  disabled"))
	case l.Quota.Exceeded:
		until := ""
		if !l.Quota.NextRecoverAt.IsZero() {
			until = " for " + time.Until(l.Quota.NextRecoverAt).Round(time.Minute).String()
		}
		u.println(u.s.Warning.Render("  ! " + who + "  quota exhausted" + until))
	case l.Status == "error" || l.Unavailable:
		msg := l.StatusMessage
		if msg == "" {
			msg = "unavailable"
		}
		u.println(u.s.Error.Render("  ✗ " + who + "  " + msg))
	default:
		u.println(u.s.Success.Render("  ✓ ") + who + u.s.Subtle.Render("  "+l.Status))
	}
}
