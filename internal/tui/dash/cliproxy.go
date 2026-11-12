package dash

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/cliproxy"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// The CLIProxyAPI server agentctl runs on this machine: a row under
// Servers, a detail pane, an install form, a keys dialog, a login picker
// and a server menu. The ask, server-menu and form dialogs are shared with
// T3 Code and OpenCode; m.svc says whose they are and m.cpMenu which list
// the menu shows.

const svcCLIProxy = "cliproxy"

// Lists the shared service dialog shows for CLIProxyAPI.
const (
	cpMenuServer = "server"
	cpMenuLogin  = "login"
	cpMenuRmKey  = "rmkey"
)

type (
	cpLoadedMsg struct {
		st  cliproxy.Status
		err error
	}
	cpLoginMsg struct{ name string } // signing in was confirmed
	cpDoneMsg  struct {
		what     string // toast on success
		err      error
		showKeys bool // open the keys dialog afterwards, on the status read after the step
		pickLast bool // and show the newest API key
		st       cliproxy.Status
	}
)

func inspectCP(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		st, err := cliproxy.Inspect(ctx, true)
		return cpLoadedMsg{st: st, err: err}
	}
}

// cpStep runs a server step; its progress output is dropped.
func cpStep(ctx context.Context, done string, step func(context.Context, io.Writer) error) tea.Cmd {
	return func() tea.Msg { return cpDoneMsg{what: done, err: step(ctx, io.Discard)} }
}

func (m Model) onCP(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cpLoginMsg:
		m.busy = ""
		return m.cpLogin(msg.name)
	case cpLoadedMsg:
		m.cp, m.cpErr, m.cpLoaded = msg.st, msg.err, true
		m.keyPick = min(m.keyPick, max(0, len(m.cp.APIKeys)-1))
	case cpDoneMsg:
		m.busy = ""
		// Installs and uninstalls change proxies, so the config is read again.
		next := tea.Batch(inspectCP(m.ctx), load)
		if msg.err != nil {
			return m.close(msg.err.Error(), true), next
		}
		if msg.showKeys {
			latest, latestErr := m.cp.Latest, m.cp.LatestErr
			m.cp, m.cpErr, m.cpLoaded = msg.st, nil, true
			m.cp.Latest, m.cp.LatestErr = latest, latestErr
			m.keyPick = 0
			if msg.pickLast {
				m.keyPick = max(0, len(m.cp.APIKeys)-1)
			}
			m.toast, m.toastErr = msg.what, false
			m.dialog = cpKeysDialog
			return m, next
		}
		return m.close(msg.what, false), next
	}
	return m, nil
}

// cpState is the sidebar's one-word state.
func (m Model) cpState() string {
	st := m.cp
	switch {
	case !m.cpLoaded:
		return "checking"
	case m.cpErr != nil:
		return "error"
	case !st.Installed:
		return "not installed"
	case !st.Running && st.Enabled:
		return "starting"
	case !st.Running:
		return "stopped"
	case st.UpdateAvailable():
		return "update"
	}
	return "running"
}

func (m Model) cpNavDetail() string {
	state := m.cpState()
	if m.cpLoaded && m.cpErr == nil && m.cp.Running {
		return state + " · :" + strconv.Itoa(m.cp.Port)
	}
	return state
}

// Each button keeps its slot in every state: the primary action first, then
// Log in and Server.
var (
	cpLoginButton  = button{"Log in", "cp-login", false}
	cpServerButton = button{"Server", "cp-server", false}
)

