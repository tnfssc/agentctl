// Package dash is the full-screen dashboard behind bare `agentctl`: see
// where each agent is routed and which proxies work, switch routes, add,
// test and remove proxies, and refresh Codex's model list. Everything works
// with the mouse (click, hover, wheel) and the keyboard.
package dash

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/tui/skin"
	"github.com/tnfssc/agentctl/internal/tui/theme"
)

// dialog is the modal on top of the home screen, if any.
type dialog int

const (
	noDialog      dialog = iota
	routeDialog          // pick local or a proxy for one agent
	modelDialog          // pick a model on that proxy
	ackDialog            // Claude-through-proxy policy warning
	confirmDialog        // diff of a planned change
	addDialog            // new proxy form
	proxyDialog          // one proxy's check result and models
	removeDialog         // confirm removing a proxy
)

// option is one row of a pick list.
type option struct {
	label, detail, value string
}

// fps paces spinners. Frames only tick while something is loading, so an
// idle dashboard sends nothing to the terminal (and nothing over SSH).
const fps = 10

type frameMsg struct{}

// pointerMsg is a mouse event resolved against the last rendered frame: id
// is the clickable element under the pointer, "" for none.
type pointerMsg struct {
	id    string
	kind  pointerKind
	wheel int // -1 up, +1 down
}

type pointerKind int

const (
	pointerMove pointerKind = iota
	pointerClick
	pointerWheel
)

// Options configure the dashboard.
type Options struct {
	// Remote drops hover tracking, for sessions over SSH where every pointer
	// movement would cost a round trip.
	Remote bool
}

// Model is the dashboard's Bubble Tea model.
type Model struct {
	ctx     context.Context
	remote  bool
	ticking bool
	width   int
	height  int
	pal     skin.Palette
	diff    theme.Styles
	frame   int

	busy     string
	cfg      *config.Config
	st       *config.State
	statuses []agentStatus
	probes   map[string]app.Probe // missing means still checking
	loaded   bool

	dialog  dialog
	focus   int    // selected sidebar item on the home screen
	hover   string // element under the mouse
	options []option
	pick    int
	top     int // first visible option
	req     app.Request
	plan    agent.Plan
	proxy   string // proxy shown in proxyDialog / removeDialog
	vp      viewport.Model

	inputs  []textinput.Model
	field   int    // focused input in addDialog
	formErr string // last failed check, shown in the form

	toast    string
	toastErr bool
}

// New returns the dashboard. ctx bounds all network and file work.
func New(ctx context.Context, opts Options) Model {
	m := Model{
		ctx:    ctx,
		remote: opts.Remote,
		probes: map[string]app.Probe{},
		vp:     viewport.New(),
	}
	m.setPalette(true)
	return m
}

func (m *Model) setPalette(isDark bool) {
	m.pal = skin.New(isDark)
	m.diff = theme.New(isDark)
}

// Init loads config and asks for the background color.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, load)
}

// needsFrames reports whether anything on screen is animating.
func (m Model) needsFrames() bool {
	if m.busy != "" || !m.loaded {
		return true
	}
	for _, n := range m.cfg.Names() {
		if _, ok := m.probes[n]; !ok {
			return true
		}
	}
	return false
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/fps, func(time.Time) tea.Msg { return frameMsg{} })
}

// Update handles a message, then starts the frame ticker if something began
// animating and it is not already running.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	if _, ok := msg.(frameMsg); ok {
		nm.ticking = false // the tick that delivered this frame is spent
	}
	if !nm.ticking && nm.needsFrames() {
		nm.ticking = true
		cmd = tea.Batch(cmd, tick())
	}
	return nm, cmd
}

