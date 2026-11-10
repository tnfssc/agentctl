package dash

import (
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
		buttons: [3]button{{"Tune", "tune:" + tune.Git, true}, {"Signing", "guide:signing", false}},
	}
	d.facts[0] = fact(p, "file", tildePath(path))
	if m.signed {
		d.facts[1] = fact(p, "signing", "on, key "+m.signingKey)
	} else {
		d.facts[1] = fact(p, "signing", "off")
	}
	return d
}
