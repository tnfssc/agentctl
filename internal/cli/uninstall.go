package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/guide"
	"github.com/tnfssc/agentctl/internal/selfupdate"
	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/tune"
)

// leftover is something agentctl set up that outlives its binary, and how
// to undo it while agentctl is still here.
type leftover struct{ what, undo string }

// leftovers lists what agentctl set up on this machine. Routed agents
// matter most: they call `agentctl token`, which stops working with the
// binary gone.
func leftovers(cmd *cobra.Command) ([]leftover, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var out []leftover
	for _, a := range app.Agents {
		if st, err := a.Status(cfg); err == nil && st.Route != agent.Local {
			out = append(out, leftover{a.Title + " routes through " + st.Route + " and breaks without agentctl", "agentctl use " + a.Name + " local"})
		}
	}
	for _, target := range []string{"claude", "codex", tune.Instructions, tune.Git} {
		items, err := tune.Inspect(target)
		if err != nil {
			continue
		}
		n := 0
		for _, it := range items {
			if it.Applied {
				n++
			}
		}
		if n > 0 {
			name := target
			if a, err := app.FindAgent(target); err == nil {
				name = a.Title
			}
			out = append(out, leftover{fmt.Sprintf("%d curated %s settings", n, name), "agentctl tune " + target + " --reset"})
		}
	}
	if st, err := t3.Inspect(cmd.Context()); err == nil && st.ServiceInstalled {
		out = append(out, leftover{"T3 Code's background service", "agentctl t3 uninstall"})
	}
	for _, topic := range guide.Applied() {
		out = append(out, leftover{
			"what your agent set up with `agentctl guide " + topic + "`",
			"paste `agentctl guide " + guide.Undo(topic) + "` into your coding agent",
		})
	}
	return out, nil
}

func newUninstallCmd() *cobra.Command {
	var yes, purge bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove agentctl from this machine",
		Long: "Deletes this agentctl binary, and with --purge its config directory too: proxies and their keys,\n" +
			"backups of agent configs and guide records. First it lists what agentctl set up that stays behind,\n" +
			"with the command that undoes each, and offers the dashboard to undo them; once agentctl is gone, those\n" +
			"commands and the undo guides are gone with it.",
		Example: `  agentctl uninstall
  agentctl uninstall --yes --purge`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u := newUI(cmd)
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			for again := false; ; again = true {
				left, err := leftovers(cmd)
				if err != nil {
					return err
				}
				if again && len(left) == 0 {
					u.ok("nothing agentctl set up is left")
				}
				if len(left) > 0 {
					u.warn("agentctl set up these, and they stay after it's gone:")
					for _, l := range left {
						u.println("  " + l.what + u.s.Subtle.Render("  ·  "+l.undo))
					}
				}
				if yes {
					break
				}
				if !u.interactive() {
					return errNeedsYes
				}
				choice, err := askUninstall(len(left) > 0)
				if err != nil || choice == "cancel" {
					return err
				}
				if choice == "dashboard" {
					if err := runInteractive(cmd.Context(), u); err != nil {
						return err
					}
					continue // look again after the dashboard closes
				}
				if _, err := os.Stat(dir); err == nil && !purge {
					if purge, err = u.confirm("Also delete "+tildeDir(dir)+"?",
						"Your proxies and their keys, backups of agent configs, and records of guides your agent followed."); err != nil {
						return err
					}
				}
				break
			}
			path, err := removeSelf()
			if err != nil {
				return err
			}
			u.ok("removed " + path)
			if purge {
				if err := os.RemoveAll(dir); err != nil {
					return err
				}
				u.ok("removed " + tildeDir(dir))
			} else if _, err := os.Stat(dir); err == nil {
				u.println(u.s.Subtle.Render("Kept " + tildeDir(dir) + "; delete it to remove your proxies' keys too."))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "uninstall without asking, leaving what agentctl set up as it is")
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete agentctl's config directory")
	return cmd
}

func askUninstall(left bool) (string, error) {
	opts := []huh.Option[string]{huh.NewOption("Uninstall agentctl", "uninstall"), huh.NewOption("Cancel", "cancel")}
	title := "Remove agentctl from this machine?"
	if left {
		title = "Undo those first?"
		opts = []huh.Option[string]{
			huh.NewOption("Open the dashboard to undo them", "dashboard"),
			huh.NewOption("Uninstall anyway, leaving them as they are", "uninstall"),
			huh.NewOption("Cancel", "cancel"),
		}
	}
	choice := opts[0].Value // huh starts on the option that holds the value
	err := ask(huh.NewSelect[string]().Title(title).Options(opts...).Value(&choice))
	if errors.Is(err, huh.ErrUserAborted) {
		return "cancel", nil
	}
	return choice, err
}

// removeSelf deletes the running binary, and the link it was started
// through when that was one.
func removeSelf() (string, error) {
	exe, err := selfupdate.Executable()
	if err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	if err := os.Remove(target); err != nil {
		return "", err
	}
	if target != exe {
		if err := os.Remove(exe); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return target, nil
}

func tildeDir(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && filepath.IsLocal(rel) {
			return "~/" + rel
		}
	}
	return path
}