// update handles background results, then pointer and key input for the
// home screen or the open dialog.
func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sizeDialog()
		return m, nil
	case tea.BackgroundColorMsg:
		m.setPalette(msg.IsDark())
		return m, nil
	case frameMsg:
		m.frame++
		return m, nil
	case tea.MouseMsg:
		return m, nil // resolved into pointerMsg by the view's OnMouse
	case pointerMsg:
		return m.onPointer(msg)
	case loadedMsg:
		return m.onLoaded(msg)
	case probedMsg:
		m.probes[msg.name] = msg.probe
		if m.dialog == proxyDialog && m.proxy == msg.name {
			m.setReport(m.proxyReport(msg.name))
		}
		return m, nil
	case modelsMsg:
		return m.onModels(msg)
	case plannedMsg:
		return m.onPlanned(msg)
	case appliedMsg:
		m.busy = ""
		if msg.err != nil {
			return m.close(msg.err.Error(), true), load
		}
		return m.close(fmt.Sprintf("%s now uses %s. Restart running sessions to pick it up.", title(msg.plan.Agent), describe(msg.plan.Route)), false), load
	case savedMsg:
		m.busy = ""
		if msg.err != nil {
			m.formErr = msg.err.Error()
			return m, nil
		}
		m.probes[msg.name] = msg.probe
		return m.close("Added "+msg.name, false), load
	case removedMsg:
		m.busy = ""
		delete(m.probes, msg.name)
		if msg.err != nil {
			return m.close(msg.err.Error(), true), load
		}
		return m.close("Removed "+msg.name, false), load
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.busy != "" {
			return m, nil
		}
		return m.onKey(msg)
	}
	if m.dialog == addDialog {
		return m.updateInputs(msg)
	}
	return m, nil
}

func (m Model) onLoaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.toast, m.toastErr = msg.err.Error(), true
		return m, nil
	}
	first := !m.loaded
	m.cfg, m.st, m.statuses, m.loaded = msg.cfg, msg.st, msg.statuses, true
	m.focus = min(m.focus, len(m.homeIDs())-1)
	if !first {
		return m, nil
	}
	return m, m.probeAll()
}

func (m *Model) probeAll() tea.Cmd {
	var cmds []tea.Cmd
	for _, name := range m.cfg.Names() {
		delete(m.probes, name)
		cmds = append(cmds, probe(m.ctx, name, m.cfg.Proxies[name]))
	}
	return tea.Batch(cmds...)
}

// homeIDs are the focusable home elements in keyboard order.
func (m Model) homeIDs() []string {
	if !m.loaded {
		return []string{"add"}
	}
	var ids []string
	for _, s := range m.statuses {
		ids = append(ids, "agent:"+s.agent.Name)
	}
	for _, n := range m.cfg.Names() {
		ids = append(ids, "proxy:"+n)
	}
	return append(ids, "add")
}

func (m Model) onPointer(p pointerMsg) (tea.Model, tea.Cmd) {
	switch p.kind {
	case pointerMove:
		m.hover = p.id // unchanged ids redraw nothing: the renderer diffs cells
		return m, nil
	case pointerWheel:
		switch m.dialog {
		case routeDialog, modelDialog:
			m.movePick(p.wheel)
		case confirmDialog, proxyDialog:
			if p.wheel < 0 {
				m.vp.ScrollUp(3)
			} else {
				m.vp.ScrollDown(3)
			}
		case noDialog:
			m.focus = max(0, min(len(m.homeIDs())-1, m.focus+p.wheel))
		}
		return m, nil
	}
	if m.busy != "" {
		return m, nil
	}
	// With a dialog open, a click outside it closes it, like a web modal.
	if m.dialog != noDialog && !strings.HasPrefix(p.id, "m:") {
		return m.close("", false), nil
	}
	return m.act(p.id)
}

