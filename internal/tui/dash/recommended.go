package dash

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/tui/skin"
	"github.com/tnfssc/agentctl/internal/tune"
)

// The recommended setup: every curated setting for every target in one
// reviewed diff, like `agentctl tune all`. Risky items get their own yes.
// Applying it ends with a short burst of confetti.

func (m Model) recommendedEntry() navEntry {
	detail := "everything on"
	if m.recLeft > 0 {
		detail = fmt.Sprintf("%d to apply", m.recLeft)
	}
	return navEntry{id: "recommended", section: "Settings", icon: "★", label: "Recommended", detail: detail}
}

func (m Model) recommendedDetail(p skin.Palette) detailView {
	d := detailView{
		label:   "Settings · recommended",
		heading: "The recommended setup",
		body: "Every curated setting for Claude Code and Codex, the shared instructions and git, and the web " +
			"search and MCP tools, in one reviewed diff: how agentctl's author runs agents. Risky ones ask first.",
		buttons: [3]button{{"Apply", "rec-apply", true}, {"Wizard", "guide:setup", false}},
	}
	if m.recLeft == 0 {
		d.heading = "Everything is on"
	}
	d.facts[0] = fact(p, "covers", "Claude Code, Codex, instructions, git, tools")
	d.facts[1] = fact(p, "left", fmt.Sprintf("%d items", m.recLeft))
	return d
}

// applyRecommended plans the whole set, asking about risky items first.
func (m Model) applyRecommended() (tea.Model, tea.Cmd) {
	risky, err := tune.RiskyAll()
	if err != nil {
		return m.close(err.Error(), true), nil
	}
	if len(risky) > 0 {
		return m.askRisky(risky, planAll(true), planAll(false), noDialog), nil
	}
	m.busy = "Planning"
	return m, planAll(false)
}

// askRisky asks before turning on risky items: yes plans with them, Leave
// off plans without them, Cancel goes back.
func (m Model) askRisky(risky []tune.Status, with, without tea.Cmd, back dialog) Model {
	var lines []string
	for _, st := range risky {
		lines = append(lines, st.Title+": "+st.Why)
	}
	heading := "Turn on the risky items too?"
	if len(risky) == 1 {
		heading = "Turn on “" + risky[0].Title + "”?"
	}
	m.svc = ""
	m.ask = askSpec{
		title: "Risky settings", heading: heading,
		body:   strings.Join(lines, " ") + " " + risky[0].Cost + ".",
		button: "Turn on", run: with, busy: "Planning",
		no: "Leave off", noRun: without, back: back,
	}
	m.dialog = askDialog
	return m
}
