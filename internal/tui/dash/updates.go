package dash

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/selfupdate"
	"github.com/tnfssc/agentctl/internal/tui/skin"
	"github.com/tnfssc/agentctl/internal/upgrade"
)

// Updates for agentctl, the agents' CLIs and the servers, like `agentctl
// update`: each agent's and server's page has its own Update button, and
// Settings › Updates lists them all with Update all. Each tool updates
// through its own updater, one after another, the footer counting along.

type (
	selfCheckedMsg struct {
		latest string
		err    error
	}
	clisMsg    struct{ list []upgrade.Status }
	updStepMsg struct {
		name, title string
		err         error
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

func inspectCLIs(ctx context.Context) tea.Cmd {
	return func() tea.Msg { return clisMsg{upgrade.InspectAll(ctx)} }
}

func runUpdate(ctx context.Context, u app.Update) tea.Cmd {
	return func() tea.Msg { return updStepMsg{name: u.Name, title: u.Title, err: u.Run(ctx, io.Discard)} }
}

// updates is what the dashboard knows about every tool, in update order.
// Tools still being checked are left out.
func (m Model) updates() []app.Update {
	var out []app.Update
	for _, s := range m.clis {
		out = append(out, app.CLIUpdate(s))
	}
	if m.ocLoaded {
		out = append(out, app.OpenCodeUpdate(m.oc))
	}
	if m.t3Loaded {
		out = append(out, app.T3Update(m.t3))
	}
	if m.release() && m.selfChecked {
		u := app.SelfUpdate(m.version, m.selfLatest, m.selfErr)
		u.Worth = u.Worth && m.selfUpdated == ""
		out = append(out, u)
	}
	slices.SortStableFunc(out, func(a, b app.Update) int {
		return slices.Index(app.UpdateNames, a.Name) - slices.Index(app.UpdateNames, b.Name)
	})
	return out
}

func (m Model) updateFor(name string) (app.Update, bool) {
	i := slices.IndexFunc(m.updates(), func(u app.Update) bool { return u.Name == name })
	if i < 0 {
		return app.Update{}, false
	}
	return m.updates()[i], true
}

// checking reports whether any tool's newest version is still unknown.
func (m Model) checking() bool {
	return !m.clisLoaded || !m.ocLoaded || !m.t3Loaded || (m.release() && !m.selfChecked)
}

// news counts tools with a newer release.
func (m Model) news() int {
	n := 0
	for _, u := range m.updates() {
		if u.Worth && u.Latest != "" && u.Latest != u.Current {
			n++
		}
	}
	return n
}

func versionLine(u app.Update) string {
	switch {
	case !u.Installed:
		return "not installed"
	case u.Worth && u.Latest != "" && u.Latest != u.Current:
		return u.Current + " → " + u.Latest
	case u.Err != nil:
		return u.Current + " · can't check"
	}
	return u.Current + " · newest"
}

// updateButton is an agent's or server's Update button, empty when there is
// nothing to update.
func (m Model) updateButton(name string) button {
	if u, ok := m.updateFor(name); ok && u.Installed && u.Worth {
		return button{"Update", "upd:" + name, false}
	}
	return button{}
}

func (m Model) onUpdates(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case selfCheckedMsg:
		m.selfLatest, m.selfErr, m.selfChecked = msg.latest, msg.err, true
	case clisMsg:
		m.clis, m.clisLoaded = msg.list, true
	case updStepMsg:
		return m.onUpdStep(msg)
	}
	m.checkingAgain = m.checkingAgain && m.checking()
	return m, nil
}

func (m Model) onUpdStep(msg updStepMsg) (Model, tea.Cmd) {
	{
		if msg.err != nil {
			m.updFailed = append(m.updFailed, msg.title+": "+msg.err.Error())
		} else {
			m.updDone = append(m.updDone, msg.title)
			if msg.name == "agentctl" {
				m.selfUpdated = m.selfLatest
			}
		}
		m.updQueue = m.updQueue[1:]
		if len(m.updQueue) > 0 {
			next, _ := m.updateFor(m.updQueue[0])
			total := len(m.updDone) + len(m.updFailed) + len(m.updQueue)
			m.busy = fmt.Sprintf("Updating %s (%d of %d)", next.Title, total-len(m.updQueue)+1, total)
			return m, runUpdate(m.ctx, next)
		}
		m.busy = ""
		toast, isErr := "Updated "+strings.Join(m.updDone, ", ")+". Restart running sessions to use the new versions.", false
		if len(m.updFailed) > 0 {
			toast, isErr = strings.Join(m.updFailed, "; "), true
		}
		m.clisLoaded = false
		cmds := []tea.Cmd{inspectCLIs(m.ctx), inspectOC(m.ctx), inspectT3(m.ctx)}
		return m.close(toast, isErr), tea.Batch(cmds...)
	}
}

// askUpdate asks before updating the named tools, then runs them in order.
func (m Model) askUpdate(names []string) Model {
	var lines []string
	for _, n := range names {
		u, _ := m.updateFor(n)
		line := u.Title
		if u.Latest != "" && u.Latest != u.Current {
			line += " " + u.Current + " → " + u.Latest
		}
		if u.Note != "" {
			line += " (" + u.Note + ")"
		}
		if n == "agentctl" {
			line += " (reopen agentctl to use it)"
		}
		lines = append(lines, line)
	}
	first, _ := m.updateFor(names[0])
	heading := "Update " + first.Title + "?"
	if len(names) > 1 {
		heading = fmt.Sprintf("Update %d tools?", len(names))
	}
	m.updQueue, m.updDone, m.updFailed = names, nil, nil
	busy := "Updating " + first.Title
	if len(names) > 1 {
		busy = fmt.Sprintf("Updating %s (1 of %d)", first.Title, len(names))
	}
	m.svc = ""
	m.ask = askSpec{
		title: "Updates", heading: heading,
		body:   strings.Join(lines, " · ") + ". Running sessions keep the old versions until restarted.",
		button: "Update", busy: busy, run: runUpdate(m.ctx, first),
	}
	m.dialog = askDialog
	return m
}

// actUpdates handles the Update, Update all and Check again buttons.
func (m Model) actUpdates(verb, arg string) (tea.Model, tea.Cmd) {
	switch verb {
	case "upd":
		if u, ok := m.updateFor(arg); ok && u.Worth {
			return m.askUpdate([]string{arg}), nil
		}
	case "upd-all":
		var names []string
		for _, u := range m.updates() {
			if u.Installed && u.Worth {
				names = append(names, u.Name)
			}
		}
		if len(names) > 0 {
			return m.askUpdate(names), nil
		}
	case "upd-check":
		m.clisLoaded, m.checkingAgain = false, true
		cmds := []tea.Cmd{inspectCLIs(m.ctx), inspectOC(m.ctx), inspectT3(m.ctx)}
		if m.release() {
			m.selfChecked, m.selfErr = false, nil
			cmds = append(cmds, checkSelf(m.ctx))
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) updatesEntry(p skin.Palette) navEntry {
	e := navEntry{id: "updates", section: "Settings", icon: "●", label: "Updates"}
	switch n := m.news(); {
	case m.checking() && n == 0:
		e.detail, e.icon, e.dot = "checking", "○", p.Dim
	case n == 1:
		e.detail, e.dot = "1 update", p.Amber
	case n > 1:
		e.detail, e.dot = fmt.Sprintf("%d updates", n), p.Amber
	default:
		e.detail, e.dot = "all newest", p.Green
	}
	return e
}

func (m Model) updatesDetail(p skin.Palette) detailView {
	d := detailView{
		label: "Settings · updates",
		body: "Each tool updates through its own updater: agentctl, Claude Code, Codex, OpenCode, Pi with its " +
			"extensions, and T3 Code. Servers restart on the new version, stopping running turns.",
	}
	// Both buttons keep their slots whatever the state.
	check := button{"Check again", "upd-check", false}
	switch n := m.news(); {
	case n > 0:
		d.heading = fmt.Sprintf("%d updates", n)
		if n == 1 {
			d.heading = "1 update"
		}
	case m.checking():
		d.heading = "Checking for updates…"
	default:
		d.heading = "Everything is up to date"
	}
	for _, u := range m.updates() {
		if u.Installed && u.Worth {
			d.buttons[0] = button{"Update all", "upd-all", true}
			break
		}
	}
	check.primary = d.buttons[0].id == ""
	d.buttons[1] = check
	// Tools with news first; one row each, the last row counting the rest.
	list := m.updates()
	slices.SortStableFunc(list, func(a, b app.Update) int {
		return boolInt(!a.Worth || !a.Installed) - boolInt(!b.Worth || !b.Installed)
	})
	for i, u := range list {
		if i == factRows-1 && len(list) > factRows {
			d.facts[i] = skin.Fg(p.Dim).Render(fmt.Sprintf("and %d more", len(list)-i))
			break
		}
		d.facts[i] = skin.Fg(p.Dim).Render(fmt.Sprintf("%-9s", u.Name)) + skin.Fg(p.Muted).Render(versionLine(u))
	}
	return d
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
