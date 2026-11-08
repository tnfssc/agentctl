// Package dash is the full-screen dashboard behind bare `agentctl`: see
// where each agent is routed and which proxies work, switch routes, add,
// test and remove proxies, and refresh Codex's model list.
package dash

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/tui/theme"
)

type screen int

const (
	homeScreen    screen = iota
	routeScreen          // pick local or a proxy for one agent
	modelScreen          // pick a model on that proxy
	ackScreen            // Claude-through-proxy policy warning
	confirmScreen        // diff of a planned change
	addScreen            // new proxy form
	proxyScreen          // one proxy's check result and models
	removeScreen         // confirm removing a proxy
)

type formAnswers struct{ name, url, key string }

// option is one row of a pick list.
type option struct {
	label, detail, value string
}

// Model is the dashboard's Bubble Tea model.
type Model struct {
	ctx    context.Context
	width  int
	height int
	styles theme.Styles

	spinner spinner.Model
	busy    string // shown with the spinner while background work runs

	cfg      *config.Config
	st       *config.State
	statuses []agentStatus
	probes   map[string]app.Probe // missing means still checking
	loaded   bool

	screen  screen
	cursor  int // home row
	options []option
	pick    int // row in options
	req     app.Request
	plan    agent.Plan
	proxy   string // proxy shown on proxyScreen / removeScreen
	vp      viewport.Model

	form *huh.Form
	// answers outlives the value copies Bubble Tea makes of Model, so the
	// form's bound pointers stay valid.
	answers *formAnswers

	toast    string
	toastErr bool
}

// New returns the dashboard. ctx bounds all network and file work.
func New(ctx context.Context) Model {
	return Model{
		ctx:     ctx,
		styles:  theme.New(true),
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		probes:  map[string]app.Probe{},
		vp:      viewport.New(),
		answers: &formAnswers{},
	}
}

// Init loads config and asks the terminal for its background color.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.spinner.Tick, load)
}

var (
	keyUp    = key.NewBinding(key.WithKeys("up", "k"))
	keyDown  = key.NewBinding(key.WithKeys("down", "j"))
	keyEnter = key.NewBinding(key.WithKeys("enter"))
	keyBack  = key.NewBinding(key.WithKeys("esc"))
	keyQuit  = key.NewBinding(key.WithKeys("q"))
	keyYes   = key.NewBinding(key.WithKeys("y"))
	keyNo    = key.NewBinding(key.WithKeys("n", "esc"))
)

