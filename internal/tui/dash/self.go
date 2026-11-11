package dash

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/selfupdate"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// agentctl's own updates, like `agentctl update`: a row under its own
// section, a detail pane with the version and the latest release, and the
// shared ask dialog before replacing the binary.

type (
	selfCheckedMsg struct {
		latest string
		err    error
	}
	selfUpdatedMsg struct {
		tag, path string
		err       error
	}
)

// release reports whether this build is a release, the only kind that
// updates itself; a source build would be silently replaced.
func (m Model) release() bool { return strings.HasPrefix(m.version, "v") }

func checkSelf(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		latest, err := selfupdate.Latest(ctx)
		return selfCheckedMsg{latest: latest, err: err}
	}
}

func installSelf(ctx context.Context, tag string) tea.Cmd {
	return func() tea.Msg {
		path, err := selfupdate.Install(ctx, tag)
		return selfUpdatedMsg{tag: tag, path: path, err: err}
	}
}

func (m Model) onSelf(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case selfCheckedMsg:
		m.selfLatest, m.selfErr, m.selfChecked = msg.latest, msg.err, true
	case selfUpdatedMsg:
		m.busy = ""
		if msg.err != nil {
			return m.close(msg.err.Error(), true), nil
		}
		m.selfUpdated = msg.tag
		return m.close("Updated "+tildePath(msg.path)+" to "+msg.tag+". Quit and reopen agentctl to use it.", false), nil
	}
	return m, nil
}

// selfState is the sidebar's word for agentctl's updates.
func (m Model) selfState() string {
	switch {
	case !m.release():
		return "source build"
	case m.selfUpdated != "":
		return "reopen"
	case !m.selfChecked:
		return "checking"
	case m.selfErr != nil:
		return "can't check"
	case selfupdate.Newer(m.version, m.selfLatest):
		return "update"
	}
	return "latest"
}

func (m Model) selfEntry(p skin.Palette) navEntry {
	state := m.selfState()
	e := navEntry{id: "self", section: "Settings", icon: "●", label: "agentctl", detail: m.version + " · " + state}
	switch state {
	case "latest":
		e.dot = p.Green
	case "update", "reopen", "checking":
		e.dot = p.Amber
	case "can't check":
		e.dot = p.Red
	default:
		e.icon, e.dot = "○", p.Dim
	}
	return e
}

func (m Model) selfDetail(p skin.Palette) detailView {
	d := detailView{label: "Settings · agentctl"}
	// Check again keeps the second slot whatever the state, so it never moves.
	check := button{"Check again", "self-check", false}
	switch state := m.selfState(); state {
	case "source build":
		d.heading = "Built from source"
		d.body = "A source build doesn't update itself; a release would silently replace it. Rebuild it, or " +
			"install a release with the install script."
	case "reopen":
		d.heading = "Updated to " + m.selfUpdated
		d.body = "Quit and reopen agentctl to use it. Agents that call agentctl already run the new one."
	case "checking":
		d.heading = "Checking for updates…"
	case "can't check":
		d.heading = "Can't check for updates"
		d.body = m.selfErr.Error()
		check.primary = true
		d.buttons[1] = check
	case "update":
		d.heading = m.selfLatest + " is out"
		d.body = "Downloads the release for this machine, checks it against the release's SHA256SUMS and " +
			"replaces this agentctl in place, so agents that call it keep working."
		d.buttons = [3]button{{"Update", "self-update", true}, check}
	default:
		d.heading = "Up to date"
		d.body = "agentctl looks for a newer release on GitHub each time the dashboard opens."
		check.primary = true
		d.buttons[1] = check
	}
	d.facts[0] = fact(p, "version", m.version)
	if m.selfLatest != "" {
		d.facts[1] = fact(p, "latest", m.selfLatest)
	}
	if m.exe != "" {
		d.facts[2] = fact(p, "binary", tildePath(m.exe))
	}
	return d
}

// actSelf handles the update buttons on the home screen.
func (m Model) actSelf(verb string) (tea.Model, tea.Cmd) {
	switch verb {
	case "self-check":
		if !m.release() {
			return m, nil
		}
		m.selfChecked, m.selfErr = false, nil
		return m, checkSelf(m.ctx)
	case "self-update":
		m.ask = askSpec{
			title: "agentctl", heading: "Update agentctl " + m.version + " → " + m.selfLatest + "?",
			body:   "This dashboard and other running agentctl sessions keep the old version until reopened.",
			button: "Update", busy: "Updating agentctl", run: installSelf(m.ctx, m.selfLatest),
		}
		m.dialog = askDialog
	}
	return m, nil
}
