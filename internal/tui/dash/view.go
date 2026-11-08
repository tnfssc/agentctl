package dash

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/models"
)

// View renders the current screen with a header and a footer of key hints.
func (m Model) View() tea.View {
	s := m.styles
	header := s.Title.Render("agentctl") + s.Subtle.Render("  route coding agents to their own login or a proxy")

	var body, hints string
	switch m.screen {
	case homeScreen:
		body = m.homeView()
		hints = "↑/↓ move · enter open · a add proxy · t test · r refresh Codex · d remove · q quit"
	case routeScreen:
		body = s.Text.Bold(true).Render("Route "+title(m.req.Agent)+" to") + "\n\n" + m.listView(m.height-10)
		hints = "↑/↓ move · enter choose · esc back"
	case modelScreen:
		note := "Claude models are only offered to Claude Code."
		if m.req.Agent == claude.Name {
			note = "Any model on the proxy works in Claude Code."
		}
		body = s.Text.Bold(true).Render("Model on "+m.req.Route) + "  " + s.Subtle.Render(note) + "\n\n" + m.listView(m.height-10)
		hints = "↑/↓ move · enter choose · esc back"
	case ackScreen:
		body = s.Warning.Render(lipgloss.Wrap(claude.PolicyWarning, m.contentWidth(), "")) + "\n\n" +
			s.Text.Render("Route Claude Code through "+m.req.Route+" anyway? You will not be asked again.")
		hints = "y continue · n cancel"
	case confirmScreen:
		body = s.Text.Bold(true).Render("Apply to "+title(m.plan.Agent)+"?") + s.Subtle.Render("  backups go to ~/.config/agentctl/backups") + "\n\n" + m.vp.View()
		hints = "y apply · n cancel · ↑/↓ scroll"
	case addScreen:
		body = s.Text.Bold(true).Render("Add a proxy") + "\n\n" + m.form.View()
		hints = "enter next · esc cancel"
	case proxyScreen:
		body = m.vp.View()
		hints = "t test again · d remove · ↑/↓ scroll · esc back"
	case removeScreen:
		if who := m.usedBy(m.proxy); who != "" {
			body = s.Warning.Render(fmt.Sprintf("%s is routed to %s. Switch it to another route first.", who, m.proxy))
			hints = "any key back"
		} else {
			body = s.Text.Render("Remove proxy " + m.proxy + "? Its key is deleted from agentctl's config.")
			hints = "y remove · n cancel"
		}
	}

	footer := ""
	switch {
	case m.busy != "":
		footer = m.spinner.View() + " " + s.Accent.Render(m.busy)
	case m.toast != "" && m.toastErr:
		footer = s.Error.Render("✗ " + m.toast)
	case m.toast != "":
		footer = s.Success.Render("✓ " + m.toast)
	}
	footer = lipgloss.Wrap(footer, m.contentWidth(), "")

	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer, s.Subtle.Render(lipgloss.Wrap(hints, m.contentWidth(), "")))
	// Clip rather than wrap: a wrapped row reads worse than a truncated one.
	content = lipgloss.NewStyle().MaxWidth(m.contentWidth()).Render(content)
	if m.height > 2 {
		content = lipgloss.NewStyle().MaxHeight(m.height - 2).Render(content)
	}
	v := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	v.AltScreen = true
	v.WindowTitle = "agentctl"
	return v
}

func (m Model) homeView() string {
	s := m.styles
	if !m.loaded {
		return m.spinner.View() + " loading"
	}
	var b strings.Builder
	row := 0
	cursor := func() string {
		defer func() { row++ }()
		if row == m.cursor {
			return s.Accent.Render("› ")
		}
		return "  "
	}

	b.WriteString(s.Subtle.Render("AGENTS") + "\n")
	for _, as := range m.statuses {
		c := cursor()
		name := s.Text.Bold(true).Width(13).Render(as.agent.Title)
		if as.err != nil {
			b.WriteString(c + name + s.Error.Render(as.err.Error()) + "\n")
			continue
		}
		route := s.Success.Render("own login")
		if as.status.Route != agent.Local {
			route = s.Accent.Render("proxy " + as.status.Route)
		}
		detail := ""
		if as.status.Model != "" {
			detail = s.Subtle.Render("  " + as.status.Model)
		}
		b.WriteString(c + name + route + detail + "\n")
		for _, n := range as.status.Notes {
			b.WriteString("    " + s.Warning.Render("! "+n) + "\n")
		}
	}

	b.WriteString("\n" + s.Subtle.Render("PROXIES") + "\n")
	names := m.cfg.Names()
	width := 0
	for _, n := range names {
		width = max(width, len(n))
	}
	for _, n := range names {
		c := cursor()
		line := c + s.Text.Bold(true).Render(fmt.Sprintf("%-*s", width, n)) + "  " + s.Subtle.Render(m.cfg.Proxies[n].URL) + "  " + m.badge(n)
		b.WriteString(ansi.Truncate(line, m.contentWidth(), "…") + "\n")
	}
	b.WriteString(cursor() + s.Accent.Render("+ Add a proxy"))
	return b.String()
}

