package cli

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/guide"
	"github.com/tnfssc/agentctl/internal/tune"
)

func newTuneCmd() *cobra.Command {
	var yes, dryRun, reset, risky bool
	var only []string
	cmd := &cobra.Command{
		Use:   "tune <claude|codex|instructions|git|all>",
		Short: "Apply agentctl's curated settings, or reset them",
		Long: "Pick curated settings for Claude Code or Codex (fewer unused tools, no background spend, privacy\n" +
			"without losing features), curated agent instructions, written to ~/.agents/AGENTS.md, or git settings\n" +
			"that make agents' git work smoother, in your global git config. Unticking an item resets it to the\n" +
			"default. Everything is recommended; `all` applies the whole set at once. Risky items (no approval\n" +
			"prompts, no sandbox) need their own yes, or --risky.",
		Example: `  agentctl tune claude             # pick interactively
  agentctl tune codex --yes        # apply the recommended set
  agentctl tune instructions       # shared behavior for every agent
  agentctl tune git                # merge on pull, clearer conflicts and diffs
  agentctl tune claude --reset     # undo every curated setting
  agentctl tune all --yes --risky  # everything, like agentctl's author
  agentctl tune instructions --only web,mcp --yes   # just these two; the rest stays as it is`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: append(app.AgentNames(), tune.Instructions, tune.Git, tune.All),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
			if args[0] == tune.All {
				if len(only) > 0 {
					return errors.New("--only picks items of one target, e.g. `agentctl tune instructions --only web,mcp`")
				}
				return tuneAll(cmd, u, yes, dryRun, reset, risky)
			}
			a := app.Agent{Name: tune.Instructions, Title: tune.InstructionsTitle}
			if args[0] == tune.Git {
				a = app.Agent{Name: tune.Git, Title: tune.GitTitle}
			} else if args[0] != tune.Instructions {
				var err error
				if a, err = app.FindAgent(args[0]); err != nil {
					return err
				}
			}
			statuses, err := tune.Inspect(a.Name)
			if err != nil {
				return err
			}
			want := tune.Recommended(statuses, risky)
			switch {
			case len(only) > 0:
				if reset {
					return errors.New("--only turns items on; to turn one off, untick it in `agentctl tune " + a.Name + "`")
				}
				if want, err = onlyThese(statuses, only); err != nil {
					return err
				}
			case reset:
				want = nil
			case u.interactive() && !yes:
				if want, err = pickTune(a.Title, statuses, tune.Recommended(statuses, true)); err != nil {
					return err
				}
				if !risky {
					if want, err = confirmRisky(u, statuses, want); err != nil {
						return err
					}
				}
			case !yes && !dryRun:
				printTune(u, statuses)
				return errors.New("not a terminal; re-run with --yes to apply the recommended settings")
			}
			plan, err := tune.Plan(a.Name, want)
			if err != nil {
				return err
			}
			st, err := config.LoadState()
			if err != nil {
				return err
			}
			if err := confirmAndApply(cmd.Context(), u, plan, st, yes, dryRun); err != nil {
				return err
			}
			if signed, _ := tune.SigningStatus(); a.Name == tune.Git && !signed {
				u.println(u.s.Subtle.Render("Commits aren't signed. To set it up, paste this into your coding agent: " +
					guide.Command("signing")))
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&yes, "yes", "y", false, "apply the recommended settings without asking")
	f.BoolVar(&dryRun, "dry-run", false, "show the changes without applying them")
	f.BoolVar(&reset, "reset", false, "reset every curated setting to the agent's default")
	f.BoolVar(&risky, "risky", false, "include the risky items (no approval prompts, no sandbox) without asking")
	f.StringSliceVar(&only, "only", nil, "turn on just these items (ids from the list), leaving the others as they are")
	return cmd
}

// onlyThese is what's applied now plus the named items, which must exist.
func onlyThese(statuses []tune.Status, ids []string) (map[string]bool, error) {
	want := map[string]bool{}
	var known []string
	for _, st := range statuses {
		want[st.ID] = st.Applied
		known = append(known, st.ID)
	}
	for _, id := range ids {
		if _, ok := want[id]; !ok {
			return nil, fmt.Errorf("no item %q; items: %s", id, strings.Join(known, ", "))
		}
		want[id] = true
	}
	return want, nil
}

// confirmRisky asks about each risky item want turns on; a no unticks it.
func confirmRisky(u *ui, statuses []tune.Status, want map[string]bool) (map[string]bool, error) {
	for _, st := range tune.RiskyNew(statuses, want) {
		ok, err := u.confirm("Turn on \""+st.Title+"\"?", st.Why+" Cost: "+st.Cost+".")
		if err != nil {
			return nil, err
		}
		want[st.ID] = ok
	}
	return want, nil
}

// tuneAll applies every target's recommended set in one reviewed diff.
func tuneAll(cmd *cobra.Command, u *ui, yes, dryRun, reset, risky bool) error {
	if reset {
		return errors.New("reset one target at a time, e.g. `agentctl tune claude --reset`")
	}
	if !yes && !dryRun && !u.interactive() {
		return errNeedsYes
	}
	pending, err := tune.RiskyAll()
	if err != nil {
		return err
	}
	if !risky && len(pending) > 0 && u.interactive() && !yes {
		var lines []string
		for _, st := range pending {
			lines = append(lines, "· "+st.Title+": "+st.Why)
		}
		if risky, err = u.confirm("Also turn on the risky items?", strings.Join(lines, "\n")); err != nil {
			return err
		}
	}
	plan, err := tune.PlanAll(risky)
	if err != nil {
		return err
	}
	st, err := config.LoadState()
	if err != nil {
		return err
	}
	if err := confirmAndApply(cmd.Context(), u, plan, st, yes, dryRun); err != nil {
		return err
	}
	if !risky && len(pending) > 0 {
		u.println(u.s.Subtle.Render("Left the risky items off; `agentctl tune all --risky` turns them on too."))
	}
	return nil
}

func pickTune(title string, statuses []tune.Status, want map[string]bool) (map[string]bool, error) {
	opts := make([]huh.Option[string], len(statuses))
	for i, st := range statuses {
		opts[i] = huh.NewOption(st.Title+"  ·  "+st.Cost, st.ID).Selected(want[st.ID])
	}
	var picked []string
	err := ask(huh.NewMultiSelect[string]().
		Title("Settings for " + title).
		Description("Ticked items are applied; unticked ones reset to the default. You review the diff next.").
		Options(opts...).
		Height(len(opts) + 4).
		Value(&picked))
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, id := range picked {
		out[id] = true
	}
	return out, nil
}

func printTune(u *ui, statuses []tune.Status) {
	for _, st := range statuses {
		mark := u.s.Subtle.Render("[ ]")
		if st.Applied {
			mark = u.s.Success.Render("[x]")
		}
		tag := ""
		if st.Recommended && !st.Applied {
			tag = u.s.Accent.Render("  recommended")
		}
		u.println(fmt.Sprintf("%s %s %s%s", mark, st.Title, u.s.Subtle.Render("("+st.ID+")"), tag))
		u.println(u.s.Subtle.Render("    " + st.Why))
		for _, n := range st.Notes {
			u.warn(n)
		}
	}
}
