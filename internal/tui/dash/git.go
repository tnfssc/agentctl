package dash

import (
	"github.com/tnfssc/agentctl/internal/guide"
	"github.com/tnfssc/agentctl/internal/tui/skin"
	"github.com/tnfssc/agentctl/internal/tune"
)

// signingState is the sidebar's word for commit signing.
func (m Model) signingState() string {
	if m.signed {
		return "signed"
	}
	return "unsigned"
}

func (m Model) gitDetail(p skin.Palette) detailView {
	path, _ := tune.GitConfigPath()
	d := detailView{
		label:   "Behavior · all agents",
		heading: "Git for agents",
		body:    "Global git settings that keep agents' pulls, pushes and conflicts smooth. Signing keys stay yours.",
		buttons: [3]button{{"Tune", "tune:" + tune.Git, true}, {"Signing", "signing", false}},
	}
	d.facts[0] = fact(p, "file", tildePath(path))
	if m.signed {
		d.facts[1] = fact(p, "signing", "on, key "+m.signingKey)
	} else {
		d.facts[1] = fact(p, "signing", "off")
	}
	return d
}

func (m Model) drawSigning(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	var extras []button
	if !m.remote {
		extras = []button{{"Copy command", "m:copy-guide", false}}
	}
	m.panel(sc, "Git · signing", "Set up commit signing with your agent", extras, nil, &button{"Done", "m:close-btn", true})
	lines := skin.Clip("agentctl doesn't manage signing keys. Paste this into your coding agent; it walks you through "+
		"the key, git and GitHub, and asks before changing anything.", b.W, 3)
	for i, l := range lines {
		sc.add(skin.Cell(skin.Fg(p.Muted).Render(l), b.W), b.X, b.Y+i, 53, "")
	}
	sc.add(skin.Cell(skin.Fg(p.Accent).Bold(true).Render(guide.Command("signing")), b.W), b.X, b.Y+4, 53, "m:command")
	status := "Commits are not signed yet."
	if m.signed {
		status = "Commits are already signed with " + m.signingKey + "."
	}
	sc.add(skin.Cell(skin.Fg(p.Dim).Render(status), b.W), b.X, b.Y+6, 53, "")
}
