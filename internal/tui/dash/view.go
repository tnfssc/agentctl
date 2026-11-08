package dash

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

const (
	sidebarWidth = 30
	minWidth     = 72
	minHeight    = 22
)

// scene collects absolutely positioned layers; the compositor draws them by
// Z and answers "what is under the pointer" from the same layers.
type scene struct{ layers []*lipgloss.Layer }

func (s *scene) add(content string, x, y, z int, id string) {
	l := lipgloss.NewLayer(content).X(x).Y(y).Z(z)
	if id != "" {
		l = l.ID(id)
	}
	s.layers = append(s.layers, l)
}

// View renders the dashboard and resolves mouse events against this frame.
func (m Model) View() tea.View {
	sc := &scene{}
	sc.add(strings.Repeat(strings.Repeat(" ", max(1, m.width))+"\n", max(1, m.height)), 0, 0, 0, "")
	if m.width < minWidth || m.height < minHeight {
		sc.add(m.pal.Body(fmt.Sprintf("agentctl needs at least %d×%d; this terminal is %d×%d.", minWidth, minHeight, m.width, m.height), max(10, m.width-4)), 2, 1, 1, "")
	} else {
		m.drawPage(sc)
		if m.dialog != noDialog {
			m.drawDialog(sc)
		}
	}

	comp := lipgloss.NewCompositor(sc.layers...)
	content := comp.Render()
	if m.width > 0 && m.height > 0 {
		content = lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	// Hover needs every pointer movement; over SSH that floods the link, so
	// remote sessions get clicks and the wheel only.
	v.MouseMode = tea.MouseModeAllMotion
	if m.remote {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.WindowTitle = "agentctl"
	v.OnMouse = func(msg tea.MouseMsg) tea.Cmd {
		mouse := msg.Mouse()
		p := pointerMsg{id: comp.Hit(mouse.X, mouse.Y).ID()}
		switch msg.(type) {
		case tea.MouseClickMsg:
			if mouse.Button != tea.MouseLeft {
				return nil
			}
			p.kind = pointerClick
		case tea.MouseWheelMsg:
			p.kind, p.wheel = pointerWheel, 1
			if mouse.Button == tea.MouseWheelUp {
				p.wheel = -1
			}
		case tea.MouseMotionMsg:
			p.kind = pointerMove
		default:
			return nil
		}
		return func() tea.Msg { return p }
	}
	return v
}

type button struct {
	label, id string
	primary   bool
}

// buttons lays out a row of buttons from x and returns the x after it.
func (m Model) buttons(sc *scene, p skin.Palette, bs []button, x, y, z int) int {
	for _, b := range bs {
		btn := p.Button(b.label, b.primary, m.hover == b.id)
		sc.add(btn, x, y, z, b.id)
		x += lipgloss.Width(btn) + 1
	}
	return x
}

func (m Model) drawPage(sc *scene) {
	p := m.pal
	if m.dialog != noDialog {
		p = p.Faded()
	}
	w, h := m.width, m.height

	// Nav: wordmark, text links, status pill.
	sc.add(p.Wordmark(), 2, 2, 1, "")
	x := 20
	for _, l := range []struct{ label, id string }{
		{"Add proxy", "add"}, {"Test proxies", "testall"}, {"Refresh Codex", "refresh"}, {"Quit", "quit"},
	} {
		style := skin.Fg(p.Muted)
		if m.hover == l.id {
			style = skin.Fg(p.Text).Underline(true)
		}
		sc.add(style.Render(l.label), x, 2, 1, l.id)
		x += lipgloss.Width(l.label) + 4
	}
	pill := m.pill(p)
	if px := w - lipgloss.Width(pill) - 2; px > x {
		sc.add(pill, px, 1, 1, "")
	}
	sc.add(p.HLine(w), 0, 4, 1, "")

	// Body: sidebar, hairline, detail.
	top, bottom := 5, h-3
	sc.add(p.VLine(bottom-top+1), sidebarWidth, top, 1, "")
	m.drawSidebar(sc, p, top)
	m.drawDetail(sc, p, top)

	// Footer: status message on the left, key hints on the right.
	sc.add(p.HLine(w), 0, h-2, 1, "")
	var status string
	switch {
	case m.busy != "":
		status = skin.Fg(p.Accent).Render(skin.Spin(m.frame)+" ") + skin.Fg(p.Text).Render(m.busy+"…")
	case m.toast != "" && m.toastErr:
		status = skin.Fg(p.Red).Render("✗ ") + skin.Fg(p.Text).Render(m.toast)
	case m.toast != "":
		status = skin.Fg(p.Green).Render("✓ ") + skin.Fg(p.Text).Render(m.toast)
	}
	hints := skin.Fg(p.Dim).Render(m.hints())
	room := w - 4 - lipgloss.Width(hints) - 2
	if room < 20 {
		hints, room = "", w-4
	}
	sc.add(ansi.Truncate(status, room, "…"), 2, h-1, 1, "")
	if hints != "" {
		sc.add(hints, w-2-lipgloss.Width(hints), h-1, 1, "")
	}
}

func (m Model) hints() string {
	switch m.dialog {
	case noDialog:
		return "↑↓ select · enter open · click anything · q quit"
	case routeDialog, modelDialog:
		return "↑↓ · enter choose · esc back"
	case tuneDialog:
		return "↑↓ · space toggle · y review · x reset all · esc close"
	case confirmDialog:
		return "y apply · n cancel · scroll"
	case addDialog:
		return "tab next · enter submit · esc cancel"
	case proxyDialog:
		return "t test · d remove · esc close"
	default:
		return "y yes · n no"
	}
}

// pill summarizes proxy health like the site's "prod server / running".
func (m Model) pill(p skin.Palette) string {
	if !m.loaded {
		return p.Pill(p.Amber, "agentctl", "loading")
	}
	total := len(m.cfg.Proxies)
	if total == 0 {
		return p.Pill(p.Dim, "proxies", "none yet")
	}
	live, checking := 0, 0
	for _, n := range m.cfg.Names() {
		r, ok := m.probes[n]
		switch {
		case !ok:
			checking++
		case r.Err() == nil:
			live++
		}
	}
	noun := "proxies"
	if total == 1 {
		noun = "proxy"
	}
	switch {
	case checking > 0:
		return p.Pill(p.Amber, fmt.Sprintf("%d %s", total, noun), "checking")
	case live == total:
		return p.Pill(p.Green, fmt.Sprintf("%d %s", total, noun), "live")
	case live == 0:
		return p.Pill(p.Red, fmt.Sprintf("%d %s", total, noun), "down")
	default:
		return p.Pill(p.Amber, fmt.Sprintf("%d/%d %s", live, total, noun), "live")
	}
}

func (m Model) drawSidebar(sc *scene, p skin.Palette, top int) {
	if !m.loaded {
		sc.add(skin.Fg(p.Dim).Render(skin.Spin(m.frame)+" loading"), 2, top+1, 1, "")
		return
	}
	rw := sidebarWidth - 3
	ids := m.homeIDs()
	selected := ""
	if m.focus < len(ids) {
		selected = ids[m.focus]
	}
	y := top + 1
	row := func(id, icon, label, detail string) {
		sc.add(p.Row(icon, label, detail, rw, id == selected, m.hover == "nav:"+id), 2, y, 2, "nav:"+id)
		y++
	}
	sc.add(p.Label("Agents"), 3, y, 1, "")
	y += 2
	for _, as := range m.statuses {
		detail := "own login"
		if as.status.Route != agent.Local {
			detail = "→ " + as.status.Route
		}
		row("agent:"+as.agent.Name, "◆", as.agent.Title, detail)
	}
	y++
	sc.add(p.Label("Proxies"), 3, y, 1, "")
	y += 2
	for _, n := range m.cfg.Names() {
		row("proxy:"+n, m.proxyDot(p, n), n, "")
	}
	row("add", "+", "Add a proxy", "")
}

// proxyDot is a colored status dot for a proxy.
func (m Model) proxyDot(p skin.Palette, name string) string {
	r, ok := m.probes[name]
	switch {
	case !ok:
		return skin.Fg(p.Amber).Render("●")
	case r.Err() != nil:
		return skin.Fg(p.Red).Render("●")
	default:
		return skin.Fg(p.Green).Render("●")
	}
}

// drawDetail renders the selected item: label, heading, body, buttons and
// a few facts.
func (m Model) drawDetail(sc *scene, p skin.Palette, top int) {
	if !m.loaded {
		return
	}
	left := sidebarWidth + 4
	textW := min(72, m.width-left-4)
	d := m.detail(p)
	y := top + 1
	sc.add(p.Label(d.label), left, y, 2, "")
	y += 2
	sc.add(ansi.Truncate(p.Heading(d.heading), textW, "…"), left, y, 2, "")
	y += 2
	body := p.Body(d.body, textW)
	sc.add(body, left, y, 2, "")
	y += lipgloss.Height(body) + 1
	m.buttons(sc, p, d.buttons, left, y, 2)
	y += 4
	for _, f := range d.facts {
		sc.add(ansi.Truncate(f, textW, "…"), left, y, 2, "")
		y++
	}
}

// detailView is what the right side shows for the selected sidebar item.
type detailView struct {
	label, heading, body string
	buttons              []button
	facts                []string
}

func (m Model) detail(p skin.Palette) detailView {
	ids := m.homeIDs()
	id := ids[min(m.focus, len(ids)-1)]
	kind, name, _ := strings.Cut(id, ":")
	switch kind {
	case "agent":
		return m.agentDetail(p, m.statusOf(name))
	case "proxy":
		return m.proxyDetail(p, name)
	}
	return detailView{
		label:   "Proxy · new",
		heading: "Add a CLIProxyAPI server",
		body:    "A URL and one of the proxy's client API keys. agentctl checks the proxy answers before saving it.",
		buttons: []button{{"Add proxy", "add", true}},
	}
}

func fact(p skin.Palette, k, v string) string {
	return skin.Fg(p.Dim).Render(fmt.Sprintf("%-8s", k)) + skin.Fg(p.Muted).Render(v)
}

func (m Model) agentDetail(p skin.Palette, as agentStatus) detailView {
	title := as.agent.Title
	d := detailView{label: "Agent · " + title}
	path, _ := claude.SettingsPath()
	if as.agent.Name == codex.Name {
		path, _ = codex.ConfigPath()
	}
	switch {
	case as.err != nil:
		d.heading = "Can't read its config"
		d.body = as.err.Error()
	case as.status.Route == agent.Local:
		d.heading = "Uses its own login"
		d.body = "Requests use the account " + title + " is logged in with."
		d.buttons = []button{{"Switch route", "switch:" + as.agent.Name, true}, {"Tune", "tune:" + as.agent.Name, false}}
	default:
		d.heading = "Routed through " + as.status.Route
		d.body = "Requests use the accounts logged in on " + as.status.Route + "."
		d.buttons = []button{{"Switch route", "switch:" + as.agent.Name, true}, {"Use own login", "local:" + as.agent.Name, false}, {"Tune", "tune:" + as.agent.Name, false}}
		if as.agent.Name == codex.Name {
			d.buttons = append(d.buttons, button{"Refresh models", "refresh", false})
		}
	}
	if as.status.Model != "" {
		d.facts = append(d.facts, fact(p, "model", as.status.Model))
	}
	d.facts = append(d.facts, fact(p, "config", tildePath(path)))
	for _, n := range as.status.Notes {
		d.facts = append(d.facts, skin.Fg(p.Amber).Render("! "+n))
	}
	return d
}

func (m Model) proxyDetail(p skin.Palette, name string) detailView {
	d := detailView{
		label:   "Proxy · " + name,
		body:    "Agents routed here use the accounts logged in on this server.",
		buttons: []button{{"Test", "test:" + name, true}, {"Details", "details:" + name, false}, {"Remove", "rm:" + name, false}},
		facts:   []string{fact(p, "url", m.cfg.Proxies[name].URL)},
	}
	if who := m.usedBy(name); who != "" {
		d.facts = append(d.facts, fact(p, "used by", who))
	}
	r, ok := m.probes[name]
	switch {
	case !ok:
		d.heading = "Checking…"
	case r.HealthErr != nil:
		d.heading = "Unreachable"
		d.facts = append(d.facts, skin.Fg(p.Red).Render("✗ "+r.HealthErr.Error()))
	case r.ModelsErr != nil:
		d.heading = "Key rejected"
		d.facts = append(d.facts, skin.Fg(p.Red).Render("✗ "+r.ModelsErr.Error()))
	default:
		d.heading = fmt.Sprintf("Live · %d models", len(r.Models))
		d.facts = append(d.facts,
			fact(p, "latency", r.Latency.Round(time.Millisecond).String()),
			fact(p, "models", fmt.Sprintf("%d Claude · %d all agents · %d unverified", r.Counts.Claude, r.Counts.Other, r.Counts.Unknown)))
		if len(r.Disguised) > 0 || r.DiscoveryErr != nil || r.SlowDiscovery() {
			d.facts = append(d.facts, skin.Fg(p.Amber).Render("! Claude Code may not list this proxy's models well; see Details"))
		}
	}
	return d
}

var homeDir = os.UserHomeDir

func tildePath(path string) string {
	if home, err := homeDir(); err == nil && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// Dialogs -------------------------------------------------------------------

func (m Model) dialogWidth() int { return max(48, min(88, m.width-10)) }

// listRows is how many options a pick dialog shows at once.
func (m Model) listRows() int { return max(3, min(len(m.options), m.height-18)) }

// sizeDialog fits the viewport and inputs to the dialog.
func (m *Model) sizeDialog() {
	w := m.dialogWidth() - 6
	m.vp.SetWidth(w)
	m.vp.SetHeight(max(5, m.height-18))
	for i := range m.inputs {
		m.inputs[i].SetWidth(w - 4)
	}
}

// panel draws a dialog: label and heading, body lines, buttons on the right
// of the last row. It returns where the body starts so callers can overlay
// clickable rows.
func (m Model) panel(sc *scene, label, heading string, body []string, buttons []button) (bx, by int) {
	p := m.pal
	w := m.dialogWidth()
	inner := w - 6
	lines := []string{p.Label(label), "", p.Heading(heading), ""}
	lines = append(lines, body...)
	lines = append(lines, "", "", "", "")
	for i, l := range lines {
		l = ansi.Truncate(l, inner, "…")
		lines[i] = l + strings.Repeat(" ", max(0, inner-lipgloss.Width(l)))
	}
	content := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(p.Strong).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	h := lipgloss.Height(content)
	x, y := (m.width-w)/2, max(1, (m.height-h)/2)
	sc.add(content, x, y, 50, "m:panel")
	closeStyle := skin.Fg(p.Dim)
	if m.hover == "m:close" {
		closeStyle = skin.Fg(p.Text)
	}
	sc.add(closeStyle.Render("×"), x+w-4, y+2, 52, "m:close")
	right := x + w - 3
	for i := len(buttons) - 1; i >= 0; i-- {
		btn := p.Button(buttons[i].label, buttons[i].primary, m.hover == buttons[i].id)
		right -= lipgloss.Width(btn)
		sc.add(btn, right, y+h-5, 52, buttons[i].id)
		right--
	}
	return x + 3, y + 6
}

func (m Model) drawDialog(sc *scene) {
	p := m.pal
	switch m.dialog {
	case routeDialog, modelDialog:
		label, heading := "Route · "+title(m.req.Agent), "Where should "+title(m.req.Agent)+" send requests?"
		buttons := []button{{"Cancel", "m:cancel", false}}
		note := ""
		if m.dialog == modelDialog {
			label, heading = "Model · "+m.req.Route, "Which model on "+m.req.Route+"?"
			buttons = []button{{"Back", "m:back", false}, {"Cancel", "m:cancel", false}}
			note = "Claude models are only offered to Claude Code."
			if m.req.Agent == claude.Name {
				note = "Any model on the proxy works in Claude Code."
			}
		}
		rows := m.listRows()
		body := make([]string, rows+1)
		if note != "" {
			body = append([]string{skin.Fg(p.Muted).Render(note), ""}, body...)
		}
		bx, by := m.panel(sc, label, heading, body, buttons)
		if note != "" {
			by += 2
		}
		m.drawOptions(sc, bx, by, rows)
	case tuneDialog:
		m.drawTune(sc)
	case ackDialog:
		body := strings.Split(p.Body(claude.PolicyWarning, m.dialogWidth()-6), "\n")
		body = append(body, "", skin.Fg(p.Text).Render("You will only be asked once."))
		m.panel(sc, "Heads up", "Anthropic's terms and proxies", body, []button{
			{"Cancel", "m:cancel", false}, {"Continue", "m:ack", true},
		})
	case confirmDialog:
		body := append([]string{skin.Fg(p.Muted).Render("Every file is backed up to ~/.config/agentctl/backups first."), ""}, strings.Split(m.vp.View(), "\n")...)
		m.panel(sc, "Review · "+title(m.plan.Agent), "Apply these changes?", body, []button{
			{"Cancel", "m:cancel", false}, {"Apply", "m:apply", true},
		})
	case proxyDialog:
		m.panel(sc, "Proxy · "+m.proxy, "Health and models", strings.Split(m.vp.View(), "\n"), []button{
			{"Test again", "m:test", false}, {"Remove", "m:rm", false}, {"Close", "m:close", true},
		})
	case removeDialog:
		if who := m.usedBy(m.proxy); who != "" {
			m.panel(sc, "Proxy · "+m.proxy, "Still in use", []string{
				skin.Fg(p.Muted).Render(who + " routes through " + m.proxy + ". Switch it to another route first."),
			}, []button{{"OK", "m:close", true}})
			return
		}
		m.panel(sc, "Proxy · "+m.proxy, "Remove "+m.proxy+"?", []string{
			skin.Fg(p.Muted).Render("agentctl forgets this proxy and its key. Nothing changes on the server."),
		}, []button{{"Cancel", "m:cancel", false}, {"Remove", "m:remove", true}})
	case addDialog:
		m.drawAddForm(sc)
	}
}

func (m Model) drawTune(sc *scene) {
	p := m.pal
	rows := m.listRows()
	w := m.dialogWidth() - 6
	cur := m.tuneItems[m.pick]
	var about []string
	about = append(about, strings.Split(p.Body(cur.Why, w), "\n")...)
	about = append(about, skin.Fg(p.Dim).Render("Cost: ")+skin.Fg(p.Muted).Render(cur.Cost))
	for _, n := range cur.Notes {
		about = append(about, skin.Fg(p.Amber).Render("! "+n))
	}
	body := append([]string{skin.Fg(p.Muted).Render("Ticked items are applied; unticked ones reset to the default."), ""}, make([]string, rows+1)...)
	body = append(body, about...)
	bx, by := m.panel(sc, "Tune · "+title(m.tuneAgent), "Curated settings", body, []button{
		{"Reset to defaults", "m:tune-reset", false}, {"Cancel", "m:cancel", false}, {"Review changes", "m:tune-review", true},
	})
	labelW := 0
	for _, o := range m.options {
		labelW = max(labelW, lipgloss.Width(o.label))
	}
	for r := range rows {
		i := m.top + r
		if i >= len(m.options) {
			break
		}
		it := m.tuneItems[i]
		box := "[ ]"
		if m.tuneWant[it.ID] {
			box = "[x]"
		}
		detail := ""
		switch {
		case it.Applied && !m.tuneWant[it.ID]:
			detail = "will reset"
		case !it.Applied && m.tuneWant[it.ID]:
			detail = "will apply"
		case it.Applied:
			detail = "applied"
		}
		id := "m:opt:" + strconv.Itoa(i)
		sc.add(p.Row(box, fmt.Sprintf("%-*s", labelW, it.Title), detail, w, i == m.pick, m.hover == id), bx, by+2+r, 53, id)
	}
}

func (m Model) drawOptions(sc *scene, x, y, rows int) {
	p := m.pal
	w := m.dialogWidth() - 6
	labelW := 0
	for _, o := range m.options {
		labelW = max(labelW, lipgloss.Width(o.label))
	}
	for r := range rows {
		i := m.top + r
		if i >= len(m.options) {
			break
		}
		o := m.options[i]
		id := "m:opt:" + strconv.Itoa(i)
		label := fmt.Sprintf("%-*s", labelW, o.label)
		sc.add(p.Row("›", label, o.detail, w, i == m.pick, m.hover == id), x, y+r, 53, id)
	}
	if more := len(m.options) - m.top - rows; more > 0 {
		sc.add(skin.Fg(p.Dim).Render(fmt.Sprintf("  ↓ %d more", more)), x, y+rows, 53, "")
	}
}

func (m Model) drawAddForm(sc *scene) {
	p := m.pal
	labels := []string{"Name", "URL", "API key"}
	hints := []string{"a short name, e.g. spark", "e.g. its Tailscale serve URL", "optional; a client key from the proxy's config"}
	var body []string
	for i := range labels {
		body = append(body, skin.Fg(p.Text).Render(labels[i])+"  "+skin.Fg(p.Dim).Render(hints[i]), "", "", "")
	}
	if m.formErr != "" {
		body = append(body, skin.Fg(p.Red).Render("✗ "+m.formErr))
	}
	bx, by := m.panel(sc, "Proxy · new", "Add a CLIProxyAPI server", body, []button{
		{"Cancel", "m:cancel", false}, {"Check and add", "m:submit", true},
	})
	w := m.dialogWidth() - 8
	for i, in := range m.inputs {
		border := p.Line
		if i == m.field {
			border = p.Accent
		}
		box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).Width(w).Render(in.View())
		sc.add(box, bx, by+i*4+1, 53, "m:input:"+strconv.Itoa(i))
	}
}

// proxyReport is the detail text for one proxy: its check and its models.
func (m Model) proxyReport(name string) string {
	p := m.pal
	px := m.cfg.Proxies[name]
	ok, bad, warn := skin.Fg(p.Green), skin.Fg(p.Red), skin.Fg(p.Amber)
	width := max(20, m.vp.Width())
	var b strings.Builder
	b.WriteString(skin.Fg(p.Dim).Render(px.URL) + "\n\n")
	r, checked := m.probes[name]
	switch {
	case !checked:
		b.WriteString(skin.Spin(m.frame) + " checking…")
		return b.String()
	case r.HealthErr != nil:
		b.WriteString(bad.Render(lipgloss.Wrap("✗ unreachable: "+r.HealthErr.Error(), width, "")))
		return b.String()
	case r.ModelsErr != nil:
		b.WriteString(bad.Render("✗ " + r.ModelsErr.Error()))
		return b.String()
	}
	b.WriteString(ok.Render("✓ ") + skin.Fg(p.Muted).Render(fmt.Sprintf("healthy (%s), key accepted", r.Latency.Round(time.Millisecond))) + "\n")
	switch {
	case r.DiscoveryErr != nil:
		b.WriteString(warn.Render(lipgloss.Wrap("! Claude Code's /model list will not show proxy models: "+r.DiscoveryErr.Error(), width, "")) + "\n")
	case r.SlowDiscovery():
		b.WriteString(warn.Render("! listing models takes over 3s; Claude Code will show only its built-in models") + "\n")
	case len(r.Disguised) > 0:
		b.WriteString(warn.Render(lipgloss.Wrap(fmt.Sprintf("! Claude Code sees %d non-Claude models under disguised names (e.g. %s); set disable-cloaking-model-list on the proxy to show real names", len(r.Disguised), r.Disguised[0]), width, "")) + "\n")
	default:
		b.WriteString(ok.Render("✓ ") + skin.Fg(p.Muted).Render("Claude Code sees the proxy's models under their real names") + "\n")
	}
	b.WriteString("\n" + p.Label(fmt.Sprintf("Models · %d Claude · %d all agents · %d unverified", r.Counts.Claude, r.Counts.Other, r.Counts.Unknown)) + "\n\n")
	idW := 0
	for _, pm := range r.Models {
		idW = max(idW, len(pm.ID))
	}
	for _, pm := range r.Models {
		var who string
		switch models.Classify(pm.ID, pm.OwnedBy, px.AllowModels) {
		case models.Anthropic:
			who = skin.Fg(p.Accent).Render("Claude Code only")
		case models.Other:
			who = skin.Fg(p.Blue).Render("all agents")
		default:
			who = warn.Render("unverified · agentctl proxy allow " + name + " " + pm.ID)
		}
		b.WriteString(skin.Fg(p.Text).Render(fmt.Sprintf("%-*s  ", idW, pm.ID)) + skin.Fg(p.Dim).Render(fmt.Sprintf("%-12s ", pm.OwnedBy)) + who + "\n")
	}
	return b.String()
}
