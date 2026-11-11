package dash

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// The home list: four groups (agents, servers, proxies, settings) of
// two-line items under section labels, with a blank row between groups. It
// is the sidebar in the wide layout and the whole home page in the compact
// one. It scrolls inside a window whose size comes from the terminal alone,
// with one row kept below for "more" hints. A status dot marks only items
// with a live state (servers, proxies, agentctl's updates).

// navEntry is one item in the home list.
type navEntry struct {
	id, section, icon, label, detail string
	dot                              color.Color // the icon's color, nil for the default
}

// navEntries lists the home items in keyboard order (see homeIDs).
func (m Model) navEntries(p skin.Palette) []navEntry {
	var out []navEntry
	for _, as := range m.statuses {
		// The route only; the model is on the agent's page.
		detail := "own login"
		if as.status.Route != agent.Local {
			detail = "→ " + as.status.Route
		}
		out = append(out, navEntry{id: "agent:" + as.agent.Name, section: "Agents", icon: "◆", label: as.agent.Title, detail: detail})
	}
	out = append(out,
		serverEntry(p, "t3", "T3 Code", m.t3State(), m.t3State()),
		serverEntry(p, "opencode", "OpenCode web", m.ocState(), m.ocNavDetail()),
	)
	for _, n := range m.cfg.Names() {
		out = append(out, navEntry{id: "proxy:" + n, section: "Proxies", icon: "●", label: n, detail: m.proxyState(n), dot: m.proxyDot(p, n)})
	}
	return append(out, addEntry,
		navEntry{id: "behavior", section: "Settings", icon: "◇", label: "Instructions", detail: "all agents"},
		navEntry{id: "git", section: "Settings", icon: "◇", label: "Git", detail: m.signingState()},
		navEntry{id: "memory", section: "Settings", icon: "◇", label: "Memory", detail: m.memoryState()},
		m.selfEntry(p),
	)
}

// serverEntry shows a server's state at a glance, like a proxy's: green
// when it runs, amber when it runs but wants something or is busy, red on
// error, hollow when it isn't running.
func serverEntry(p skin.Palette, id, label, state, detail string) navEntry {
	e := navEntry{id: id, section: "Servers", icon: "●", label: label, detail: detail}
	switch state {
	case "running":
		e.dot = p.Green
	case "update", "restart", "checking", "starting":
		e.dot = p.Amber
	case "error":
		e.dot = p.Red
	default:
		e.icon, e.dot = "○", p.Dim
	}
	return e
}

// addEntry ends the proxies.
var addEntry = navEntry{id: "add", section: "Proxies", icon: "+", label: "Add a proxy", detail: "a URL and a key"}

// proxyState is a proxy's one-line health for the home list.
func (m Model) proxyState(name string) string {
	r, ok := m.probes[name]
	switch {
	case !ok:
		return "checking"
	case r.HealthErr != nil:
		return "down"
	case r.ModelsErr != nil:
		return "key rejected"
	}
	return fmt.Sprintf("live · %d models", len(r.Models))
}

// navLine is one row of the list: a section label, the blank before it, or
// one of the two lines of an item.
type navLine struct {
	entry  int // -1 for labels and blanks
	part   int // 0 or 1 for items
	header string
}

func navLines(entries []navEntry) []navLine {
	var lines []navLine
	for i, e := range entries {
		if i == 0 || e.section != entries[i-1].section {
			if i > 0 {
				lines = append(lines, navLine{entry: -1})
			}
			lines = append(lines, navLine{entry: -1, header: e.section})
		}
		lines = append(lines, navLine{entry: i}, navLine{entry: i, part: 1})
	}
	return lines
}

// navArea is where the list goes: the scrolling window, then a "more" row.
func (m Model) navArea() rect {
	if m.compact() {
		c := m.content()
		return rect{0, c.Y, m.width, c.H}
	}
	top := m.bodyTop()
	return rect{2, top, m.sidebarWidth() - 3, m.height - 3 - top + 1}
}

// keepNavVisible scrolls the list just enough to show the focused item,
// with its section label when that fits. It runs after every update so the
// window only moves when the focus leaves it.
func (m *Model) keepNavVisible() {
	if !m.loaded || m.width == 0 {
		return
	}
	lines := navLines(m.navEntries(m.pal))
	h := m.navWindow()
	start := 0
	for i, l := range lines {
		if l.entry == m.focus && l.part == 0 {
			start = i
			break
		}
	}
	if start > 0 && lines[start-1].header != "" {
		start--
	}
	end := start + 2
	if lines[start].header != "" {
		end++
	}
	switch {
	case start < m.navTop:
		m.navTop = start
	case end > m.navTop+h:
		m.navTop = end - h
	}
	m.navTop = max(0, min(m.navTop, len(lines)-h))
}

// showMore moves the focus to the first item below the window, or back to
// the top when nothing is below: tapping "more" pages through the list on
// terminals that don't turn swipes into scrolling.
func (m *Model) showMore() {
	lines := navLines(m.navEntries(m.pal))
	for i := m.navTop + m.navWindow() - 1; i < len(lines); i++ {
		if lines[i].entry >= 0 && lines[i].part == 0 {
			m.focus = lines[i].entry
			return
		}
	}
	m.focus = 0
}

// navWindow is how many list rows show at once: the area less the more row.
func (m Model) navWindow() int { return max(2, m.navArea().H-1) }

func (m Model) drawNav(sc *scene, p skin.Palette) {
	a := m.navArea()
	if !m.loaded {
		sc.add(skin.Fg(p.Dim).Render(skin.Spin(m.frame)+" loading"), a.X+1, a.Y, 1, "")
		return
	}
	entries := m.navEntries(p)
	lines := navLines(entries)
	ids := m.homeIDs()
	selected := ids[min(m.focus, len(ids)-1)]
	mark := ""
	if m.compact() {
		mark = "›"
	}
	h := m.navWindow()
	for r := range h {
		i := m.navTop + r
		if i >= len(lines) {
			break
		}
		l := lines[i]
		switch {
		case l.header != "":
			sc.add(p.Label(l.header), a.X+1, a.Y+r, 1, "")
		case l.entry >= 0 && l.part == 0 && r+1 < h:
			e := entries[l.entry]
			sc.add(p.TallDot(e.icon, e.dot, e.label, e.detail, mark, a.W, e.id == selected, m.hover == "nav:"+e.id), a.X, a.Y+r, 2, "nav:"+e.id)
		}
	}
	// Items that don't fit whole, including one cut by the window's edge.
	var below int
	for i := m.navTop + h - 1; i < len(lines); i++ {
		if lines[i].entry >= 0 && lines[i].part == 0 {
			below++
		}
	}
	var more []string
	if m.navTop > 0 {
		more = append(more, "↑")
	}
	if below > 0 {
		more = append(more, fmt.Sprintf("↓ %d more", below))
	}
	sc.add(skin.Cell(skin.Fg(p.Dim).Render(" "+strings.Join(more, " ")), a.W), a.X, a.Y+h, 1, "more")
}
