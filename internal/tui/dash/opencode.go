package dash

import (
	"context"
	"io"
	"net/url"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/ocweb"
	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// OpenCode's web server on this machine: a row under Servers, a detail pane,
// a server menu and the pairing form. The ask, server, pair form and
// pairing dialogs are shared with T3 Code; m.svc says whose they are.

const svcOpenCode = "opencode"

type (
	ocLoadedMsg struct {
		st  ocweb.Status
		err error
	}
	ocDoneMsg struct {
		what string // toast on success
		err  error
	}
)

func inspectOC(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		st, err := ocweb.Inspect(ctx)
		return ocLoadedMsg{st: st, err: err}
	}
}

// ocStep runs an opencode step. Its output is dropped; failures carry
// opencode's message. Steps that start the server return once it answers.
func ocStep(ctx context.Context, done string, step func(context.Context, io.Writer) error) tea.Cmd {
	return func() tea.Msg { return ocDoneMsg{what: done, err: step(ctx, io.Discard)} }
}

// ocPair mints a link and remembers base as the link address when it
// differs from where the server listens. An empty base means the OpenTunnel
// address, and forgets any address set before.
func ocPair(ctx context.Context, base, listen string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return pairedMsg{err: err}
		}
		want := base
		if base == listen {
			want = ""
		}
		if cfg.OpenCode.URL != want {
			cfg.OpenCode.URL = want
			if err := cfg.Save(); err != nil {
				return pairedMsg{err: err}
			}
		}
		pair := func() (ocweb.Pairing, error) { return ocweb.Pair(ctx, base) }
		if base == "" {
			pair = func() (ocweb.Pairing, error) { return ocweb.PairRemote(ctx) }
		}
		p, err := pair()
		return pairedMsg{pairing: t3.Pairing(p), err: err}
	}
}

func (m Model) onOC(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ocLoadedMsg:
		m.oc, m.ocErr, m.ocLoaded = msg.st, msg.err, true
	case ocDoneMsg:
		m.busy = ""
		if msg.err != nil {
			return m.close(msg.err.Error(), true), inspectOC(m.ctx)
		}
		return m.close(msg.what, false), inspectOC(m.ctx)
	}
	return m, nil
}

// ocState is the sidebar's one-word state for OpenCode's server.
func (m Model) ocState() string {
	st := m.oc
	switch {
	case !m.ocLoaded:
		return "checking"
	case m.ocErr != nil:
		return "error"
	case !st.Installed():
		return "not installed"
	case !st.Supported():
		return "needs v2"
	case !st.Running:
		return "stopped"
	case st.UpdateAvailable():
		return "update"
	case st.Serving != "":
		return "restart"
	}
	return "running"
}

// ocNavDetail is the sidebar's line for the server: its state, and the port
// it listens on while it runs (the whole address doesn't fit).
func (m Model) ocNavDetail() string {
	state := m.ocState()
	if u, err := url.Parse(m.oc.URL); err == nil && m.oc.Running && m.ocErr == nil && u.Port() != "" {
		return state + " · :" + u.Port()
	}
	return state
}

func (m Model) ocLinkBase() string { return ocweb.LinkBase(m.cfg.OpenCode.URL, m.oc) }

func (m Model) ocDetail(p skin.Palette) detailView {
	st := m.oc
	d := detailView{label: "Server · OpenCode web"}
	switch {
	case !m.ocLoaded:
		d.heading = "Checking…"
		return d
	case m.ocErr != nil:
		d.heading = "Can't read OpenCode's state"
		d.body = m.ocErr.Error()
		return d
	case !st.Installed():
		d.heading = "Not installed"
		d.body = "OpenCode v2 runs a background server that serves its web app. Install OpenCode to use it from " +
			"a browser or phone."
		return d
	case !st.Supported():
		d.heading = "OpenCode " + st.Version + " has no web server"
		d.body = "The background server and its web app came with OpenCode v2. Upgrade OpenCode to use them."
		return d
	case !st.Running:
		d.heading = "Stopped"
		d.body = "Any opencode command starts the server, but it is not a system service: after a reboot it stays " +
			"down until something starts it."
		d.buttons = [3]button{{"Start", "oc-start", true}, {}, ocServerButton}
	default:
		d.heading = "Running"
		d.body = "Open OpenCode's web app on other devices with one-time links. A paired device stays signed in " +
			"for 30 days."
		d.buttons = [3]button{{"Pair device", "oc-pair", true}, {}, ocServerButton}
	}
	if st.UpdateAvailable() {
		d.buttons[1] = button{"Update", "oc-update", false}
	}
	d.facts[0] = fact(p, "version", st.Version)
	switch {
	case st.UpdateAvailable():
		d.facts[1] = fact(p, "update", st.Latest)
	case st.LatestErr != nil:
		d.facts[1] = fact(p, "update", "can't check: "+st.LatestErr.Error())
	case st.OnLatest():
		d.facts[1] = fact(p, "update", "on the newest release")
	}
	if st.Serving != "" {
		d.facts[2] = fact(p, "serving", st.Serving+" · restart to run "+st.Version)
	}
	if st.URL != "" {
		listen := st.URL
		if st.Remote {
			listen += " · internet on (OpenTunnel)"
		}
		d.facts[3] = fact(p, "listen", listen)
	}
	switch base := m.ocLinkBase(); {
	case base != "":
		where := base
		if t3.LocalOnly(base) {
			where += " (this machine only)"
		}
		d.facts[4] = fact(p, "pair at", where)
	case st.Remote:
		d.facts[4] = fact(p, "pair at", "the OpenTunnel address")
	}
	return d
}