// act performs what clicking element id (or pressing enter on it) means.
func (m Model) act(id string) (tea.Model, tea.Cmd) {
	m.toast = ""
	verb, arg, _ := strings.Cut(id, ":")
	switch verb {
	case "nav":
		// Clicking a sidebar item selects it; clicking it again opens it.
		i := slices.Index(m.homeIDs(), arg)
		if i < 0 {
			return m, nil
		}
		if i != m.focus {
			m.focus = i
			return m, nil
		}
		return m.act(arg)
	case "local":
		m.req = app.Request{Agent: arg, Route: agent.Local}
		m.busy = "Planning"
		return m, plan(m.ctx, m.cfg, m.st, m.req)
	case "agent", "switch":
		return m.openRoutes(m.statusOf(arg))
	case "proxy", "details":
		return m.openProxy(arg), nil
	case "test":
		delete(m.probes, arg)
		return m, probe(m.ctx, arg, m.cfg.Proxies[arg])
	case "rm":
		m.proxy, m.dialog = arg, removeDialog
		return m, nil
	case "add", "add-btn":
		return m.openAdd()
	case "refresh":
		m.busy = "Refreshing Codex's model list"
		return m, refresh(m.ctx, m.cfg, m.st)
	case "testall":
		return m, m.probeAll()
	case "quit":
		return m, tea.Quit
	case "m":
		return m.actDialog(arg)
	}
	return m, nil
}

