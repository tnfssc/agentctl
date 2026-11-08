package dash

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// Layout. Everything is placed on a fixed grid computed from the terminal
// size alone, never from content, so nothing moves while you use it (see
// "No layout shift" in AGENTS.md). Text that does not fit is truncated or
// scrolls inside its own fixed area.
const (
	minWidth  = 80
	minHeight = 24

	navTop     = 1 // nav row band: rows 1-3
	pageTop    = 5 // first row under the nav hairline
	pillWidth  = 30
	homeSlotW  = 15 // detail-pane button slot
	dialogSlot = 18 // dialog button slot
	bodyLines  = 3  // detail-pane description rows
	factRows   = 5  // detail-pane fact rows
)

type rect struct{ X, Y, W, H int }

// scene collects absolutely positioned layers; the compositor draws them by
// Z and answers "what is under the pointer" from the same layers. Rects of
// identified layers are kept so tests can check that nothing moves.
type scene struct {
	layers []*lipgloss.Layer
	rects  map[string]rect
}

func (s *scene) add(content string, x, y, z int, id string) {
	l := lipgloss.NewLayer(content).X(x).Y(y).Z(z)
	if id != "" {
		l = l.ID(id)
		w, h := lipgloss.Size(content)
		s.rects[id] = rect{x, y, w, h}
	}
	s.layers = append(s.layers, l)
}

func (m Model) scene() *scene {
	sc := &scene{rects: map[string]rect{}}
	sc.add(strings.Repeat(strings.Repeat(" ", max(1, m.width))+"\n", max(1, m.height)), 0, 0, 0, "")
	if m.width < minWidth || m.height < minHeight {
		msg := fmt.Sprintf("agentctl needs at least %d×%d; this terminal is %d×%d.", minWidth, minHeight, m.width, m.height)
		sc.add(m.pal.Body(msg, max(10, m.width-4)), 2, 1, 1, "")
		return sc
	}
	m.drawPage(sc)
	if m.dialog != noDialog {
		m.drawDialog(sc)
	}
	return sc
}