var ocServerButton = button{"Server", "oc-server", false}

// ocServerOptions are the server menu's actions, in order.
func (m Model) ocServerOptions() []option {
	remote := option{label: "Internet access", detail: "off · turn on OpenTunnel", value: "oc-remote"}
	if m.oc.Remote {
		remote.detail = "on over OpenTunnel · turn off"
	}
	return []option{
		{label: "Restart", detail: "stops running agent turns", value: "oc-restart"},
		{label: "Update", detail: "opencode upgrade, then restart", value: "oc-update"},
		remote,
		{label: "Sign out devices", detail: "every paired browser and app", value: "oc-signout"},
		{label: "Stop", detail: "until an opencode command starts it", value: "oc-stop"},
	}
}

// actOC handles the OpenCode buttons on the home screen.
func (m Model) actOC(verb string) (tea.Model, tea.Cmd) {
	m.svc = svcOpenCode
	switch verb {
	case "oc-start":
		m.busy = "Starting OpenCode"
		return m, ocStep(m.ctx, "OpenCode's server is running", ocweb.Start)
	case "oc-restart":
		return m.askOC(askSpec{
			heading: "Restart OpenCode's server?", body: "Running agent turns stop.",
			button: "Restart", busy: "Restarting OpenCode", run: ocStep(m.ctx, "OpenCode's server restarted", ocweb.Restart),
		}), nil
	case "oc-update":
		heading := "Update OpenCode to " + m.oc.Latest + "?"
		if m.oc.OnLatest() {
			heading = "OpenCode is on the newest release. Update anyway?"
		} else if m.oc.Latest == "" {
			heading = "Update OpenCode?"
		}
		return m.askOC(askSpec{
			heading: heading,
			body: "Runs `opencode upgrade` the way OpenCode was installed, then restarts the server on the new " +
				"version, which stops running agent turns.",
			button: "Update", busy: "Updating OpenCode", run: ocStep(m.ctx, "OpenCode updated", ocweb.Update),
		}), nil
	case "oc-signout":
		return m.askOC(askSpec{
			heading: "Sign out every paired device?",
			body: "OpenCode keeps no list of devices, so this gives the server a new password, which ends every " +
				"paired session; each device needs a new link. The server restarts, which stops running agent turns.",
			button: "Sign out", busy: "Signing out devices", run: ocStep(m.ctx, "Signed out every paired device", ocweb.SignOutAll),
		}), nil
	case "oc-stop":
		return m.askOC(askSpec{
			heading: "Stop OpenCode's server?",
			body:    "Running agent turns stop, and the web app goes offline until an opencode command or Start runs it again.",
			button:  "Stop", busy: "Stopping OpenCode", run: ocStep(m.ctx, "OpenCode's server stopped", ocweb.Stop),
		}), nil
	case "oc-remote":
		if m.oc.Remote {
			return m.askOC(askSpec{
				heading: "Turn off internet access?",
				body: "The OpenTunnel address, and links made for it, stop working. The server restarts, which " +
					"stops running agent turns.",
				button: "Turn off", busy: "Turning off internet access",
				run: ocStep(m.ctx, "Internet access is off", func(ctx context.Context, out io.Writer) error {
					return ocweb.SetRemote(ctx, out, false)
				}),
			}), nil
		}
		return m.askOC(askSpec{
			heading: "Reach OpenCode from the internet?",
			body: ocweb.RemoteWarning + " Pairing links then use the OpenTunnel address unless you set one. The " +
				"server restarts; the first time, setting up the tunnel can take a minute.",
			button: "Turn on", busy: "Turning on internet access",
			run: ocStep(m.ctx, "Internet access is on; pair a device to use it", func(ctx context.Context, out io.Writer) error {
				return ocweb.SetRemote(ctx, out, true)
			}),
		}), nil
	case "oc-server":
		m.options, m.pick, m.top = m.ocServerOptions(), 0, 0
		m.dialog = serviceDialog
		return m, nil
	case "oc-pair":
		return m.openOCPairForm()
	}
	return m, nil
}

func (m Model) askOC(a askSpec) Model {
	a.title = "OpenCode web"
	m.ask, m.dialog = a, askDialog
	return m
}

func (m Model) openOCPairForm() (tea.Model, tea.Cmd) {
	in := textinput.New()
	in.Placeholder = "https://this-machine.your-tailnet.ts.net"
	if m.oc.Remote {
		in.Placeholder = "empty for the OpenTunnel address"
	}
	in.Prompt = ""
	in.SetStyles(textinput.DefaultStyles(m.pal.IsDark))
	in.SetValue(m.ocLinkBase())
	m.inputs = []textinput.Model{in}
	m.field, m.formErr = 0, ""
	m.dialog = pairFormDialog
	m.sizeDialog()
	return m, m.focusInput(0)
}

func (m Model) submitOCPair() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.inputs[0].Value()) == "" && m.oc.Remote {
		m.formErr = ""
		m.busy = "Making a pairing link"
		return m, ocPair(m.ctx, "", m.oc.URL)
	}
	base, err := t3.NormalizeURL(m.inputs[0].Value())
	if err != nil {
		m.formErr = err.Error()
		return m, m.focusInput(0)
	}
	m.formErr = ""
	m.busy = "Making a pairing link"
	return m, ocPair(m.ctx, base, m.oc.URL)
}