func (m Model) actDialog(id string) (tea.Model, tea.Cmd) {
	verb, arg, _ := strings.Cut(id, ":")
	switch verb {
	case "close", "cancel":
		return m.close("", false), nil
	case "back":
		return m.openRoutes(m.statusOf(m.req.Agent))
	case "opt":
		i, err := strconv.Atoi(arg)
		if err != nil || i >= len(m.options) {
			return m, nil
		}
		m.pick = i
		return m.choose()
	case "ack":
		m.busy = "Planning"
		return m, plan(m.ctx, m.cfg, m.st, m.req)
	case "apply":
		m.busy = "Applying"
		return m, apply(m.plan, m.st)
	case "test":
		delete(m.probes, m.proxy)
		m.setReport(m.proxyReport(m.proxy))
		return m, probe(m.ctx, m.proxy, m.cfg.Proxies[m.proxy])
	case "rm":
		m.dialog = removeDialog
		return m, nil
	case "remove":
		if m.usedBy(m.proxy) != "" {
			return m.close("", false), nil
		}
		m.busy = "Removing " + m.proxy
		return m, removeProxy(m.proxy)
	case "input":
		i, _ := strconv.Atoi(arg)
		return m, m.focusInput(i)
	case "submit":
		return m.submit()
	}
	return m, nil
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch m.dialog {
	case noDialog:
		ids := m.homeIDs()
		switch k {
		case "q", "esc":
			return m, tea.Quit
		case "up", "k", "shift+tab":
			m.focus = (m.focus - 1 + len(ids)) % len(ids)
		case "down", "j", "tab":
			m.focus = (m.focus + 1) % len(ids)
		case "enter", "space":
			return m.act(ids[m.focus])
		case "a":
			return m.act("add")
		case "t":
			return m.act("testall")
		case "r":
			return m.act("refresh")
		case "d":
			if name, ok := strings.CutPrefix(ids[m.focus], "proxy:"); ok {
				return m.act("rm:" + name)
			}
		}
		return m, nil
	case routeDialog, modelDialog:
		switch k {
		case "up", "k":
			m.movePick(-1)
		case "down", "j":
			m.movePick(1)
		case "enter", "space":
			return m.choose()
		case "esc":
			if m.dialog == modelDialog {
				return m.actDialog("back")
			}
			return m.close("", false), nil
		}
		return m, nil
	case ackDialog, removeDialog:
		switch k {
		case "y", "enter":
			if m.dialog == ackDialog {
				return m.actDialog("ack")
			}
			return m.actDialog("remove")
		case "n", "esc":
			return m.close("", false), nil
		}
		return m, nil
	case confirmDialog, proxyDialog:
		switch {
		case k == "esc" || k == "n" || (k == "q" && m.dialog == proxyDialog):
			return m.close("", false), nil
		case (k == "y" || k == "enter") && m.dialog == confirmDialog:
			return m.actDialog("apply")
		case k == "t" && m.dialog == proxyDialog:
			return m.actDialog("test")
		case k == "d" && m.dialog == proxyDialog:
			return m.actDialog("rm")
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	case addDialog:
		switch k {
		case "esc":
			return m.close("", false), nil
		case "tab", "down":
			return m, m.focusInput((m.field + 1) % len(m.inputs))
		case "shift+tab", "up":
			return m, m.focusInput((m.field - 1 + len(m.inputs)) % len(m.inputs))
		case "enter":
			if m.field < len(m.inputs)-1 {
				return m, m.focusInput(m.field + 1)
			}
			return m.submit()
		}
		return m.updateInputs(msg)
	}
	return m, nil
}

func (m *Model) movePick(delta int) {
	m.pick = max(0, min(len(m.options)-1, m.pick+delta))
	rows := m.listRows()
	if m.pick < m.top {
		m.top = m.pick
	}
	if m.pick >= m.top+rows {
		m.top = m.pick - rows + 1
	}
}

func (m Model) openRoutes(as agentStatus) (tea.Model, tea.Cmd) {
	if len(m.cfg.Proxies) == 0 {
		m.toast, m.toastErr = "Add a proxy first", true
		return m, nil
	}
	m.req = app.Request{Agent: as.agent.Name}
	m.options = []option{{label: "Its own login", detail: "whatever " + as.agent.Title + " is logged in with", value: agent.Local}}
	m.pick, m.top = 0, 0
	for i, name := range m.cfg.Names() {
		m.options = append(m.options, option{label: "Proxy " + name, detail: m.cfg.Proxies[name].URL, value: name})
		if as.status.Route == name {
			m.pick = i + 1
		}
	}
	m.dialog = routeDialog
	return m, nil
}

func (m Model) choose() (tea.Model, tea.Cmd) {
	choice := m.options[m.pick].value
	if m.dialog == routeDialog {
		m.req.Route = choice
		if choice == agent.Local {
			m.req.Model = ""
			m.busy = "Planning"
			return m, plan(m.ctx, m.cfg, m.st, m.req)
		}
		m.busy = "Loading models from " + choice
		return m, fetchModels(m.ctx, choice, m.cfg.Proxies[choice])
	}
	m.req.Model = choice
	if app.NeedsClaudeAck(m.st, m.req) {
		m.dialog = ackDialog
		return m, nil
	}
	m.busy = "Planning"
	return m, plan(m.ctx, m.cfg, m.st, m.req)
}

func (m Model) onModels(msg modelsMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	if msg.err != nil {
		return m.close(msg.route+": "+msg.err.Error(), true), nil
	}
	allow := m.cfg.Proxies[msg.route].AllowModels
	m.options = []option{{label: "Keep the current model", detail: "or the proxy's best if it lacks it"}}
	for _, pm := range msg.list {
		class := models.Classify(pm.ID, pm.OwnedBy, allow)
		if m.req.Agent != claude.Name && !models.Usable(class) {
			continue
		}
		detail := pm.OwnedBy
		if class == models.Anthropic {
			detail += " · Claude"
		}
		m.options = append(m.options, option{label: pm.ID, detail: detail, value: pm.ID})
	}
	m.pick, m.top = 0, 0
	m.dialog = modelDialog
	return m, nil
}

func (m Model) onPlanned(msg plannedMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	switch {
	case errors.Is(msg.err, app.ErrNothingToRefresh):
		return m.close("Codex uses its own login; nothing to refresh", false), nil
	case msg.err != nil:
		return m.close(msg.err.Error(), true), nil
	case msg.plan.Empty() && msg.refresh:
		return m.close("Codex's model list is already up to date", false), nil
	case msg.plan.Empty():
		return m.close(fmt.Sprintf("%s already uses %s; nothing to change", title(msg.plan.Agent), describe(msg.plan.Route)), false), nil
	}
	m.plan = msg.plan
	m.dialog = confirmDialog
	m.sizeDialog()
	var b strings.Builder
	for _, w := range msg.plan.Warnings {
		b.WriteString(m.diff.Warning.Render(lipgloss.Wrap("! "+w, m.vp.Width(), "")) + "\n")
	}
	if len(msg.plan.Warnings) > 0 {
		b.WriteString("\n")
	}
	b.WriteString(m.diff.Diff(msg.plan.Diff()))
	m.setReport(b.String())
	return m, nil
}

func (m Model) openProxy(name string) Model {
	m.proxy, m.dialog = name, proxyDialog
	m.sizeDialog()
	m.setReport(m.proxyReport(name))
	return m
}

func (m Model) openAdd() (tea.Model, tea.Cmd) {
	placeholders := []string{"spark", "https://spark.your-tailnet.ts.net:8445", "client API key from the proxy's config"}
	m.inputs = make([]textinput.Model, len(placeholders))
	for i, ph := range placeholders {
		in := textinput.New()
		in.Placeholder = ph
		in.Prompt = ""
		in.SetStyles(textinput.DefaultStyles(m.pal.IsDark))
		if i == 2 {
			in.EchoMode = textinput.EchoPassword
		}
		m.inputs[i] = in
	}
	m.field, m.formErr = 0, ""
	m.dialog = addDialog
	m.sizeDialog()
	return m, m.focusInput(0)
}

func (m *Model) focusInput(i int) tea.Cmd {
	m.field = i
	var cmd tea.Cmd
	for j := range m.inputs {
		if j == i {
			cmd = m.inputs[j].Focus()
		} else {
			m.inputs[j].Blur()
		}
	}
	return cmd
}

func (m Model) updateInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.inputs) == 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.field], cmd = m.inputs[m.field].Update(msg)
	return m, cmd
}