// View renders the dashboard and resolves mouse events against this frame.
func (m Model) View() tea.View {
	comp := lipgloss.NewCompositor(m.scene().layers...)
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

// Home ----------------------------------------------------------------------

func (m Model) sidebarWidth() int {
	if m.width >= 100 {
		return 30
	}
	return 28
}

var navLinks = []struct{ label, id string }{
	{"Add proxy", "add"}, {"Test proxies", "testall"}, {"Refresh Codex", "refresh"}, {"Quit", "quit"},
}

func (m Model) drawPage(sc *scene) {
	p := m.pal
	if m.dialog != noDialog {
		p = p.Faded()
	}
	w, h := m.width, m.height

	// Nav: wordmark, text links, status pill.
	sc.add(p.Wordmark(), 2, navTop+1, 1, "")
	x := 20
	for _, l := range navLinks {
		style := skin.Fg(p.Muted)
		if m.hover == l.id {
			style = skin.Fg(p.Text).Underline(true)
		}
		sc.add(style.Render(l.label), x, navTop+1, 1, l.id)
		x += lipgloss.Width(l.label) + 4
	}
	// The pill shows whenever the terminal is wide enough, whatever it says.
	if px := w - pillWidth - 2; px >= x {
		sc.add(m.pill(p), px, navTop, 1, "pill")
	}
	sc.add(p.HLine(w), 0, pageTop-1, 1, "")

	// Body: sidebar, hairline, detail.
	bottom := h - 3
	sw := m.sidebarWidth()
	sc.add(p.VLine(bottom-pageTop+1), sw, pageTop, 1, "")
	m.drawSidebar(sc, p, bottom)
	m.drawDetail(sc, p)

	// Footer: status on the left, hints from a fixed column on the right.
	sc.add(p.HLine(w), 0, h-2, 1, "")
	hintX := w - 2 - maxHintWidth
	var status string
	switch {
	case m.busy != "":
		status = skin.Fg(p.Accent).Render(skin.Spin(m.frame)+" ") + skin.Fg(p.Text).Render(m.busy+"…")
	case m.toast != "" && m.toastErr:
		status = skin.Fg(p.Red).Render("✗ ") + skin.Fg(p.Text).Render(m.toast)
	case m.toast != "":
		status = skin.Fg(p.Green).Render("✓ ") + skin.Fg(p.Text).Render(m.toast)
	}
	sc.add(skin.Cell(status, hintX-4), 2, h-1, 1, "status")
	sc.add(skin.Fg(p.Dim).Render(skin.Cell(m.hints(), maxHintWidth)), hintX, h-1, 1, "hints")
}

var hintTexts = map[dialog]string{
	noDialog:      "↑↓ select · enter open · q quit",
	routeDialog:   "↑↓ · enter choose · esc back",
	modelDialog:   "↑↓ · enter choose · esc back",
	tuneDialog:    "space toggle · y review · x reset",
	confirmDialog: "y apply · n cancel · ↑↓ scroll",
	addDialog:     "tab next · enter submit · esc",
	proxyDialog:   "t test · d remove · esc close",
	ackDialog:     "y yes · n no",
	removeDialog:  "y yes · n no",
}

var maxHintWidth = func() int {
	n := 0
	for _, h := range hintTexts {
		n = max(n, lipgloss.Width(h))
	}
	return n
}()

func (m Model) hints() string { return hintTexts[m.dialog] }

// pill summarizes proxy health like the site's "prod server / running".
func (m Model) pill(p skin.Palette) string {
	if !m.loaded {
		return p.Pill(p.Amber, "agentctl", "loading", pillWidth)
	}
	total := len(m.cfg.Proxies)
	if total == 0 {
		return p.Pill(p.Dim, "proxies", "none yet", pillWidth)
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
		return p.Pill(p.Amber, fmt.Sprintf("%d %s", total, noun), "checking", pillWidth)
	case live == total:
		return p.Pill(p.Green, fmt.Sprintf("%d %s", total, noun), "live", pillWidth)
	case live == 0:
		return p.Pill(p.Red, fmt.Sprintf("%d %s", total, noun), "down", pillWidth)
	default:
		return p.Pill(p.Amber, fmt.Sprintf("%d/%d %s", live, total, noun), "live", pillWidth)
	}
}

// proxyWindow is the first proxy shown and the window's height, keeping the
// selected proxy visible. The height depends only on the terminal; "Add a
// proxy" sits on the row below it, so it never moves.
func (m Model) proxyWindow(listTop, bottom int) (first, rows int) {
	names := m.cfg.Names()
	rows = max(1, bottom-listTop)
	if len(names) <= rows {
		return 0, rows
	}
	sel := m.focus - len(m.statuses)
	first = max(0, min(sel-rows+1, len(names)-rows))
	return first, rows
}

func (m Model) drawSidebar(sc *scene, p skin.Palette, bottom int) {
	if !m.loaded {
		sc.add(skin.Fg(p.Dim).Render(skin.Spin(m.frame)+" loading"), 2, pageTop+1, 1, "")
		return
	}
	rw := m.sidebarWidth() - 3
	ids := m.homeIDs()
	selected := ids[min(m.focus, len(ids)-1)]
	row := func(y int, id, icon, label string, labelW int, detail string) {
		sc.add(p.Row(icon, label, labelW, detail, rw, id == selected, m.hover == "nav:"+id), 2, y, 2, "nav:"+id)
	}
	y := pageTop + 1
	sc.add(p.Label("Agents"), 3, y, 1, "")
	y += 2
	for _, as := range m.statuses {
		detail := "own login"
		if as.status.Route != agent.Local {
			detail = "→ " + as.status.Route
		}
		row(y, "agent:"+as.agent.Name, "◆", as.agent.Title, 11, detail)
		y++
	}
	y++
	names := m.cfg.Names()
	first, rows := m.proxyWindow(y+2, bottom)
	label := p.Label("Proxies")
	if len(names) > rows {
		label += skin.Fg(p.Dim).Render(fmt.Sprintf("  %d–%d of %d", first+1, first+rows, len(names)))
	}
	sc.add(label, 3, y, 1, "")
	y += 2
	for i, n := range names[first:min(len(names), first+rows)] {
		row(y+i, "proxy:"+n, m.proxyDot(p, n), n, rw-4, "")
	}
	row(y+rows, "add", "+", "Add a proxy", rw-4, "")
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

// detailView is what the right side shows for the selected sidebar item.
// Buttons and facts sit in fixed slots; an empty slot stays empty.
type detailView struct {
	label, heading, body string
	buttons              [3]button
	facts                [factRows]string
}

// drawDetail renders the selected item on a fixed grid: label, heading, a
// three-line description, one row of button slots, then fact rows.
func (m Model) drawDetail(sc *scene, p skin.Palette) {
	if !m.loaded {
		return
	}
	left := m.sidebarWidth() + 3
	textW := min(72, m.width-left-2)
	d := m.detail(p)
	sc.add(p.Label(d.label), left, pageTop+1, 2, "")
	sc.add(skin.Cell(p.Heading(d.heading), textW), left, pageTop+2, 2, "")
	for i, line := range skin.Clip(d.body, textW, bodyLines) {
		sc.add(skin.Fg(p.Muted).Render(line), left, pageTop+4+i, 2, "")
	}
	by := pageTop + 4 + bodyLines + 1
	for i, b := range d.buttons {
		if b.label != "" {
			sc.add(p.Button(b.label, homeSlotW, b.primary, m.hover == b.id), left+i*(homeSlotW+1), by, 2, b.id)
		}
	}
	for i, f := range d.facts {
		sc.add(skin.Cell(f, textW), left, by+4+i, 2, "fact:"+strconv.Itoa(i))
	}
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
		buttons: [3]button{{"Add proxy", "add-btn", true}},
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
		d.buttons = [3]button{{"Switch route", "switch:" + as.agent.Name, true}, {}, {"Tune", "tune:" + as.agent.Name, false}}
	default:
		d.heading = "Routed through " + as.status.Route
		d.body = "Requests use the accounts logged in on " + as.status.Route + "."
		d.buttons = [3]button{
			{"Switch route", "switch:" + as.agent.Name, true},
			{"Own login", "local:" + as.agent.Name, false},
			{"Tune", "tune:" + as.agent.Name, false},
		}
	}
	if as.status.Model != "" {
		d.facts[0] = fact(p, "model", as.status.Model)
	}
	d.facts[1] = fact(p, "config", tildePath(path))
	if len(as.status.Notes) > 0 {
		d.facts[2] = skin.Fg(p.Amber).Render("! " + as.status.Notes[0])
	}
	return d
}

func (m Model) proxyDetail(p skin.Palette, name string) detailView {
	d := detailView{
		label:   "Proxy · " + name,
		body:    "Agents routed here use the accounts logged in on this server.",
		buttons: [3]button{{"Test", "test:" + name, true}, {"Details", "details:" + name, false}, {"Remove", "rm:" + name, false}},
	}
	d.facts[0] = fact(p, "url", m.cfg.Proxies[name].URL)
	if who := m.usedBy(name); who != "" {
		d.facts[1] = fact(p, "used by", who)
	}
	r, ok := m.probes[name]
	switch {
	case !ok:
		d.heading = "Checking…"
	case r.HealthErr != nil:
		d.heading = "Unreachable"
		d.facts[2] = skin.Fg(p.Red).Render("✗ " + r.HealthErr.Error())
	case r.ModelsErr != nil:
		d.heading = "Key rejected"
		d.facts[2] = skin.Fg(p.Red).Render("✗ " + r.ModelsErr.Error())
	default:
		d.heading = fmt.Sprintf("Live · %d models", len(r.Models))
		d.facts[2] = fact(p, "latency", r.Latency.Round(time.Millisecond).String())
		d.facts[3] = fact(p, "models", fmt.Sprintf("%d Claude · %d all agents · %d unverified", r.Counts.Claude, r.Counts.Other, r.Counts.Unknown))
		if len(r.Disguised) > 0 || r.DiscoveryErr != nil || r.SlowDiscovery() {
			d.facts[4] = skin.Fg(p.Amber).Render("! Claude Code may not list this proxy's models well; see Details")
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
//
// Every dialog uses the same shell: one size and position per terminal size,
// a label and heading at the top, a body area, and button slots along the
// bottom (extras from the left, Cancel and the primary action from the
// right). Bodies scroll or truncate inside their area.

func (m Model) dialogRect() rect {
	w := max(62, min(88, m.width-10))
	h := min(30, m.height-2)
	return rect{(m.width - w) / 2, (m.height - h) / 2, w, h}
}

// dialogBody is the area inside the shell for a dialog's content.
func (m Model) dialogBody() rect {
	d := m.dialogRect()
	return rect{d.X + 3, d.Y + 4, d.W - 6, d.H - 9}
}

// listRows is how many options a pick dialog shows at once; it depends on
// the terminal size only.
func (m Model) listRows() int {
	b := m.dialogBody()
	if m.dialog == tuneDialog {
		return max(1, b.H-tuneChrome)
	}
	return max(1, b.H-3)
}

// tuneChrome is the tune body minus its list: intro, blank, more row,
// blank, and the item description area.
const tuneChrome = 2 + 1 + 1 + tuneAbout

const tuneAbout = 5 // why (3), cost (1), note (1)

// confirm body: backup note, blank, two warning rows, blank, then the diff.
const confirmChrome = 5

// sizeDialog fits the viewport and inputs to the dialog body.
func (m *Model) sizeDialog() {
	b := m.dialogBody()
	m.vp.SetWidth(b.W)
	h := b.H
	if m.dialog == confirmDialog {
		h -= confirmChrome
	}
	m.vp.SetHeight(max(1, h))
	for i := range m.inputs {
		m.inputs[i].SetWidth(b.W - 4)
	}
}

// panel draws the dialog shell. extras fill slots from the left; cancel and
// primary take the two right-hand slots.
func (m Model) panel(sc *scene, label, heading string, extras []button, cancel, primary *button) {
	p := m.pal
	d := m.dialogRect()
	shell := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(p.Strong).
		Width(d.W).Height(d.H).Render("")
	sc.add(shell, d.X, d.Y, 50, "m:panel")
	iw := d.W - 6
	sc.add(skin.Cell(p.Label(label), iw-2), d.X+3, d.Y+1, 51, "")
	sc.add(skin.Cell(p.Heading(heading), iw), d.X+3, d.Y+2, 51, "")
	closeStyle := skin.Fg(p.Dim)
	if m.hover == "m:close" {
		closeStyle = skin.Fg(p.Text)
	}
	sc.add(closeStyle.Render("×"), d.X+d.W-4, d.Y+1, 52, "m:close")

	by := d.Y + d.H - 4
	put := func(b *button, x int) {
		if b != nil {
			sc.add(p.Button(b.label, dialogSlot, b.primary, m.hover == b.id), x, by, 52, b.id)
		}
	}
	right := d.X + d.W - 3
	put(primary, right-dialogSlot)
	put(cancel, right-2*dialogSlot-1)
	for i := range extras {
		put(&extras[i], d.X+3+i*(dialogSlot+1))
	}
}

func (m Model) drawDialog(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	cancel := &button{"Cancel", "m:cancel", false}
	text := func(lines []string) {
		for i, l := range lines {
			if i < b.H {
				sc.add(skin.Cell(l, b.W), b.X, b.Y+i, 53, "")
			}
		}
	}
	switch m.dialog {
	case routeDialog, modelDialog:
		agentTitle := title(m.req.Agent)
		if m.dialog == routeDialog {
			m.panel(sc, "Route · "+agentTitle, "Where should "+agentTitle+" send requests?", nil, cancel, nil)
		} else {
			note := "Claude models are only offered to Claude Code."
			if m.req.Agent == claude.Name {
				note = "Any model on the proxy works in Claude Code."
			}
			m.panel(sc, "Model · "+m.req.Route, "Which model on "+m.req.Route+"?", []button{{"Back", "m:back", false}}, cancel, nil)
			text([]string{skin.Fg(p.Muted).Render(note)})
		}
		m.drawOptions(sc, b.X, b.Y+2, m.listRows())
	case tuneDialog:
		m.drawTune(sc)
	case ackDialog:
		m.panel(sc, "Heads up", "Anthropic's terms and proxies", nil, cancel, &button{"Continue", "m:ack", true})
		lines := skin.Clip(claude.PolicyWarning, b.W, b.H-2)
		for i := range lines {
			lines[i] = skin.Fg(p.Muted).Render(lines[i])
		}
		text(append(lines, "", skin.Fg(p.Text).Render("You will only be asked once.")))
	case confirmDialog:
		m.panel(sc, "Review · "+title(m.plan.Agent), "Apply these changes?", nil, cancel, &button{"Apply", "m:apply", true})
		warn := make([]string, 2)
		for i, w := range m.plan.Warnings {
			switch {
			case i == 1 && len(m.plan.Warnings) > 2:
				warn[1] = fmt.Sprintf("! and %d more warnings", len(m.plan.Warnings)-1)
			case i < 2:
				warn[i] = "! " + w
			}
		}
		text([]string{
			skin.Fg(p.Muted).Render("Every file is backed up to ~/.config/agentctl/backups first."), "",
			skin.Fg(p.Amber).Render(warn[0]), skin.Fg(p.Amber).Render(warn[1]),
		})
		sc.add(m.vp.View(), b.X, b.Y+confirmChrome, 53, "m:body")
	case proxyDialog:
		m.panel(sc, "Proxy · "+m.proxy, "Health and models",
			[]button{{"Test again", "m:test", false}, {"Remove", "m:rm", false}}, nil, &button{"Close", "m:close-btn", true})
		sc.add(m.vp.View(), b.X, b.Y, 53, "m:body")
	case removeDialog:
		if who := m.usedBy(m.proxy); who != "" {
			m.panel(sc, "Proxy · "+m.proxy, "Still in use", nil, nil, &button{"OK", "m:close-btn", true})
			text(skin.Clip(who+" routes through "+m.proxy+". Switch it to another route first.", b.W, b.H))
			return
		}
		m.panel(sc, "Proxy · "+m.proxy, "Remove "+m.proxy+"?", nil, cancel, &button{"Remove", "m:remove", true})
		text(skin.Clip("agentctl forgets this proxy and its key. Nothing changes on the server.", b.W, b.H))
	case addDialog:
		m.drawAddForm(sc)
	}
}

func (m Model) drawTune(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	m.panel(sc, "Tune · "+title(m.tuneAgent), "Curated settings",
		[]button{{"Reset to default", "m:tune-reset", false}}, &button{"Cancel", "m:cancel", false}, &button{"Review", "m:tune-review", true})
	sc.add(skin.Cell(skin.Fg(p.Muted).Render("Ticked items are applied; unticked ones reset to the default."), b.W), b.X, b.Y, 53, "")
	rows := m.listRows()
	labelW := b.W * 6 / 10
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
		state := ""
		switch {
		case it.Applied && !m.tuneWant[it.ID]:
			state = "will reset"
		case !it.Applied && m.tuneWant[it.ID]:
			state = "will apply"
		case it.Applied:
			state = "applied"
		}
		id := "m:opt:" + strconv.Itoa(i)
		sc.add(p.Row(box, it.Title, labelW, state, b.W, i == m.pick, m.hover == id), b.X, b.Y+2+r, 53, id)
	}
	m.drawMore(sc, b.X, b.Y+2+rows, rows)

	cur := m.tuneItems[m.pick]
	about := b.Y + b.H - tuneAbout
	for i, line := range skin.Clip(cur.Why, b.W, 3) {
		sc.add(skin.Fg(p.Muted).Render(line), b.X, about+i, 53, "")
	}
	sc.add(skin.Cell(skin.Fg(p.Dim).Render("Cost: ")+skin.Fg(p.Muted).Render(cur.Cost), b.W), b.X, about+3, 53, "")
	if len(cur.Notes) > 0 {
		sc.add(skin.Cell(skin.Fg(p.Amber).Render("! "+cur.Notes[0]), b.W), b.X, about+4, 53, "")
	}
}

// drawMore reserves one row under a list for "↓ N more".
func (m Model) drawMore(sc *scene, x, y, rows int) {
	if more := len(m.options) - m.top - rows; more > 0 {
		sc.add(skin.Fg(m.pal.Dim).Render(fmt.Sprintf("  ↓ %d more", more)), x, y, 53, "")
	}
}

func (m Model) drawOptions(sc *scene, x, y, rows int) {
	p := m.pal
	w := m.dialogBody().W
	labelW := w * 45 / 100
	for r := range rows {
		i := m.top + r
		if i >= len(m.options) {
			break
		}
		o := m.options[i]
		id := "m:opt:" + strconv.Itoa(i)
		sc.add(p.Row("›", o.label, labelW, o.detail, w, i == m.pick, m.hover == id), x, y+r, 53, id)
	}
	m.drawMore(sc, x, y+rows, rows)
}

func (m Model) drawAddForm(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	m.panel(sc, "Proxy · new", "Add a CLIProxyAPI server", nil, &button{"Cancel", "m:cancel", false}, &button{"Check and add", "m:submit", true})
	labels := []string{"Name", "URL", "API key"}
	hints := []string{"a short name, e.g. spark", "e.g. its Tailscale serve URL", "optional; a client key from the proxy's config"}
	for i := range labels {
		sc.add(skin.Cell(skin.Fg(p.Text).Render(labels[i])+"  "+skin.Fg(p.Dim).Render(hints[i]), b.W), b.X, b.Y+i*4, 53, "")
	}
	for i, in := range m.inputs {
		border := p.Line
		if i == m.field {
			border = p.Accent
		}
		box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).Width(b.W).Render(skin.Cell(in.View(), b.W-2))
		sc.add(box, b.X, b.Y+i*4+1, 53, "m:input:"+strconv.Itoa(i))
	}
	// The error row is always reserved, empty or not.
	sc.add(skin.Cell(skin.Fg(p.Red).Render(errLine(m.formErr)), b.W), b.X, b.Y+12, 53, "m:error")
}

func errLine(s string) string {
	if s == "" {
		return ""
	}
	return "✗ " + s
}

// proxyReport is the detail text for one proxy: its check and its models.
// The first seven rows are always the same rows, so the model list starts at
// the same place whatever the check found.
func (m Model) proxyReport(name string) string {
	p := m.pal
	px := m.cfg.Proxies[name]
	ok, bad, warn := skin.Fg(p.Green), skin.Fg(p.Red), skin.Fg(p.Amber)
	width := max(20, m.dialogBody().W)
	lines := make([]string, 7)
	lines[0] = skin.Fg(p.Dim).Render(px.URL)
	r, checked := m.probes[name]
	switch {
	case !checked:
		lines[2] = skin.Spin(m.frame) + " checking…"
	case r.HealthErr != nil:
		e := skin.Clip("✗ unreachable: "+r.HealthErr.Error(), width, 2)
		lines[2] = bad.Render(e[0])
		if len(e) > 1 {
			lines[3] = bad.Render(e[1])
		}
	case r.ModelsErr != nil:
		lines[2] = bad.Render("✗ " + r.ModelsErr.Error())
	default:
		lines[2] = ok.Render("✓ ") + skin.Fg(p.Muted).Render(fmt.Sprintf("healthy (%s), key accepted", r.Latency.Round(time.Millisecond)))
		switch {
		case r.DiscoveryErr != nil:
			lines[3] = warn.Render("! Claude Code's /model list will not show proxy models: " + r.DiscoveryErr.Error())
		case r.SlowDiscovery():
			lines[3] = warn.Render("! listing models takes over 3s; Claude Code will show only its built-in models")
		case len(r.Disguised) > 0:
			lines[3] = warn.Render(fmt.Sprintf("! Claude Code sees %d models under disguised names; set disable-cloaking-model-list", len(r.Disguised)))
		default:
			lines[3] = ok.Render("✓ ") + skin.Fg(p.Muted).Render("Claude Code sees the proxy's models under their real names")
		}
		lines[5] = p.Label(fmt.Sprintf("Models · %d Claude · %d all agents · %d unverified", r.Counts.Claude, r.Counts.Other, r.Counts.Unknown))
	}
	idW, ownerW := width*4/10, 12
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
		lines = append(lines, skin.Fg(p.Text).Render(skin.Cell(pm.ID, idW))+"  "+skin.Fg(p.Dim).Render(skin.Cell(pm.OwnedBy, ownerW))+"  "+who)
	}
	for i := range lines {
		lines[i] = skin.Cell(lines[i], width) // one row each, whatever the data holds
	}
	return strings.Join(lines, "\n")
}
