package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/app"
)

func newUpdateCmd() *cobra.Command {
	var check, yes bool
	cmd := &cobra.Command{
		Use:   "update [agentctl|claude|codex|opencode|pi|t3|all]...",
		Short: "Update agentctl, the agents' CLIs and the servers",
		Long: "Updates each tool through its own updater: agentctl replaces itself with the latest release\n" +
			"(checked against its SHA256SUMS), `claude update`, `codex update`, `opencode upgrade` (restarting\n" +
			"its server if it runs), `pi update --all` (pi and its extensions) and `t3 update` (restarting its\n" +
			"service). Without a tool it updates agentctl; `all` updates everything installed that has news.",
		Example: `  agentctl update
  agentctl update claude pi
  agentctl update all
  agentctl update --check all`,
		ValidArgs: append(slices.Clone(app.UpdateNames), "all"),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			want, err := updateTargets(args)
			if err != nil {
				return err
			}
			list := app.CheckUpdates(cmd.Context(), buildVersion(), want)
			if check {
				for _, up := range list {
					u.println(fmt.Sprintf("%-28s %s", up.Title, describeUpdate(u, up)))
				}
				return nil
			}
			var todo []app.Update
			for _, up := range list {
				switch {
				case !up.Installed:
					if len(want) == 1 {
						return fmt.Errorf("%s is not installed", up.Title)
					}
				case up.Name == "agentctl" && !strings.HasPrefix(up.Current, "v"):
					if len(want) == 1 {
						return errors.New("this agentctl was built from source (" + up.Current + "); rebuild it, or install a release with the install script")
					}
					u.println(u.s.Subtle.Render("agentctl " + up.Note))
				case up.Err != nil && !up.Worth:
					u.warn(up.Title + ": " + up.Err.Error())
				case !up.Worth:
					u.ok(up.Title + " " + up.Current + " is the newest release")
				default:
					todo = append(todo, up)
				}
			}
			if len(todo) == 0 {
				return nil
			}
			if !yes {
				var lines []string
				for _, up := range todo {
					lines = append(lines, "· "+up.Title+" "+arrow(up)+notes(up))
				}
				ok, err := u.confirm("Update these?", strings.Join(lines, "\n")+
					"\nRunning sessions keep the old versions until restarted.")
				if err != nil || !ok {
					return err
				}
			}
			var failed []error
			for _, up := range todo {
				u.println(u.s.Title.Render("Updating " + up.Title))
				if err := up.Run(cmd.Context(), u.err); err != nil {
					u.fail(err.Error())
					failed = append(failed, err)
					continue
				}
				u.ok("updated " + up.Title)
			}
			return errors.Join(failed...)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only show what's installed and what's newest")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "update without asking")
	return cmd
}

// updateTargets turns arguments into tool names: agentctl alone by default,
// every tool for "all".
func updateTargets(args []string) ([]string, error) {
	if len(args) == 0 {
		return []string{"agentctl"}, nil
	}
	var out []string
	for _, a := range args {
		switch {
		case a == "all":
			return app.UpdateNames, nil
		case !slices.Contains(app.UpdateNames, a):
			return nil, fmt.Errorf("can't update %q; tools: %s, all", a, strings.Join(app.UpdateNames, ", "))
		case !slices.Contains(out, a):
			out = append(out, a)
		}
	}
	return out, nil
}

func arrow(up app.Update) string {
	if up.Latest != "" && up.Latest != up.Current {
		return up.Current + " → " + up.Latest
	}
	return up.Current
}

func notes(up app.Update) string {
	if up.Note == "" {
		return ""
	}
	return " (" + up.Note + ")"
}

func describeUpdate(u *ui, up app.Update) string {
	switch {
	case !up.Installed:
		return u.s.Subtle.Render("not installed")
	case up.Name == "agentctl" && !strings.HasPrefix(up.Current, "v"):
		return up.Current + u.s.Subtle.Render(" · "+up.Note)
	case up.Worth && up.Latest != up.Current && up.Latest != "":
		return up.Current + " → " + u.s.Accent.Render(up.Latest)
	case up.Err != nil:
		return up.Current + u.s.Subtle.Render(" · can't check: "+up.Err.Error())
	case up.Worth:
		return up.Current + u.s.Subtle.Render(" · newest; its extras may have updates")
	}
	return up.Current + u.s.Subtle.Render(" · newest")
}