func (m Model) cpDetail(p skin.Palette) detailView {
	st := m.cp
	d := detailView{label: "Server · CLIProxyAPI"}
	switch {
	case !m.cpLoaded:
		d.heading = "Checking…"
		return d
	case m.cpErr != nil:
		d.heading = "Can't read CLIProxyAPI's state"
		d.body = m.cpErr.Error()
		return d
	case !st.Installed:
		d.heading = "Not installed"
		d.body = "Run CLIProxyAPI here: agentctl installs it from GitHub, runs it at login, gives it fresh API and " +
			"management keys and attaches it as a proxy."
		d.buttons = [3]button{{"Install", "cp-install", true}}
		return d
	case !st.Running && st.Enabled:
		d.heading = "Starting, or failing to"
		d.body = "The service runs but the server doesn't answer yet. Its log is " + tildePath(cpLog()) + "."
		d.buttons = [3]button{{"Keys", "cp-keys", true}, cpLoginButton, cpServerButton}
	case !st.Running:
		d.heading = "Stopped"
		d.body = "Agents routed through it fail until it starts. It stays stopped after a reboot until started. " +
			"Server › Show keys has its keys."
		d.buttons = [3]button{{"Start", "cp-start", true}, cpLoginButton, cpServerButton}
	default:
		d.heading = "Running"
		d.body = "Add accounts with Log in; agents route through it as proxy " + m.cpProxyName() + ". Keys shows the " +
			"API key and the management key."
		d.buttons = [3]button{{"Keys", "cp-keys", true}, cpLoginButton, cpServerButton}
	}
	ver := st.Version
	switch {
	case st.UpdateAvailable():
		ver += " · update " + st.Latest
	case st.LatestErr != nil:
		ver += " · can't check for updates"
	case st.OnLatest():
		ver += " · newest"
	}
	d.facts[0] = fact(p, "version", ver)
	listen := st.Host + ":" + strconv.Itoa(st.Port) + " · this machine only"
	if st.AllInterfaces() {
		listen = "every interface · port " + strconv.Itoa(st.Port)
	}
	d.facts[1] = fact(p, "listen", listen)
	d.facts[2] = fact(p, "logins", cpLoginsText(st.Logins))
	d.facts[3] = fact(p, "panel", st.PanelURL())
	if w := cpWarning(st); w != "" {
		d.facts[4] = skin.Fg(p.Amber).Render("! " + w)
	} else {
		d.facts[4] = fact(p, "keys", strconv.Itoa(len(st.APIKeys))+" API · management "+map[bool]string{true: "kept", false: "unknown"}[st.ManagementKey != ""])
	}
	return d
}

func cpLog() string {
	p, _ := cliproxy.LogPath()
	return p
}

func cpLoginsText(ls []cliproxy.Login) string {
	if len(ls) == 0 {
		return "none yet · Log in adds one"
	}
	var names []string
	for _, l := range ls {
		if len(names) == 0 || names[len(names)-1] != l.Provider {
			names = append(names, l.Provider)
		}
	}
	return strconv.Itoa(len(ls)) + " · " + strings.Join(names, ", ")
}

// cpWarning is the most pressing problem, if any, in one line.
func cpWarning(st cliproxy.Status) string {
	for _, l := range st.Logins {
		if l.Provider == "claude" {
			return "holds a Claude login: against Anthropic's terms"
		}
	}
	switch {
	case st.NoLinger:
		return "stops at logout: run loginctl enable-linger"
	case st.ManagementKey == "":
		return "management key unknown: Server › Rotate"
	}
	return ""
}

func (m Model) cpProxyName() string {
	if n := m.cfg.CLIProxy.Proxy; n != "" {
		return n
	}
	return cliproxy.DefaultProxy
}

func (m Model) cpServerOptions() []option {
	listen := option{label: "Listen on every interface", detail: "anyone on your networks can try the keys", value: "cp-listen-all"}
	if m.cp.AllInterfaces() {
		listen = option{label: "Listen on this machine only", detail: "127.0.0.1; reach it through a tunnel", value: "cp-listen-local"}
	}
	return []option{
		{label: "Show keys", detail: "API keys and the management key", value: "cp-keys"},
		{label: "Restart", detail: "requests in flight fail", value: "cp-restart"},
		{label: "Update", detail: "newest GitHub release, then restart", value: "cp-update"},
		listen,
		{label: "New API key", detail: "for another client", value: "cp-addkey"},
		{label: "Remove an API key", detail: "clients using it stop working", value: "cp-rmkeys"},
		{label: "Rotate management key", detail: "the old one stops working", value: "cp-rotate"},
		{label: "Stop", detail: "until started again, also after a reboot", value: "cp-stop"},
		{label: "Uninstall", detail: "keeps the config and keys", value: "cp-uninstall"},
		{label: "Uninstall everything", detail: "also the config, keys and proxy", value: "cp-uninstall-all"},
	}
}

