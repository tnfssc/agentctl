package cli

import (
	"context"
	"errors"

	"charm.land/huh/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
)

// runInteractive is bare `agentctl` in a terminal: show where things stand,
// then walk through one route change.
func runInteractive(ctx context.Context, u *ui) error {
	if err := printStatus(u); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Proxies) == 0 {
		u.println(u.s.Subtle.Render("\nNo proxies yet. Add one with `agentctl proxy add <name> --url <url>`."))
		return nil
	}
	u.println()

	o := useOpts{}
	agentOpts := make([]huh.Option[string], len(agents))
	for i, a := range agents {
		agentOpts[i] = huh.NewOption(a.title, a.name)
	}
	routeOpts := []huh.Option[string]{huh.NewOption("Its own login", agent.Local)}
	for _, n := range cfg.Names() {
		routeOpts = append(routeOpts, huh.NewOption("Proxy "+n+"  "+cfg.Proxies[n].URL, n))
	}
	err = ask(huh.NewSelect[string]().Title("Which agent?").Options(agentOpts...).Value(&o.agent))
	if err == nil {
		err = ask(huh.NewSelect[string]().Title("Route it to").Options(routeOpts...).Value(&o.route))
	}
	if err == nil && o.route != agent.Local {
		err = pickModel(ctx, cfg, &o)
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	if err != nil {
		return err
	}
	return runUse(ctx, u, o)
}

func pickModel(ctx context.Context, cfg *config.Config, o *useOpts) error {
	p, err := cfg.Lookup(o.route)
	if err != nil {
		return err
	}
	list, err := fetchModels(ctx, p)
	if err != nil {
		return err
	}
	opts := []huh.Option[string]{huh.NewOption("Keep the current model", "")}
	for _, m := range list {
		c := models.Classify(m.ID, m.OwnedBy, p.AllowModels)
		if o.agent != "claude" && !models.Usable(c) {
			continue
		}
		opts = append(opts, huh.NewOption(m.ID, m.ID))
	}
	return ask(huh.NewSelect[string]().
		Title("Model").
		Description("Claude models are only offered to Claude Code.").
		Options(opts...).
		Height(min(len(opts)+2, 15)).
		Value(&o.model))
}