// submit validates the form and checks the proxy before saving it.
func (m Model) submit() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[0].Value())
	key := strings.TrimSpace(m.inputs[2].Value())
	url, urlErr := proxy.NormalizeURL(m.inputs[1].Value())
	switch {
	case config.ValidName(name) != nil:
		m.formErr = config.ValidName(name).Error()
		return m, m.focusInput(0)
	case m.cfg.Proxies[name].URL != "":
		m.formErr = fmt.Sprintf("a proxy named %q already exists", name)
		return m, m.focusInput(0)
	case urlErr != nil:
		m.formErr = urlErr.Error()
		return m, m.focusInput(1)
	case key == "":
		m.formErr = "the API key is required"
		return m, m.focusInput(2)
	}
	m.formErr = ""
	m.busy = "Checking " + url
	return m, saveProxy(m.ctx, name, config.Proxy{URL: url, Key: key})
}

// setReport shows text in the dialog's scroll area, which shrinks to fit
// short text and scrolls long text.
func (m *Model) setReport(text string) {
	m.sizeDialog()
	m.vp.SetHeight(max(3, min(lipgloss.Height(text), m.vp.Height())))
	m.vp.SetContent(text)
	m.vp.GotoTop()
}

// close dismisses any dialog and shows an optional message.
func (m Model) close(toast string, isErr bool) Model {
	m.dialog = noDialog
	m.toast, m.toastErr = toast, isErr
	return m
}

func (m Model) statusOf(name string) agentStatus {
	for _, s := range m.statuses {
		if s.agent.Name == name {
			return s
		}
	}
	return agentStatus{}
}

// usedBy names the agent routed to a proxy, if any.
func (m Model) usedBy(name string) string {
	i := slices.IndexFunc(m.statuses, func(s agentStatus) bool { return s.status.Route == name })
	if i < 0 {
		return ""
	}
	return m.statuses[i].agent.Title
}

func title(agentName string) string {
	if a, err := app.FindAgent(agentName); err == nil {
		return a.Title
	}
	return agentName
}

func describe(route string) string {
	if route == agent.Local {
		return "its own login"
	}
	return "proxy " + route
}