func cpLoginOptions() []option {
	out := make([]option, len(cliproxy.Providers))
	for i, p := range cliproxy.Providers {
		out[i] = option{label: p.Title, detail: p.Note, value: "cp-login:" + p.Name}
	}
	return out
}

func (m Model) cpKeyOptions() []option {
	out := make([]option, len(m.cp.APIKeys))
	for i, k := range m.cp.APIKeys {
		detail := ""
		if p, ok := m.cfg.Proxies[m.cfg.CLIProxy.Proxy]; ok && p.Key == k {
			detail = "proxy " + m.cfg.CLIProxy.Proxy + " uses it"
		}
		out[i] = option{label: cliproxy.Mask(k), detail: detail, value: "cp-rmkey:" + k}
	}
	return out
}

func (m Model) openCPMenu(menu string, opts []option) (tea.Model, tea.Cmd) {
	m.svc, m.cpMenu = svcCLIProxy, menu
	m.options, m.pick, m.top = opts, 0, 0
	m.dialog = serviceDialog
	return m, nil
}

func (m Model) askCP(a askSpec) Model {
	a.title = cliproxy.Title
	m.svc = svcCLIProxy
	m.ask, m.dialog = a, askDialog
	return m
}

// actCP handles CLIProxyAPI's buttons and menu entries.
func (m Model) actCP(verb, arg string) (tea.Model, tea.Cmd) {
	m.svc = svcCLIProxy
	switch verb {
	case "cp-install":
		return m.openCPInstallForm()
	case "cp-start":
		m.busy = "Starting CLIProxyAPI"
		return m, cpStep(m.ctx, "CLIProxyAPI is running", cliproxy.Start)
	case "cp-keys":
		if !m.cp.Installed {
			return m, nil
		}
		m.keyPick = 0
		m.dialog = cpKeysDialog
		return m, nil
	case "cp-server":
		return m.openCPMenu(cpMenuServer, m.cpServerOptions())
	case "cp-login":
		if arg == "" {
			return m.openCPMenu(cpMenuLogin, cpLoginOptions())
		}
		if p, ok := cliproxy.FindProvider(arg); ok && p.Risk != "" {
			return m.askCP(askSpec{
				heading: "Sign in to " + p.Title + " anyway?", body: p.Risk,
				button: "Sign in anyway", back: serviceDialog,
				run: func() tea.Msg { return cpLoginMsg{name: arg} },
			}), nil
		}
		return m.cpLogin(arg)
	case "cp-rmkeys":
		return m.openCPMenu(cpMenuRmKey, m.cpKeyOptions())
	case "cp-rmkey":
		if len(m.cp.APIKeys) < 2 {
			return m.close("That is the only API key; add another first", true), nil
		}
		return m.askCP(askSpec{
			heading: "Remove API key " + cliproxy.Mask(arg) + "?",
			body: "Clients that send it stop working. If proxy " + m.cpProxyName() + " uses it, agentctl moves it to " +
				"another key.",
			button: "Remove", busy: "Removing the key", back: serviceDialog,
			run: cpStep(m.ctx, "Removed the API key", func(context.Context, io.Writer) error { return cliproxy.RemoveKey(arg) }),
		}), nil
	case "cp-addkey":
		m.busy = "Adding a key"
		return m, func() tea.Msg {
			_, err := cliproxy.AddKey()
			return keysShown(m.ctx, cpDoneMsg{what: "Added an API key", err: err, pickLast: true})
		}
	case "cp-restart":
		return m.askCP(askSpec{
			heading: "Restart CLIProxyAPI?", body: "Requests in flight fail.",
			button: "Restart", busy: "Restarting CLIProxyAPI", run: cpStep(m.ctx, "CLIProxyAPI restarted", cliproxy.Restart),
		}), nil
	case "cp-update":
		heading := "Update CLIProxyAPI to " + m.cp.Latest + "?"
		if m.cp.OnLatest() {
			heading = "CLIProxyAPI is on the newest release. Install it again?"
		} else if m.cp.Latest == "" {
			heading = "Update CLIProxyAPI?"
		}
		return m.askCP(askSpec{
			heading: heading,
			body:    "Downloads the newest release from GitHub, checks it, and restarts the server on it; requests in flight fail.",
			button:  "Update", busy: "Updating CLIProxyAPI",
			run: cpStep(m.ctx, "CLIProxyAPI updated", func(ctx context.Context, out io.Writer) error {
				_, err := cliproxy.Update(ctx, out)
				return err
			}),
		}), nil
	case "cp-listen-all", "cp-listen-local":
		all := verb == "cp-listen-all"
		heading, body, done := "Listen on every interface?",
			"Anyone on your networks can reach the server and try its API keys. The management API stays "+
				"local-only. The server restarts.", "CLIProxyAPI listens on every interface"
		if !all {
			heading, body, done = "Listen on this machine only?",
				"Only 127.0.0.1 reaches it; other devices need a tunnel such as Tailscale Serve. The server restarts.",
				"CLIProxyAPI listens on this machine only"
		}
		return m.askCP(askSpec{
			heading: heading, body: body, button: "Change", busy: "Restarting CLIProxyAPI",
			run: cpStep(m.ctx, done, func(ctx context.Context, out io.Writer) error { return cliproxy.Listen(ctx, out, all) }),
		}), nil
	case "cp-rotate":
		return m.askCP(askSpec{
			heading: "Set a new management key?",
			body:    "The old key stops working, in the panel too. A running server restarts to apply it.",
			button:  "Rotate", busy: "Rotating the key",
			run: func() tea.Msg {
				_, err := cliproxy.RotateManagement(m.ctx, io.Discard)
				return keysShown(m.ctx, cpDoneMsg{what: "New management key", err: err})
			},
		}), nil
	case "cp-stop":
		return m.askCP(askSpec{
			heading: "Stop CLIProxyAPI?",
			body:    "Agents routed through it fail until it starts again. It stays stopped after a reboot.",
			button:  "Stop", busy: "Stopping CLIProxyAPI", run: cpStep(m.ctx, "CLIProxyAPI stopped", cliproxy.Stop),
		}), nil
	case "cp-uninstall", "cp-uninstall-all":
		all := verb == "cp-uninstall-all"
		if who := m.usedBy(m.cfg.CLIProxy.Proxy); m.cfg.CLIProxy.Proxy != "" && who != "" {
			return m.close(who+" routes through "+m.cfg.CLIProxy.Proxy+"; switch it first", true), nil
		}
		body := "The service and binary go; the config and keys stay, so installing again brings the same keys. " +
			"Logins stay in " + tildePath(m.cp.AuthDir) + "."
		if all {
			body = "The service, binary, config, API keys and management key go, and agentctl forgets proxy " +
				m.cpProxyName() + ". Logins stay in " + tildePath(m.cp.AuthDir) + "."
		}
		return m.askCP(askSpec{
			heading: "Uninstall CLIProxyAPI?", body: body, button: "Uninstall", busy: "Uninstalling CLIProxyAPI",
			run: cpStep(m.ctx, "CLIProxyAPI uninstalled", func(ctx context.Context, _ io.Writer) error {
				return cliproxy.Uninstall(ctx, all)
			}),
		}), nil
	}
	return m, nil
}