// badge summarizes a proxy's last check in one short string.
func (m Model) badge(name string) string {
	s := m.styles
	r, ok := m.probes[name]
	switch {
	case !ok:
		return m.spinner.View() + s.Subtle.Render(" checking")
	case r.HealthErr != nil:
		return s.Error.Render("✗ unreachable")
	case r.ModelsErr != nil:
		return s.Error.Render("✗ " + r.ModelsErr.Error())
	}
	text := s.Success.Render(fmt.Sprintf("✓ %d models · %s", len(r.Models), r.Latency.Round(time.Millisecond)))
	if len(r.Disguised) > 0 || r.DiscoveryErr != nil || r.SlowDiscovery() {
		text += s.Warning.Render("  ! see details")
	}
	return text
}

// listView renders m.options with the selected row marked, scrolled so the
// selection stays within height rows.
func (m Model) listView(height int) string {
	s := m.styles
	height = max(3, height)
	start := 0
	if m.pick >= height {
		start = m.pick - height + 1
	}
	end := min(len(m.options), start+height)
	labelWidth := 0
	for _, o := range m.options[start:end] {
		labelWidth = max(labelWidth, lipgloss.Width(o.label))
	}
	var lines []string
	for i := start; i < end; i++ {
		o := m.options[i]
		mark, label := "  ", s.Text.Render(fmt.Sprintf("%-*s", labelWidth, o.label))
		if i == m.pick {
			mark, label = s.Accent.Render("› "), s.Accent.Bold(true).Render(fmt.Sprintf("%-*s", labelWidth, o.label))
		}
		lines = append(lines, ansi.Truncate(mark+label+"  "+s.Subtle.Render(o.detail), m.contentWidth(), "…"))
	}
	if end < len(m.options) {
		lines = append(lines, s.Subtle.Render(fmt.Sprintf("  … %d more", len(m.options)-end)))
	}
	return strings.Join(lines, "\n")
}

// proxyReport is the detail page for one proxy: its check and its models.
func (m Model) proxyReport(name string) string {
	s := m.styles
	p := m.cfg.Proxies[name]
	var b strings.Builder
	b.WriteString(s.Text.Bold(true).Render(name) + "  " + s.Subtle.Render(p.URL) + "\n\n")
	r, ok := m.probes[name]
	switch {
	case !ok:
		b.WriteString(m.spinner.View() + " checking…")
		return b.String()
	case r.HealthErr != nil:
		b.WriteString(s.Error.Render("✗ unreachable: " + r.HealthErr.Error()))
		return b.String()
	case r.ModelsErr != nil:
		b.WriteString(s.Error.Render("✗ " + r.ModelsErr.Error()))
		return b.String()
	}
	b.WriteString(s.Success.Render(fmt.Sprintf("✓ healthy (%s), key accepted", r.Latency.Round(time.Millisecond))) + "\n")
	warn := func(text string) {
		b.WriteString(s.Warning.Render(lipgloss.Wrap("! "+text, m.contentWidth(), "")) + "\n")
	}
	switch {
	case r.DiscoveryErr != nil:
		warn("Claude Code's /model list will not show proxy models: " + r.DiscoveryErr.Error())
	case r.SlowDiscovery():
		warn("listing models takes over 3s; Claude Code will show only its built-in models")
	case len(r.Disguised) > 0:
		warn(fmt.Sprintf("Claude Code sees %d non-Claude models under disguised names (e.g. %s); "+
			"set disable-cloaking-model-list on the proxy to show real names", len(r.Disguised), r.Disguised[0]))
	default:
		b.WriteString(s.Success.Render("✓ Claude Code sees the proxy's models under their real names") + "\n")
	}
	b.WriteString("\n" + s.Subtle.Render(fmt.Sprintf("MODELS  %d Claude · %d all agents · %d unverified", r.Counts.Claude, r.Counts.Other, r.Counts.Unknown)) + "\n")
	width := 0
	for _, pm := range r.Models {
		width = max(width, len(pm.ID))
	}
	for _, pm := range r.Models {
		var who string
		switch models.Classify(pm.ID, pm.OwnedBy, p.AllowModels) {
		case models.Anthropic:
			who = s.Accent.Render("Claude Code only")
		case models.Other:
			who = s.Success.Render("all agents")
		default:
			who = s.Warning.Render("Claude Code only (unverified; `agentctl proxy allow " + name + " " + pm.ID + "`)")
		}
		b.WriteString(fmt.Sprintf("  %-*s  ", width, pm.ID) + s.Subtle.Render(fmt.Sprintf("%-12s ", pm.OwnedBy)) + who + "\n")
	}
	return b.String()
}