// Update routes messages: results of background work first, then keys for
// the current screen. The add-proxy form gets every message while it is up.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.SetWidth(m.contentWidth())
		m.vp.SetHeight(max(3, m.height-10))
		if m.form != nil {
			m.form = m.form.WithWidth(m.contentWidth())
		}
		return m, nil
	case tea.BackgroundColorMsg:
		m.styles = theme.New(msg.IsDark())
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	if m.screen == addScreen {
		return m.updateForm(msg)
	}
	switch msg := msg.(type) {
	case loadedMsg:
		return m.onLoaded(msg)
	case probedMsg:
		m.probes[msg.name] = msg.probe
		if m.screen == proxyScreen && m.proxy == msg.name {
			m.vp.SetContent(m.proxyReport(msg.name))
		}
		return m, nil
	case modelsMsg:
		return m.onModels(msg)
	case plannedMsg:
		return m.onPlanned(msg)
	case appliedMsg:
		m.busy = ""
		if msg.err != nil {
			return m.home(msg.err.Error(), true), load
		}
		return m.home(fmt.Sprintf("%s now uses %s. Restart running sessions to pick it up.", title(msg.plan.Agent), describe(msg.plan.Route)), false), load
	case removedMsg:
		m.busy = ""
		delete(m.probes, msg.name)
		if msg.err != nil {
			return m.home(msg.err.Error(), true), load
		}
		return m.home("Removed "+msg.name, false), load
	case tea.KeyPressMsg:
		if m.busy != "" {
			return m, nil
		}
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onLoaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.home(msg.err.Error(), true), nil
	}
	first := !m.loaded
	m.cfg, m.st, m.statuses, m.loaded = msg.cfg, msg.st, msg.statuses, true
	m.cursor = min(m.cursor, m.rows()-1)
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

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// A message describes the last action; any new action replaces it.
	m.toast, m.toastErr = "", false
	switch m.screen {
	case homeScreen:
		return m.homeKey(msg)
	case routeScreen, modelScreen:
		return m.pickKey(msg)
	case ackScreen:
		switch {
		case key.Matches(msg, keyYes):
			m.busy = "Planning…"
			return m, plan(m.ctx, m.cfg, m.st, m.req)
		case key.Matches(msg, keyNo):
			return m.home("", false), nil
		}
	case confirmScreen:
		switch {
		case key.Matches(msg, keyYes):
			m.busy = "Applying…"
			return m, apply(m.plan, m.st)
		case key.Matches(msg, keyNo):
			return m.home("Cancelled; nothing changed", false), nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	case proxyScreen:
		switch msg.String() {
		case "esc", "q":
			return m.home("", false), nil
		case "t":
			delete(m.probes, m.proxy)
			m.vp.SetContent(m.proxyReport(m.proxy))
			return m, probe(m.ctx, m.proxy, m.cfg.Proxies[m.proxy])
		case "d":
			m.screen = removeScreen
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	case removeScreen:
		switch {
		case key.Matches(msg, keyYes) && m.usedBy(m.proxy) == "":
			m.busy = "Removing…"
			return m, removeProxy(m.proxy)
		case key.Matches(msg, keyNo), key.Matches(msg, keyYes):
			return m.home("", false), nil
		}
	}
	return m, nil
}

func (m Model) homeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if !m.loaded {
		if key.Matches(msg, keyQuit) {
			return m, tea.Quit
		}
		return m, nil
	}
	nAgents, names := len(m.statuses), m.cfg.Names()
	switch {
	case key.Matches(msg, keyQuit), key.Matches(msg, keyBack):
		return m, tea.Quit
	case key.Matches(msg, keyUp):
		m.cursor = max(0, m.cursor-1)
	case key.Matches(msg, keyDown):
		m.cursor = min(m.rows()-1, m.cursor+1)
	case msg.String() == "a":
		return m.openForm()
	case msg.String() == "t":
		m.toast = "Checking proxies…"
		m.toastErr = false
		return m, m.probeAll()
	case msg.String() == "r":
		m.busy = "Refreshing Codex's model list…"
		return m, refresh(m.ctx, m.cfg, m.st)
	case msg.String() == "d" && m.cursor >= nAgents && m.cursor < nAgents+len(names):
		m.proxy = names[m.cursor-nAgents]
		m.screen = removeScreen
	case key.Matches(msg, keyEnter):
		switch {
		case m.cursor < nAgents:
			return m.openRoutes(m.statuses[m.cursor])
		case m.cursor < nAgents+len(names):
			m.proxy = names[m.cursor-nAgents]
			m.screen = proxyScreen
			m.vp.SetContent(m.proxyReport(m.proxy))
			m.vp.GotoTop()
		default:
			return m.openForm()
		}
	}
	return m, nil
}

func (m Model) openRoutes(as agentStatus) (tea.Model, tea.Cmd) {
	if len(m.cfg.Proxies) == 0 {
		m.toast, m.toastErr = "Add a proxy first (a)", true
		return m, nil
	}
	m.req = app.Request{Agent: as.agent.Name}
	m.options = []option{{label: "Its own login", detail: "whatever " + as.agent.Title + " is logged in with", value: agent.Local}}
	m.pick = 0
	for i, name := range m.cfg.Names() {
		m.options = append(m.options, option{label: "Proxy " + name, detail: m.cfg.Proxies[name].URL, value: name})
		if as.status.Route == name {
			m.pick = i + 1
		}
	}
	m.screen = routeScreen
	return m, nil
}

func (m Model) pickKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyBack):
		if m.screen == modelScreen {
			return m.openRoutes(m.statusOf(m.req.Agent))
		}
		return m.home("", false), nil
	case key.Matches(msg, keyUp):
		m.pick = max(0, m.pick-1)
	case key.Matches(msg, keyDown):
		m.pick = min(len(m.options)-1, m.pick+1)
	case key.Matches(msg, keyEnter):
		choice := m.options[m.pick].value
		if m.screen == routeScreen {
			m.req.Route = choice
			if choice == agent.Local {
				m.req.Model = ""
				m.busy = "Planning…"
				return m, plan(m.ctx, m.cfg, m.st, m.req)
			}
			m.busy = "Loading models from " + choice + "…"
			return m, fetchModels(m.ctx, choice, m.cfg.Proxies[choice])
		}
		m.req.Model = choice
		if app.NeedsClaudeAck(m.st, m.req) {
			m.screen = ackScreen
			return m, nil
		}
		m.busy = "Planning…"
		return m, plan(m.ctx, m.cfg, m.st, m.req)
	}
	return m, nil
}