// keysShown makes a successful step open the keys dialog on fresh state.
func keysShown(ctx context.Context, msg cpDoneMsg) cpDoneMsg {
	if msg.err != nil {
		return msg
	}
	msg.st, msg.err = cliproxy.Inspect(ctx, false)
	msg.showKeys = msg.err == nil
	return msg
}

// cpMenuText is the server dialog's heading and note for the list it shows.
func (m Model) cpMenuText() (heading, note string) {
	switch m.cpMenu {
	case cpMenuLogin:
		return "Add an account", "Prints a sign-in link; over SSH, paste back the URL the browser ends on."
	case cpMenuRmKey:
		return "Remove an API key", "Clients that send the key stop working."
	}
	return "Background service", "Runs at login under " + cpManager() + "; files in " + tildePath(cpDir()) + "."
}

func cpManager() string {
	if p, _ := cliproxy.ServicePath(); strings.HasSuffix(p, ".plist") {
		return "launchd"
	}
	return "systemd --user"
}

func cpDir() string {
	d, _ := cliproxy.Dir()
	return d
}

// cpLogin hands the terminal to CLIProxyAPI's own login, which prints a
// link and may ask for the URL the browser ends on. Its last lines stay on
// screen until enter, so a failure can be read.
func (m Model) cpLogin(name string) (Model, tea.Cmd) {
	p, ok := cliproxy.FindProvider(name)
	if !ok {
		return m, nil
	}
	c, err := cliproxy.LoginCmd(m.ctx, p)
	if err != nil {
		return m.close(err.Error(), true), nil
	}
	// A trap, not an ignore, so ctrl+c still stops the login (exec resets
	// handlers, not ignores) while the shell lives on to wait for enter.
	const script = `trap : INT; "$0" "$@"; s=$?; printf '\nPress enter to go back to agentctl. '; read _; exit $s`
	wrapped := exec.CommandContext(m.ctx, "/bin/sh", append([]string{"-c", script}, c.Args...)...) //nolint:gosec // agentctl's own install, fixed flags
	wrapped.Dir = c.Dir
	m = m.close("", false)
	handedOver.Store(true)
	return m, tea.ExecProcess(wrapped, func(err error) tea.Msg {
		handedOver.Store(false)
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return cpDoneMsg{err: errors.New(p.Title + " login didn't finish")}
			}
			return cpDoneMsg{err: err}
		}
		return cpDoneMsg{what: p.Title + " login saved"}
	})
}

