package dash

import (
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/agent/codex"
	"github.com/tnfssc/agentctl/internal/agent/opencode"
	"github.com/tnfssc/agentctl/internal/agent/pi"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/tui/skin"
	"github.com/tnfssc/agentctl/internal/tune"
)

// Layout. Everything is placed on a fixed grid computed from the terminal
// size alone, never from content, so nothing moves while you use it (see
// "No layout shift" in AGENTS.md). Text that does not fit is truncated or
// scrolls inside its own fixed area.
const (
	minWidth  = 80 // narrower is the compact layout
	minHeight = 20 // shorter is the compact layout
	logoRows  = 24 // from this height the header has the three-row logo

	navTop     = 1  // nav row band: rows 1-3
	pageTop    = 5  // first row under the nav hairline
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
	if m.tooSmall() {
		msg := fmt.Sprintf("agentctl needs at least %d×%d; this terminal is %d×%d.", compactMinWidth, compactMinHeight, m.width, m.height)
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
	switch {
	case m.dialog == pairDialog:
		// The terminal keeps the mouse, so the link can be selected and
		// copied; the dialog's keys still work.
		v.MouseMode = tea.MouseModeNone
	case m.remote:
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

func (m Model) drawPage(sc *scene) {
	p := m.pal
	if m.dialog != noDialog {
		p = p.Faded()
	}
	w, h := m.width, m.height
	if m.compact() {
		m.drawCompactHeader(sc, p)
		if m.page == pageDetail {
			m.drawCompactDetail(sc, p)
		} else {
			m.drawNav(sc, p)
		}
		m.drawFooter(sc, p)
		return
	}

	// Header: the logo alone; actions live on the items they act on, and
	// state in the sidebar's dots. A short terminal gets the one-line
	// wordmark.
	if m.shortHeader() {
		sc.add(p.Wordmark(), 2, 0, 1, "")
	} else {
		sc.add(p.Logo(), 2, navTop, 1, "")
	}
	top := m.contentTop()
	sc.add(p.HLine(w), 0, top-1, 1, "")

	// Body: sidebar, hairline, detail.
	bottom := h - 3
	sw := m.sidebarWidth()
	sc.add(p.VLine(bottom-top+1), sw, top, 1, "")
	m.drawNav(sc, p)
	m.drawDetail(sc, p)
	m.drawFooter(sc, p)
}

// drawFooter puts the status on the left and hints from a fixed column on
// the right. A compact footer has room for one: the status when there is
// one, else the hints.
func (m Model) drawFooter(sc *scene, p skin.Palette) {
	w, h := m.width, m.height
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
	if m.compact() {
		if status == "" {
			status = skin.Fg(p.Dim).Render(m.hints())
		}
		sc.add(skin.Cell(status, w-2), 1, h-1, 1, "status")
		return
	}
	sc.add(skin.Cell(status, hintX-4), 2, h-1, 1, "status")
	sc.add(skin.Fg(p.Dim).Render(skin.Cell(m.hints(), maxHintWidth)), hintX, h-1, 1, "hints")
}

var hintTexts = map[dialog]string{
	noDialog:       "↑↓ select · enter open · q quit",
	routeDialog:    "↑↓ · enter choose · esc back",
	modelDialog:    "↑↓ · enter choose · esc back",
	tuneDialog:     "space toggle · y review · x reset",
	confirmDialog:  "y apply · n cancel · ↑↓ scroll",
	addDialog:      "tab next · enter submit · esc",
	proxyDialog:    "t test · d remove · esc close",
	ackDialog:      "y yes · n no",
	removeDialog:   "y yes · n no",
	askDialog:      "y yes · n no",
	devicesDialog:  "p pair · r revoke · i idle · esc",
	pairFormDialog: "tab next · enter submit · esc",
	pairDialog:     "select the link · c copy · enter done",
	serviceDialog:  "↑↓ · enter choose · esc back",
	signingDialog:  "c copy · enter done",
}

var maxHintWidth = func() int {
	n := 0
	for _, h := range hintTexts {
		n = max(n, lipgloss.Width(h))
	}
	return n
}()

func (m Model) hints() string {
	switch {
	case m.dialog == signingDialog && m.remote:
		return "enter done" // no clipboard over SSH
	case m.compact() && m.dialog == noDialog && m.page == pageDetail:
		return "1-3 buttons · esc back"
	case m.compact() && m.dialog == noDialog:
		return "tap or ↑↓ enter · q quit"
	}
	return hintTexts[m.dialog]
}

// proxyDot is the color of a proxy's status dot.
func (m Model) proxyDot(p skin.Palette, name string) color.Color {
	r, ok := m.probes[name]
	switch {
	case !ok:
		return p.Amber
	case r.Err() != nil:
		return p.Red
	default:
		return p.Green
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
	top := m.bodyTop()
	sc.add(p.Label(d.label), left, top, 2, "")
	sc.add(skin.Cell(p.Heading(d.heading), textW), left, top+1, 2, "")
	for i, line := range skin.Clip(d.body, textW, bodyLines) {
		sc.add(skin.Fg(p.Muted).Render(line), left, top+3+i, 2, "")
	}
	by := top + 3 + bodyLines + 1
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
	case "t3":
		return m.t3Detail(p)
	case "opencode":
		return m.ocDetail(p)
	case "git":
		return m.gitDetail(p)
	case "behavior":
		path, _ := tune.InstructionsPath()
		d := detailView{
			label:   "Behavior · all agents",
			heading: "Shared agent instructions",
			body:    "Curated sections of " + tildePath(path) + ", read by every agent. Your own text there stays untouched.",
			buttons: [3]button{{"Tune", "tune:" + tune.Instructions, true}},
		}
		d.facts[1] = fact(p, "file", tildePath(path))
		return d
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
	path := configPath(as.agent.Name)
	tuneBtn := button{"Tune", "tune:" + as.agent.Name, false}
	if len(tune.Items(as.agent.Name)) == 0 {
		tuneBtn = button{}
	}
	switch {
	case as.err != nil:
		d.heading = "Can't read its config"
		d.body = as.err.Error()
	case as.status.Route == agent.Local:
		d.heading = "Uses its own login"
		d.body = "Requests use the account " + title + " is logged in with."
		d.buttons = [3]button{{"Switch route", "switch:" + as.agent.Name, true}, {}, tuneBtn}
	default:
		d.heading = "Routed through " + as.status.Route
		d.body = "Requests use the accounts logged in on " + as.status.Route + "."
		d.buttons = [3]button{
			{"Switch route", "switch:" + as.agent.Name, true},
			{"Own login", "local:" + as.agent.Name, false},
			tuneBtn,
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

// configPath is the file that holds an agent's route.
func configPath(agentName string) string {
	var path string
	switch agentName {
	case claude.Name:
		path, _ = claude.SettingsPath()
	case codex.Name:
		path, _ = codex.ConfigPath()
	case opencode.Name:
		path, _ = opencode.ConfigPath()
	case pi.Name:
		path, _ = pi.ModelsPath()
	}
	return path
}

func (m Model) proxyDetail(p skin.Palette, name string) detailView {
	d := detailView{
		label:   "Proxy · " + name,
		body:    "Agents routed here use the accounts logged in on this server.",
		buttons: [3]button{{"Test", "test:" + name, true}, {"Refresh models", "refresh", false}, {"Remove", "rm:" + name, false}},
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
		d.facts[3] = fact(p, "models", r.Counts.String())
		if r.DiscoveryErr != nil || r.SlowDiscovery() {
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

const (
	maxDialogH   = 30
	dialogChrome = 9 // shell rows that are not body: border, label, heading, gaps, buttons
)

func (m Model) dialogRect() rect {
	if m.compact() {
		return m.content() // the whole page; header and footer stay
	}
	w := max(62, min(88, m.width-10))
	h := min(maxDialogH, m.height-2)
	return rect{(m.width - w) / 2, (m.height - h) / 2, w, h}
}

// dialogBody is the area inside the shell for a dialog's content.
func (m Model) dialogBody() rect {
	d := m.dialogRect()
	if m.compact() {
		// Border, label and heading above; buttons and border below.
		return rect{d.X + 2, d.Y + 3, d.W - 4, d.H - 7}
	}
	return rect{d.X + 3, d.Y + 4, d.W - 6, d.H - dialogChrome}
}

// listRows is how many options a pick dialog shows at once, each two rows
// tall; it depends on the terminal size only.
func (m Model) listRows() int {
	b := m.dialogBody()
	switch m.dialog {
	case tuneDialog:
		return max(1, (b.H-m.tuneChrome())/2)
	case devicesDialog:
		return m.devicesRows()
	}
	return max(1, (b.H-3)/2) // intro and blank above, more row below
}

// tuneChrome is the tune body minus its list: intro, blank, more row,
// blank, and the item description area.
func (m Model) tuneChrome() int { return 2 + 1 + 1 + m.tuneAbout() }

// tuneAbout is the item description area: why (3), cost (1) and note (1),
// or why and cost on one row each when the dialog is short.
func (m Model) tuneAbout() int {
	if m.dialogBody().H < 16 {
		return 2
	}
	return 5
}

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
	w := b.W - 4
	if m.compact() {
		if _, inline := m.fieldLayout(); inline {
			w = b.W - labelW - 3
		} else {
			w = b.W - 3
		}
	}
	for i := range m.inputs {
		m.inputs[i].SetWidth(max(1, w))
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
	pad := 3
	if m.compact() {
		pad = 2
	}
	iw := d.W - 2*pad
	sc.add(skin.Cell(p.Label(label), iw-2), d.X+pad, d.Y+1, 51, "")
	sc.add(skin.Cell(p.Heading(heading), iw), d.X+pad, d.Y+2, 51, "")
	closeStyle := skin.Fg(p.Dim)
	if m.hover == "m:close" {
		closeStyle = skin.Fg(p.Text)
	}
	sc.add(closeStyle.Render("×"), d.X+d.W-pad-1, d.Y+1, 52, "m:close")

	by := d.Y + d.H - 4
	if m.compact() {
		// Three equal slots across: extras from the left, then cancel, then
		// the primary action on the right.
		sw := (iw - 2) / 3
		put := func(b *button, slot int) {
			if b != nil {
				sc.add(p.Button(fit(b.label, sw), sw, b.primary, m.hover == b.id), d.X+pad+slot*(sw+1), by, 52, b.id)
			}
		}
		put(primary, 2)
		put(cancel, 1)
		for i := range extras {
			if i == 0 || cancel == nil {
				put(&extras[i], i)
			}
		}
		return
	}
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
	case askDialog:
		m.panel(sc, m.ask.title, m.ask.heading, nil, cancel, &button{m.ask.button, "m:yes", true})
		text(skin.Clip(m.ask.body, b.W, b.H))
	case devicesDialog:
		m.drawDevices(sc)
	case pairFormDialog:
		m.drawPairForm(sc)
	case pairDialog:
		m.drawPairing(sc)
	case signingDialog:
		m.drawSigning(sc)
	case serviceDialog:
		if m.svc == svcOpenCode {
			m.panel(sc, "OpenCode web · server", "Background server", nil, cancel, nil)
			text([]string{skin.Fg(p.Muted).Render("Started on demand by opencode, not at boot; it serves the web app and the TUI.")})
		} else {
			m.panel(sc, "T3 Code · service", "Background service", nil, cancel, nil)
			text([]string{skin.Fg(p.Muted).Render("Runs T3 Code at boot under systemd or launchd; data in " + tildePath(m.t3.Home) + ".")})
		}
		m.drawOptions(sc, b.X, b.Y+2, m.listRows())
	}
}

func (m Model) drawTune(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	m.panel(sc, "Tune · "+title(m.tuneAgent), "Curated settings",
		[]button{{"Reset to default", "m:tune-reset", false}}, &button{"Cancel", "m:cancel", false}, &button{"Review", "m:tune-review", true})
	sc.add(skin.Cell(skin.Fg(p.Muted).Render("Ticked items are applied; unticked ones reset to the default."), b.W), b.X, b.Y, 53, "")
	rows := m.listRows()
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
		sc.add(p.Tall(box, it.Title, state, "", b.W, i == m.pick, m.hover == id), b.X, b.Y+2+2*r, 53, id)
	}
	m.drawMore(sc, b.X, b.Y+2+2*rows, rows)

	cur := m.tuneItems[m.pick]
	aboutRows := m.tuneAbout()
	about := b.Y + b.H - aboutRows
	whyRows := max(1, aboutRows-2)
	for i, line := range skin.Clip(cur.Why, b.W, whyRows) {
		sc.add(skin.Fg(p.Muted).Render(line), b.X, about+i, 53, "")
	}
	sc.add(skin.Cell(skin.Fg(p.Dim).Render("Cost: ")+skin.Fg(p.Muted).Render(cur.Cost), b.W), b.X, about+whyRows, 53, "")
	if len(cur.Notes) > 0 && aboutRows > 2 {
		sc.add(skin.Cell(skin.Fg(p.Amber).Render("! "+cur.Notes[0]), b.W), b.X, about+whyRows+1, 53, "")
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
	for r := range rows {
		i := m.top + r
		if i >= len(m.options) {
			break
		}
		o := m.options[i]
		id := "m:opt:" + strconv.Itoa(i)
		sc.add(p.Tall("›", o.label, o.detail, "", w, i == m.pick, m.hover == id), x, y+2*r, 53, id)
	}
	m.drawMore(sc, x, y+2*rows, rows)
}

func (m Model) drawAddForm(sc *scene) {
	m.panel(sc, "Proxy · new", "Add a CLIProxyAPI server", nil, &button{"Cancel", "m:cancel", false}, &button{"Check and add", "m:submit", true})
	m.drawFields(sc, []string{"Name", "URL", "API key"},
		[]string{"a short name, e.g. spark", "e.g. its Tailscale serve URL", "optional; a client key from the proxy's config"})
}

// formErrRow is the row every wide form reserves for its error, empty or not.
const formErrRow = 12

// fieldLayout is where form fields go: rows per field, and whether label
// and input share a row. Compact dialogs give each field a label line and a
// one-line input, or one line for both when even that doesn't fit; the
// choice depends on the terminal size only.
func (m Model) fieldLayout() (step int, inline bool) {
	switch b := m.dialogBody(); {
	case !m.compact():
		return 4, false
	case maxFields*2+1 <= b.H:
		return 2, false
	default:
		return 1, true
	}
}

// labelW is the label column of inline fields.
const labelW = 10

// maxFields is the most fields any form has; every form lays out as if it
// had that many, so all forms match at a given terminal size.
const maxFields = 3

// drawFields draws a form's labeled inputs and its reserved error row.
func (m Model) drawFields(sc *scene, labels, hints []string) {
	p := m.pal
	b := m.dialogBody()
	step, inline := m.fieldLayout()
	for i, in := range m.inputs {
		y := b.Y + i*step
		id := "m:input:" + strconv.Itoa(i)
		switch {
		case inline:
			sc.add(skin.Cell(skin.Fg(p.Text).Render(labels[i]), labelW), b.X, y, 53, "")
			sc.add(m.inputLine(in, i == m.field, b.W-labelW), b.X+labelW, y, 53, id)
		case m.compact():
			sc.add(skin.Cell(skin.Fg(p.Text).Render(labels[i])+"  "+skin.Fg(p.Dim).Render(hints[i]), b.W), b.X, y, 53, "")
			sc.add(m.inputLine(in, i == m.field, b.W), b.X, y+1, 53, id)
		default:
			sc.add(skin.Cell(skin.Fg(p.Text).Render(labels[i])+"  "+skin.Fg(p.Dim).Render(hints[i]), b.W), b.X, y, 53, "")
			sc.add(m.inputBox(in, i == m.field, b.W), b.X, y+1, 53, id)
		}
	}
	errY := b.Y + formErrRow
	if m.compact() {
		errY = b.Y + b.H - 1
	}
	// The error row is always reserved, empty or not.
	sc.add(skin.Cell(skin.Fg(p.Red).Render(errLine(m.formErr)), b.W), b.X, errY, 53, "m:error")
}

// inputLine is a one-row input for compact forms, its caret accented when
// focused.
func (m Model) inputLine(in textinput.Model, focused bool, width int) string {
	caret := skin.Fg(m.pal.Dim).Render("› ")
	if focused {
		caret = skin.Fg(m.pal.Accent).Render("› ")
	}
	return skin.Cell(caret+in.View(), width)
}

// inputBox is a text input in a hairline box, accented when focused.
func (m Model) inputBox(in textinput.Model, focused bool, width int) string {
	border := m.pal.Line
	if focused {
		border = m.pal.Accent
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).Width(width).Render(skin.Cell(in.View(), width-2))
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
			lines[3] = ok.Render("✓ ") + skin.Fg(p.Muted).Render(fmt.Sprintf("Claude Code lists %d other models as claude-fable-5-dd-<name reversed>", len(r.Disguised)))
		case r.Counts.Other+r.Counts.Unknown > 0:
			lines[3] = ok.Render("✓ ") + skin.Fg(p.Muted).Render("Claude Code lists only the Claude models; it drops other ids")
		default:
			lines[3] = ok.Render("✓ ") + skin.Fg(p.Muted).Render("Claude Code lists the proxy's models")
		}
		lines[5] = p.Label("Models · " + r.Counts.String())
	}
	idW, ownerW := width*4/10, 12
	for _, pm := range r.Models {
		var who string
		switch class := models.Classify(pm.ID, pm.OwnedBy, px.AllowModels); {
		case class == models.Anthropic:
			who = skin.Fg(p.Accent).Render("Claude Code only")
		case !models.Chat(pm.ID):
			who = skin.Fg(p.Dim).Render("not a chat model")
		case class == models.Other:
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
