package dash

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/guide"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// Guides for setups agentctl leaves to the user's coding agent (`agentctl
// guide <topic>`): the dialog shows the command to paste into an agent, and
// once the agent recorded a setup, the command that undoes it.

// guideText is how the dashboard introduces a guide.
type guideText struct{ label, heading, body string }

var guideTexts = map[string]guideText{
	"signing": {
		"Git · signing", "Set up commit signing with your agent",
		"agentctl doesn't manage signing keys. Paste this into your coding agent; it walks you through the key, " +
			"git and GitHub, and asks before changing anything.",
	},
	"memory": {
		"Memory limits", "Cap memory for agents and Docker",
		"On Linux, every agent shares one cap and Docker another, below the RAM, and T3 Code's server gets a " +
			"guarantee; macOS can only cap Docker. Paste this into your coding agent; it asks before changing anything.",
	},
	"tunnel": {
		"Internet access", "Reach T3 Code and OpenCode from anywhere",
		"A Cloudflare Tunnel to hostnames on your own domain, with no ports opened. Paste this into your coding " +
			"agent; it walks you through cloudflared, the domain and optional Cloudflare Access, and asks before " +
			"changing anything.",
	},
}

func (m Model) openGuide(topic string) Model {
	m.guide, m.dialog = topic, guideDialog
	return m
}

// memoryState is the sidebar's word for the memory guide.
func (m Model) memoryState() string {
	if m.setUp("memory") {
		return "limits set up"
	}
	return "no limits"
}

// setUp reports whether a guide's setup is on record.
func (m Model) setUp(topic string) bool { return slices.Contains(m.applied, topic) }

// memoryDetail is the Behavior item for the memory guide: it applies to
// every agent on the machine, not to one server.
func (m Model) memoryDetail(p skin.Palette) detailView {
	d := detailView{
		label:   "Settings · this machine",
		heading: "Memory limits for agents",
		body: "One shared cap for every agent, however it starts (T3 Code, OpenCode, Claude Code, Codex, Pi), " +
			"and one for Docker, so a full machine slows tests down instead of freezing. Your coding agent sets it up.",
		buttons: [3]button{{"Guide", "guide:memory", true}},
	}
	d.facts[0] = fact(p, "state", "not set up")
	if m.setUp("memory") {
		dir, _ := guide.RecordsDir()
		d.facts[0] = fact(p, "state", "set up · undo with "+guide.Command(guide.Undo("memory")))
		d.facts[1] = fact(p, "record", tildePath(dir)+"/memory.md")
	}
	d.facts[2] = fact(p, "works on", "Linux; macOS caps Docker only")
	return d
}

// copyGuide puts a guide's command on the clipboard; there is none over SSH.
func (m Model) copyGuide(topic string) (tea.Model, tea.Cmd) {
	if m.remote || topic == "" {
		return m, nil
	}
	m.toast, m.toastErr = "Copied; paste it into your coding agent", false
	return m, tea.SetClipboard(guide.Command(topic))
}

// guideUndo is the open guide's undo topic once its setup is on record.
func (m Model) guideUndo() string {
	if !m.setUp(m.guide) {
		return ""
	}
	return guide.Undo(m.guide)
}

func (m Model) drawGuide(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	t := guideTexts[m.guide]
	undo := m.guideUndo()
	var extras []button
	if !m.remote {
		extras = append(extras, button{"Copy command", "m:copy-guide", false})
		if undo != "" {
			extras = append(extras, button{"Copy undo", "m:copy-undo", false})
		}
	}
	m.panel(sc, t.label, t.heading, extras, nil, &button{"Done", "m:close-btn", true})
	// Body, the command, then the state; the command wins on short dialogs.
	bodyRows := min(3, max(1, b.H-4))
	for i, l := range skin.Clip(t.body, b.W, bodyRows) {
		sc.add(skin.Cell(skin.Fg(p.Muted).Render(l), b.W), b.X, b.Y+i, 53, "")
	}
	cmdY := b.Y + bodyRows + 1
	sc.add(skin.Cell(skin.Fg(p.Accent).Bold(true).Render(guide.Command(m.guide)), b.W), b.X, cmdY, 53, "m:command")
	var status string
	switch {
	case m.guide == "signing" && m.signed:
		status = "Commits are already signed with " + m.signingKey + "."
	case m.guide == "signing":
		status = "Commits are not signed yet."
	case undo != "":
		status = "Set up on this machine. To undo it, paste this instead:"
	default:
		status = "Not set up on this machine yet."
	}
	rows := []string{skin.Fg(p.Dim).Render(status)}
	if undo != "" {
		rows = append(rows, skin.Fg(p.Accent).Bold(true).Render(guide.Command(undo)))
	}
	for i, r := range rows {
		if y := cmdY + 2 + i; y < b.Y+b.H {
			sc.add(skin.Cell(r, b.W), b.X, y, 53, "")
		}
	}
}