// handedOver is set while a login has the terminal.
var handedOver atomic.Bool

// HandedOver reports whether the dashboard has handed the terminal to
// another program, which gets ctrl+c as SIGINT.
func HandedOver() bool { return handedOver.Load() }

// The install form: port and proxy name, both prefilled.

func (m Model) openCPInstallForm() (tea.Model, tea.Cmd) {
	values := []string{strconv.Itoa(cliproxy.DefaultPort), cliproxy.DefaultProxy}
	m.inputs = make([]textinput.Model, len(values))
	for i, v := range values {
		in := textinput.New()
		in.Prompt = ""
		in.SetStyles(textinput.DefaultStyles(m.pal.IsDark))
		in.SetValue(v)
		m.inputs[i] = in
	}
	m.svc = svcCLIProxy
	m.field, m.formErr = 0, ""
	m.dialog = pairFormDialog
	m.sizeDialog()
	return m, m.focusInput(0)
}

func (m Model) submitCPInstall() (tea.Model, tea.Cmd) {
	port, err := strconv.Atoi(strings.TrimSpace(m.inputs[0].Value()))
	if err != nil || port < 1 || port > 65535 {
		m.formErr = "port must be a number from 1 to 65535"
		return m, m.focusInput(0)
	}
	name := strings.TrimSpace(m.inputs[1].Value())
	if err := config.ValidName(name); err != nil {
		m.formErr = err.Error()
		return m, m.focusInput(1)
	}
	if p, ok := m.cfg.Proxies[name]; ok && p.URL != "http://127.0.0.1:"+strconv.Itoa(port) {
		m.formErr = "proxy " + name + " already points at " + p.URL
		return m, m.focusInput(1)
	}
	m.formErr = ""
	m.busy = "Installing CLIProxyAPI"
	opts := cliproxy.InstallOptions{Port: port, Proxy: name}
	return m, func() tea.Msg {
		err := cliproxy.Install(m.ctx, io.Discard, opts)
		return keysShown(m.ctx, cpDoneMsg{what: "CLIProxyAPI is running as proxy " + name, err: err})
	}
}

func (m Model) drawCPInstallForm(sc *scene) {
	m.panel(sc, "CLIProxyAPI · install", "Install and run it here", nil,
		&button{"Cancel", "m:cancel", false}, &button{"Install", "m:submit-pair", true})
	m.drawFields(sc, []string{"Port", "Proxy"}, []string{"it listens on 127.0.0.1", "the name agents route through"})
}

// The keys dialog. Like the pairing dialog it leaves the mouse to the
// terminal (see View), so the keys can be selected and copied with the
// terminal's own tools; a and m copy through the clipboard too.

// cpKeyRows is how many rows each key wraps over, reserved even when it fits
// on one.
const cpKeyRows = 2

