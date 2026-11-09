package cli

import (
	"errors"
	"fmt"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/guide"
	"github.com/tnfssc/agentctl/internal/tune"
)

func newTuneCmd() *cobra.Command {
	var yes, dryRun, reset bool
	cmd := &cobra.Command{
		Use:   "tune <claude|codex|instructions|git>",
		Short: "Apply agentctl's curated settings, or reset them",
		Long: "Pick curated settings for Claude Code or Codex (fewer unused tools, no background spend, privacy\n" +
			"without losing features), curated agent instructions, written to ~/.agents/AGENTS.md, or git settings\n" +
			"that make agents' git work smoother, in your global git config. Unticking an item resets it to the\n" +
			"default.",
		Example: `  agentctl tune claude             # pick interactively
  agentctl tune codex --yes        # apply the recommended set
  agentctl tune instructions       # shared behavior for every agent
  agentctl tune git                # merge on pull, clearer conflicts and diffs
  agentctl tune claude --reset     # undo every curated setting`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: append(app.AgentNames(), tune.Instructions, tune.Git),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := newUI(cmd)
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
			want := tune.Recommended(statuses)
			switch {
			case reset:
				want = nil
			case u.interactive() && !yes:
				if want, err = pickTune(a.Title, statuses, want); err != nil {
					return err
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
	return cmd
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
		u.println(fmt.Sprintf("%s %s%s", mark, st.Title, tag))
		u.println(u.s.Subtle.Render("    " + st.Why))
		for _, n := range st.Notes {
			u.warn(n)
		}
	}
}
