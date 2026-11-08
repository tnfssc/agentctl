package dash

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/tui/neo"
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

// card adds content with a hard shadow two cells right and one down.
func (s *scene) card(p neo.Palette, content string, x, y, z int, id string) {
	w, h := lipgloss.Size(content)
	s.add(p.Shadow(w, h), x+2, y+1, z, "")
	s.add(content, x, y, z+1, id)
}

// View renders the dashboard and resolves mouse events against this frame.
func (m Model) View() tea.View {
	sc := &scene{}
	sc.add(strings.Repeat(strings.Repeat(" ", max(1, m.width))+"\n", max(1, m.height)), 0, 0, 0, "")
	m.drawHome(sc)
	if m.dialog != noDialog {
		m.drawDialog(sc)
	}
	m.drawFooter(sc)

	comp := lipgloss.NewCompositor(sc.layers...)
	content := comp.Render()
	if m.width > 0 && m.height > 0 {
		content = lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
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

// active reports whether element id is hovered, or focused by keyboard on
// the home screen.
func (m Model) active(id string) bool {
	if m.hover == id {
		return true
	}
	ids := m.homeIDs()
	return m.dialog == noDialog && m.focus < len(ids) && ids[m.focus] == id
}

type button struct {
	label, id string
	fill      color.Color
}

func (m Model) drawHome(sc *scene) {
	p := m.pal
	if m.dialog != noDialog {
		p = p.Dimmed()
	} else {
		for _, st := range p.Sparkles(m.width, m.height, m.frame/2, max(6, m.width*m.height/220)) {
			sc.add(st.Glyph, st.X, st.Y, 1, "")
		}
	}
	x, y := 2, 1-m.scroll
	full := max(30, m.width-6)

	// Header: the big shimmering title when there is room, else one line.
	toolbar := []button{
		{"REFRESH CODEX", "refresh", p.Cyan},
		{"TEST PROXIES", "testall", p.Lime},
		{"QUIT ×", "quit", p.Pink},
	}
	bw := 0
	for _, b := range toolbar {
		bw = max(bw, lipgloss.Width(b.label)+2)
	}
	tagline := p.On(p.Yellow).Render("Route Claude Code and Codex to their own login or a CLIProxyAPI server.")
	big := m.height >= 36 && full-4 >= neo.TitleWidth
	var lines []string
	side := false
	if big {
		lines = append(strings.Split(p.Title(p.Yellow, m.frame), "\n"), tagline)
		side = full-4 >= neo.TitleWidth+bw+6
	} else {
		lines = []string{p.SmallTitle(p.Yellow, m.frame) + p.On(p.Yellow).Render("   ") + tagline}
	}
	header := p.Card(p.Yellow, p.Ink, full, lines...)
	sc.card(p, header, x, y, 10, "")
	if side {
		for i, b := range toolbar {
			label := b.label + strings.Repeat(" ", bw-2-lipgloss.Width(b.label))
			sc.add(p.ButtonOn(p.Yellow, label, b.fill, m.hover == b.id), x+full-bw-4, y+1+i*2, 12, b.id)
		}
		y += lipgloss.Height(header) + 2
	} else {
		y += lipgloss.Height(header) + 2
		bx := x
		for _, b := range toolbar {
			btn := p.Button(b.label, b.fill, m.hover == b.id)
			sc.add(btn, bx, y, 12, b.id)
			bx += lipgloss.Width(btn) + 2
		}
		y += 3
	}

	if !m.loaded {
		sc.add(p.Text().Render(neo.Spin(m.frame)+" loading"), x, y, 10, "")
		return
	}

	// Agents: one card each, side by side when they fit.
	sc.add(p.Chip("agents", p.Ink)+" "+p.Dim().Render("click a card to choose where it sends requests"), x, y, 10, "")
	y += 2
	cols := 1
	if full >= 2*44+4 {
		cols = 2
	}
	cw := (full - (cols-1)*4) / cols
	fills := map[string]color.Color{claude.Name: p.Pink, "codex": p.Cyan}
	for i, as := range m.statuses {
		m.drawAgent(sc, p, as, fills[as.agent.Name], x+(i%cols)*(cw+4), y+(i/cols)*10, cw)
	}
	y += (len(m.statuses) + cols - 1) / cols * 10

	// Proxies: a grid of cards plus an "add" card.
	sc.add(p.Chip("proxies", p.Ink)+" "+p.Dim().Render("CLIProxyAPI servers you can route to"), x, y, 10, "")
	y += 2
	pcols := max(1, (full+4)/44)
	pw := (full - (pcols-1)*4) / pcols
	names := m.cfg.Names()
	for i, n := range names {
		m.drawProxy(sc, p, n, x+(i%pcols)*(pw+4), y+(i/pcols)*10, pw)
	}
	i := len(names)
	m.drawAddCard(sc, p, x+(i%pcols)*(pw+4), y+(i/pcols)*10, pw)
}

func (m Model) drawAgent(sc *scene, p neo.Palette, as agentStatus, fill color.Color, x, y, w int) {
	id := "agent:" + as.agent.Name
	on := p.On(fill)
	inner := w - 4
	name := on.Bold(true).Render(strings.ToUpper(as.agent.Title))
	var route, detail, note string
	switch {
	case as.err != nil:
		route, detail = p.Chip("error", p.Red), on.Render(as.err.Error())
	case as.status.Route == agent.Local:
		route = p.Chip("own login", p.Lime)
		detail = on.Render("Uses the login " + as.agent.Title + " already has.")
	default:
		route = p.Chip("proxy "+as.status.Route, p.Yellow)
		detail = on.Render("Through " + as.status.Route + " · " + as.status.URL)
	}
	if as.status.Model != "" {
		note = on.Render("model ") + on.Bold(true).Render(as.status.Model)
	}
	if len(as.status.Notes) > 0 {
		note = on.Render("! " + as.status.Notes[0])
	}
	gap := on.Render(strings.Repeat(" ", max(1, inner-lipgloss.Width(name)-lipgloss.Width(route))))
	border := p.Ink
	if m.active(id) {
		border = p.Yellow
	}
	sc.card(p, p.Card(fill, border, w, name+gap+route, detail, note, "", "", ""), x, y, 20, id)
	sid := "switch:" + as.agent.Name
	sc.add(p.ButtonOn(fill, "SWITCH ROUTE →", p.Yellow, m.hover == sid), x+2, y+5, 22, sid)
}

func (m Model) drawProxy(sc *scene, p neo.Palette, name string, x, y, w int) {
	id := "proxy:" + name
	fill := p.Paper
	on := p.On(fill)
	inner := w - 4
	r, checked := m.probes[name]
	var chip, summary, warn string
	switch {
	case !checked:
		chip, summary = p.Chip(neo.Spin(m.frame)+" checking", p.Yellow), on.Render("Talking to the proxy…")
	case r.HealthErr != nil:
		chip, summary = p.Chip("✗ down", p.Red), on.Render(r.HealthErr.Error())
	case r.ModelsErr != nil:
		chip, summary = p.Chip("✗ key", p.Red), on.Render(r.ModelsErr.Error())
	default:
		chip = p.Chip("✓ live", p.Lime)
		summary = on.Render(fmt.Sprintf("%d models · %d Claude · %s", len(r.Models), r.Counts.Claude, r.Latency.Round(time.Millisecond)))
		if len(r.Disguised) > 0 || r.DiscoveryErr != nil || r.SlowDiscovery() {
			warn = on.Render("! Claude Code may not list models well")
		}
	}
	title := on.Bold(true).Render(strings.ToUpper(name))
	gap := on.Render(strings.Repeat(" ", max(1, inner-lipgloss.Width(title)-lipgloss.Width(chip))))
	url := lipgloss.NewStyle().Foreground(lipgloss.Color("#5E5A52")).Background(fill).Render(m.cfg.Proxies[name].URL)
	border := p.Ink
	if m.active(id) {
		border = p.Pink
	}
	sc.card(p, p.Card(fill, border, w, title+gap+chip, url, summary, warn, "", ""), x, y, 20, id)
	bx := x + 2
	for _, b := range []button{
		{"TEST", "test:" + name, p.Lime}, {"DETAILS", "details:" + name, p.Cyan}, {"REMOVE", "rm:" + name, p.Red},
	} {
		btn := p.ButtonOn(fill, b.label, b.fill, m.hover == b.id)
		sc.add(btn, bx, y+5, 22, b.id)
		bx += lipgloss.Width(btn) + 1
	}
}

func (m Model) drawAddCard(sc *scene, p neo.Palette, x, y, w int) {
	fill := p.Violet
	on := p.On(fill)
	border := p.Ink
	if m.active("add") {
		border = p.Yellow
	}
	plus := "+"
	if m.frame/6%2 == 0 {
		plus = "*"
	}
	card := p.Card(fill, border, w,
		on.Bold(true).Render(plus+"  ADD A PROXY"),
		on.Render("A URL and an API key; agentctl checks"),
		on.Render("that it answers before saving it."), "", "", "")
	sc.card(p, card, x, y, 20, "add")
	sc.add(p.ButtonOn(fill, "ADD →", p.Yellow, m.hover == "add-btn"), x+2, y+5, 22, "add-btn")
}

func (m Model) drawFooter(sc *scene) {
	p := m.pal
	var hints string
	switch m.dialog {
	case noDialog:
		hints = "click, or ←/→ + enter · a add · t test · r refresh Codex · d remove · wheel scrolls · q quit"
	case routeDialog, modelDialog:
		hints = "click a row, or ↑/↓ + enter · esc back"
	case confirmDialog:
		hints = "y apply · n cancel · wheel or ↑/↓ scrolls"
	case addDialog:
		hints = "tab next field · enter submit · esc cancel"
	case proxyDialog:
		hints = "t test again · d remove · wheel scrolls · esc close"
	default:
		hints = "y yes · n no"
	}
	sc.add(p.Dim().Render(ansi.Truncate(hints, max(1, m.width-4), "…")), 2, m.height-1, 100, "")

	var toast string
	switch {
	case m.busy != "":
		toast = p.On(p.Yellow).Bold(true).Render(" " + neo.Spin(m.frame) + " " + m.busy + "… ")
	case m.toast != "" && m.toastErr:
		toast = p.On(p.Red).Bold(true).Render(" ✗ " + m.toast + " ")
	case m.toast != "":
		toast = p.On(p.Lime).Bold(true).Render(" ✓ " + m.toast + " ")
	}
	if toast != "" {
		sc.card(p, ansi.Truncate(toast, max(1, m.width-8), "…"), 2, m.height-4, 98, "")
	}
}

// Dialogs -------------------------------------------------------------------

func (m Model) dialogWidth() int { return max(40, min(96, m.width-8)) }

// listRows is how many options a pick dialog shows at once.
func (m Model) listRows() int { return max(3, min(len(m.options), m.height-18)) }

// sizeDialog fits the viewport and inputs to the dialog.
func (m *Model) sizeDialog() {
	w := m.dialogWidth() - 4
	m.vp.SetWidth(w)
	m.vp.SetHeight(max(5, m.height-18))
	for i := range m.inputs {
		m.inputs[i].SetWidth(w - 4)
	}
}

// panel draws a dialog with body lines and buttons along its bottom, and
// returns where the body starts so callers can overlay clickable rows.
func (m Model) panel(sc *scene, fill color.Color, title string, body []string, buttons []button) (bx, by int) {
	p := m.pal
	w := m.dialogWidth()
	lines := append([]string{""}, body...)
	lines = append(lines, "", "", "")
	content := p.Panel(w, fill, title, lines)
	h := lipgloss.Height(content)
	x, y := (m.width-w)/2, max(1, (m.height-h)/2-1)
	sc.card(p, content, x, y, 50, "m:panel")
	sc.add(p.On(fill).Bold(true).Render(" × "), x+w-5, y+1, 52, "m:close")
	right := x + w - 3
	for i := len(buttons) - 1; i >= 0; i-- {
		b := buttons[i]
		btn := p.Button(b.label, b.fill, m.hover == b.id)
		right -= lipgloss.Width(btn)
		sc.add(btn, right, y+h-4, 52, b.id)
		right -= 2
	}
	return x + 2, y + 3
}

func (m Model) drawDialog(sc *scene) {
	p := m.pal
	switch m.dialog {
	case routeDialog, modelDialog:
		heading := "Route " + title(m.req.Agent) + " to"
		buttons := []button{{"CANCEL", "m:cancel", p.Pink}}
		var note string
		if m.dialog == modelDialog {
			heading = "Model on " + m.req.Route
			buttons = []button{{"← BACK", "m:back", p.Cyan}, {"CANCEL", "m:cancel", p.Pink}}
			note = "Claude models are only offered to Claude Code."
			if m.req.Agent == claude.Name {
				note = "Any model on the proxy works in Claude Code."
			}
		}
		rows := m.listRows()
		body := make([]string, rows+1)
		if note != "" {
			body = append([]string{p.Dim().Render(note), ""}, body...)
		}
		bx, by := m.panel(sc, p.Yellow, heading, body, buttons)
		if note != "" {
			by += 2
		}
		m.drawOptions(sc, bx, by, rows)
	case ackDialog:
		body := strings.Split(p.Text().Render(lipgloss.Wrap(claude.PolicyWarning, m.dialogWidth()-6, "")), "\n")
		body = append(body, "", p.Text().Bold(true).Render("Route Claude Code through "+m.req.Route+" anyway? You will not be asked again."))
		m.panel(sc, p.Red, "Heads up: Anthropic's terms", body, []button{
			{"CANCEL", "m:cancel", p.Cyan}, {"I UNDERSTAND, CONTINUE", "m:ack", p.Yellow},
		})
	case confirmDialog:
		body := append([]string{p.Dim().Render("Every file is backed up to ~/.config/agentctl/backups first."), ""}, strings.Split(m.vp.View(), "\n")...)
		m.panel(sc, p.Lime, "Apply to "+title(m.plan.Agent)+"?", body, []button{
			{"CANCEL", "m:cancel", p.Pink}, {"APPLY ✓", "m:apply", p.Lime},
		})
	case proxyDialog:
		m.panel(sc, p.Cyan, "Proxy "+m.proxy, strings.Split(m.vp.View(), "\n"), []button{
			{"TEST AGAIN", "m:test", p.Lime}, {"REMOVE", "m:rm", p.Red}, {"CLOSE", "m:close", p.Yellow},
		})
	case removeDialog:
		if who := m.usedBy(m.proxy); who != "" {
			m.panel(sc, p.Yellow, "Can't remove "+m.proxy, []string{
				p.Text().Render(who + " is routed to " + m.proxy + ". Switch it to another route first."),
			}, []button{{"OK", "m:close", p.Yellow}})
			return
		}
		m.panel(sc, p.Red, "Remove "+m.proxy+"?", []string{
			p.Text().Render("agentctl forgets this proxy and its key. Nothing changes on the server."),
		}, []button{{"CANCEL", "m:cancel", p.Cyan}, {"REMOVE", "m:remove", p.Red}})
	case addDialog:
		m.drawAddForm(sc)
	}
}

func (m Model) drawOptions(sc *scene, x, y, rows int) {
	p := m.pal
	w := m.dialogWidth() - 4
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
		label := fmt.Sprintf(" %-*s  ", labelW, o.label)
		var row string
		switch {
		case i == m.pick:
			on := p.On(p.Yellow)
			row = p.Line(p.Yellow, w, on.Bold(true).Render("›"+label), on.Render(o.detail))
		case m.hover == id:
			on := p.On(p.Cyan)
			row = p.Line(p.Cyan, w, on.Render(" "+label), on.Render(o.detail))
		default:
			row = ansi.Truncate(p.Text().Render(" "+label)+p.Dim().Render(o.detail), w, "…")
			row += strings.Repeat(" ", max(0, w-lipgloss.Width(row)))
		}
		sc.add(row, x, y+r, 53, id)
	}
	if more := len(m.options) - m.top - rows; more > 0 {
		sc.add(p.Dim().Render(fmt.Sprintf(" ↓ %d more (wheel or ↓)", more)), x, y+rows, 53, "")
	}
}

func (m Model) drawAddForm(sc *scene) {
	p := m.pal
	labels := []string{"Name", "URL", "API key"}
	hints := []string{
		"a short name you will type, e.g. spark",
		"where the proxy listens, e.g. its Tailscale serve URL",
		"a client key from the proxy's config; kept in ~/.config/agentctl",
	}
	var body []string
	for i := range labels {
		body = append(body, p.Text().Bold(true).Render(labels[i])+"  "+p.Dim().Render(hints[i]), "", "", "")
	}
	if m.formErr != "" {
		body = append(body, lipgloss.NewStyle().Foreground(p.Red).Render("✗ "+m.formErr))
	}
	bx, by := m.panel(sc, p.Violet, "Add a proxy", body, []button{
		{"CANCEL", "m:cancel", p.Pink}, {"CHECK & ADD →", "m:submit", p.Lime},
	})
	w := m.dialogWidth() - 6
	for i, in := range m.inputs {
		border := p.Muted
		if i == m.field {
			border = p.Pink
		}
		box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(border).Width(w).Render(in.View())
		sc.add(box, bx, by+i*4+1, 53, "m:input:"+strconv.Itoa(i))
	}
}

// proxyReport is the detail text for one proxy: its check and its models.
func (m Model) proxyReport(name string) string {
	p := m.pal
	px := m.cfg.Proxies[name]
	ok := lipgloss.NewStyle().Foreground(p.Lime)
	bad := lipgloss.NewStyle().Foreground(p.Red)
	warn := lipgloss.NewStyle().Foreground(p.Yellow)
	width := max(20, m.vp.Width())
	var b strings.Builder
	b.WriteString(p.Dim().Render(px.URL) + "\n\n")
	r, checked := m.probes[name]
	switch {
	case !checked:
		b.WriteString(neo.Spin(m.frame) + " checking…")
		return b.String()
	case r.HealthErr != nil:
		b.WriteString(bad.Render(lipgloss.Wrap("✗ unreachable: "+r.HealthErr.Error(), width, "")))
		return b.String()
	case r.ModelsErr != nil:
		b.WriteString(bad.Render("✗ " + r.ModelsErr.Error()))
		return b.String()
	}
	b.WriteString(ok.Render(fmt.Sprintf("✓ healthy (%s), key accepted", r.Latency.Round(time.Millisecond))) + "\n")
	switch {
	case r.DiscoveryErr != nil:
		b.WriteString(warn.Render(lipgloss.Wrap("! Claude Code's /model list will not show proxy models: "+r.DiscoveryErr.Error(), width, "")) + "\n")
	case r.SlowDiscovery():
		b.WriteString(warn.Render("! listing models takes over 3s; Claude Code will show only its built-in models") + "\n")
	case len(r.Disguised) > 0:
		b.WriteString(warn.Render(lipgloss.Wrap(fmt.Sprintf("! Claude Code sees %d non-Claude models under disguised names (e.g. %s); set disable-cloaking-model-list on the proxy to show real names", len(r.Disguised), r.Disguised[0]), width, "")) + "\n")
	default:
		b.WriteString(ok.Render("✓ Claude Code sees the proxy's models under their real names") + "\n")
	}
	b.WriteString("\n" + p.Chip(fmt.Sprintf("models · %d claude · %d all agents · %d unverified", r.Counts.Claude, r.Counts.Other, r.Counts.Unknown), p.Ink) + "\n")
	idW := 0
	for _, pm := range r.Models {
		idW = max(idW, len(pm.ID))
	}
	for _, pm := range r.Models {
		var who string
		switch models.Classify(pm.ID, pm.OwnedBy, px.AllowModels) {
		case models.Anthropic:
			who = lipgloss.NewStyle().Foreground(p.Pink).Render("Claude Code only")
		case models.Other:
			who = ok.Render("all agents")
		default:
			who = warn.Render("unverified · agentctl proxy allow " + name + " " + pm.ID)
		}
		b.WriteString(p.Text().Render(fmt.Sprintf("%-*s  ", idW, pm.ID)) + p.Dim().Render(fmt.Sprintf("%-12s ", pm.OwnedBy)) + who + "\n")
	}
	return b.String()
}