func (m Model) drawCPKeys(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	m.panel(sc, "CLIProxyAPI · keys", "API and management keys",
		[]button{{"Copy API", "m:cp-copy-api", false}, {"Copy mgmt", "m:cp-copy-mgmt", false}}, nil,
		&button{"Done", "m:close-btn", true})
	y := b.Y
	line := func(s string) {
		if y < b.Y+b.H {
			sc.add(skin.Cell(s, b.W), b.X, y, 53, "")
		}
		y++
	}
	value := func(v string) {
		rows := wrapRunes(v, b.W, cpKeyRows)
		for i := range cpKeyRows {
			s := ""
			if i < len(rows) {
				s = skin.Fg(p.Text).Render(rows[i])
			}
			line(s)
		}
	}
	mgmt := func() {
		line(skin.Fg(p.Dim).Render(m.cpPageLabel("Management key")))
		if m.cp.ManagementKey != "" {
			value(m.cp.ManagementKey)
			return
		}
		line(skin.Fg(p.Amber).Render("Unknown: CLIProxyAPI keeps only a hash."))
		line(skin.Fg(p.Muted).Render("Server › Rotate management key sets one."))
	}
	api := func() {
		label := "API key"
		if n := len(m.cp.APIKeys); n > 1 {
			label += " " + strconv.Itoa(m.cpAPIPick()+1) + " of " + strconv.Itoa(n)
		}
		line(skin.Fg(p.Dim).Render(m.cpPageLabel(label)))
		if i := m.cpAPIPick(); i < len(m.cp.APIKeys) {
			value(m.cp.APIKeys[i])
		} else {
			value("")
		}
	}
	switch {
	case !m.cpKeysPaged():
		api()
		mgmt()
		line("")
	case m.keyPick >= len(m.cp.APIKeys):
		mgmt()
	default:
		api()
	}
	line(fact(p, "url", m.cp.URL()))
	line(fact(p, "panel", m.cp.PanelURL()))
	line(skin.Fg(p.Muted).Render("Select a key, or a / m to copy"))
}

// cpKeysRows is the keys dialog body with both keys shown: two labeled keys,
// a blank, url, panel and the note.
const cpKeysRows = 2*(1+cpKeyRows) + 4

// cpKeysPaged reports whether the keys dialog is too short for both keys, so
// it shows one at a time: each API key, then the management key.
func (m Model) cpKeysPaged() bool { return m.dialogBody().H < cpKeysRows }

// cpPages is how many keys ←→ steps through.
func (m Model) cpPages() int {
	if m.cpKeysPaged() {
		return len(m.cp.APIKeys) + 1
	}
	return len(m.cp.APIKeys)
}

// cpAPIPick is the API key shown, or copied by a.
func (m Model) cpAPIPick() int { return max(0, min(m.keyPick, len(m.cp.APIKeys)-1)) }

// cpPageLabel adds the ←→ hint to a key's label when there is more to see.
func (m Model) cpPageLabel(label string) string {
	if m.cpPages() > 1 {
		return label + " · ←→ more"
	}
	return label
}

// wrapRunes cuts s into at most n rows of width w, ending a cut one in …
func wrapRunes(s string, w, n int) []string {
	w = max(1, w)
	r := []rune(s)
	var rows []string
	for len(r) > 0 && len(rows) < n {
		k := min(w, len(r))
		rows = append(rows, string(r[:k]))
		r = r[k:]
	}
	if len(r) > 0 {
		last := []rune(rows[len(rows)-1])
		rows[len(rows)-1] = string(last[:len(last)-1]) + "…"
	}
	return rows
}

func (m Model) cpCopy(what string) (tea.Model, tea.Cmd) {
	var v, name string
	switch what {
	case "api":
		if len(m.cp.APIKeys) > 0 {
			v, name = m.cp.APIKeys[m.cpAPIPick()], "API key"
		}
	case "mgmt":
		v, name = m.cp.ManagementKey, "management key"
	}
	if v == "" {
		return m, nil
	}
	m.toast, m.toastErr = "Copied the "+name, false
	return m, tea.SetClipboard(v)
}

func (m Model) onCPKeysKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "a", "c":
		return m.cpCopy("api")
	case "m":
		return m.cpCopy("mgmt")
	case "left", "h", "shift+tab":
		if n := m.cpPages(); n > 0 {
			m.keyPick = (m.keyPick - 1 + n) % n
		}
	case "right", "l", "tab":
		if n := m.cpPages(); n > 0 {
			m.keyPick = (m.keyPick + 1) % n
		}
	case "enter", "esc", "q":
		return m.close("", false), nil
	}
	return m, nil
}