func (m Model) onModels(msg modelsMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	if msg.err != nil {
		return m.home(msg.route+": "+msg.err.Error(), true), nil
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
	m.pick = 0
	m.screen = modelScreen
	return m, nil
}

func (m Model) onPlanned(msg plannedMsg) (tea.Model, tea.Cmd) {
	m.busy = ""
	switch {
	case errors.Is(msg.err, app.ErrNothingToRefresh):
		return m.home("Codex uses its own login; nothing to refresh", false), nil
	case msg.err != nil:
		return m.home(msg.err.Error(), true), nil
	case msg.plan.Empty() && msg.refresh:
		return m.home("Codex's model list is already up to date", false), nil
	case msg.plan.Empty():
		return m.home(fmt.Sprintf("%s already uses %s; nothing to change", title(msg.plan.Agent), describe(msg.plan.Route)), false), nil
	}
	m.plan = msg.plan
	var b strings.Builder
	for _, w := range msg.plan.Warnings {
		b.WriteString(m.styles.Warning.Render(lipgloss.Wrap("! "+w, m.contentWidth(), "")) + "\n")
	}
	if len(msg.plan.Warnings) > 0 {
		b.WriteString("\n")
	}
	b.WriteString(m.styles.Diff(msg.plan.Diff()))
	m.vp.SetContent(b.String())
	m.vp.GotoTop()
	m.screen = confirmScreen
	return m, nil
}

// openForm shows the add-proxy form, keeping earlier answers after a failed
// check so the user only fixes what was wrong.
func (m Model) openForm() (tea.Model, tea.Cmd) {
	keys := huh.NewDefaultKeyMap()
	keys.Quit = key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel"))
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Name").Description("Short name, e.g. spark").Value(&m.answers.name).
			Validate(func(s string) error {
				if err := config.ValidName(s); err != nil {
					return err
				}
				if _, taken := m.cfg.Proxies[s]; taken {
					return fmt.Errorf("%q already exists", s)
				}
				return nil
			}),
		huh.NewInput().Title("URL").Description("e.g. https://spark.your-tailnet.ts.net:8445").Value(&m.answers.url).
			Validate(func(s string) error { _, err := proxy.NormalizeURL(s); return err }),
		huh.NewInput().Title("API key").Description("A client key from the proxy's config").
			EchoMode(huh.EchoModePassword).Value(&m.answers.key).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return errors.New("required")
				}
				return nil
			}),
	)).WithKeyMap(keys).WithShowHelp(false).WithWidth(m.contentWidth())
	m.screen = addScreen
	return m, m.form.Init()
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if saved, ok := msg.(savedMsg); ok {
		m.busy = ""
		if saved.err != nil {
			m.toast, m.toastErr = saved.name+": "+saved.err.Error(), true
			return m.openForm()
		}
		m.probes[saved.name] = saved.probe
		*m.answers = formAnswers{}
		m.form = nil
		return m.home("Added "+saved.name, false), load
	}
	if m.busy != "" {
		return m, nil
	}
	next, cmd := m.form.Update(msg)
	if f, ok := next.(*huh.Form); ok {
		m.form = f
	}
	switch m.form.State {
	case huh.StateAborted:
		m.form = nil
		return m.home("", false), nil
	case huh.StateCompleted:
		url, _ := proxy.NormalizeURL(m.answers.url)
		m.busy = "Checking " + url + "…"
		return m, saveProxy(m.ctx, m.answers.name, config.Proxy{URL: url, Key: strings.TrimSpace(m.answers.key)})
	}
	return m, cmd
}

// home returns to the main screen with an optional message.
func (m Model) home(toast string, isErr bool) Model {
	m.screen = homeScreen
	m.toast, m.toastErr = toast, isErr
	return m
}

// rows is the number of selectable home rows: agents, proxies, "add".
func (m Model) rows() int {
	if m.cfg == nil {
		return 1
	}
	return len(m.statuses) + len(m.cfg.Proxies) + 1
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

func (m Model) contentWidth() int { return max(20, min(100, m.width-4)) }

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
