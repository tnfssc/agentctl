package dash

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// T3 Code on this machine: a row under Servers, a detail pane with the
// server's state, a devices dialog, a pairing form and the pairing link.

const svcT3 = "t3"

// svcTitle names the app the shared ask, server and pairing dialogs act on.
func (m Model) svcTitle() string {
	if m.svc == svcOpenCode {
		return "OpenCode web"
	}
	return "T3 Code"
}

type (
	t3LoadedMsg struct {
		st     t3.Status
		err    error
		waited bool // from waitT3: the wait is over either way
	}
	t3AccessMsg struct {
		list []t3.Access
		err  error
	}
	t3DoneMsg struct {
		what string // toast on success
		err  error
		wait bool // the server restarts; wait for it to answer
	}
	pairedMsg struct {
		pairing t3.Pairing
		err     error
	}
)

// askSpec is a yes/no question in askDialog and what yes runs.
type askSpec struct {
	title, heading, body, button string
	run                          tea.Cmd
	busy                         string
	reopen                       bool // return to the devices dialog afterwards
}

func inspectT3(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		st, err := t3.Inspect(ctx)
		return t3LoadedMsg{st: st, err: err}
	}
}

// waitT3 inspects until the server answers, for up to 20 seconds: after an
// install, update or restart, systemd and launchd return before it is up.
func waitT3(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		var msg t3LoadedMsg
		for range 20 {
			msg.st, msg.err = t3.Inspect(ctx)
			if msg.err != nil || msg.st.Running || ctx.Err() != nil {
				break
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
		msg.waited = true
		return msg
	}
}

func listAccess(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		list, err := t3.ListAccess(ctx)
		return t3AccessMsg{list: list, err: err}
	}
}

// t3Step runs a t3 setup step. Its output is dropped; failures carry t3's
// message. wait says the server restarts and should be waited for.
func t3Step(ctx context.Context, done string, wait bool, step func(context.Context, io.Writer) error) tea.Cmd {
	return func() tea.Msg { return t3DoneMsg{what: done, err: step(ctx, io.Discard), wait: wait} }
}

func revoke(ctx context.Context, list []t3.Access) tea.Cmd {
	return func() tea.Msg {
		for i, a := range list {
			if err := t3.Revoke(ctx, a); err != nil {
				if i > 0 {
					err = fmt.Errorf("revoked %d of %d, then: %w", i, len(list), err)
				}
				return t3DoneMsg{err: err}
			}
		}
		noun := "devices"
		if len(list) == 1 {
			noun = "device"
		}
		return t3DoneMsg{what: fmt.Sprintf("Revoked %d %s", len(list), noun)}
	}
}

// pair mints a link and remembers base as the link address when it differs
// from where the server listens.
func pair(ctx context.Context, label, base, origin string) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return pairedMsg{err: err}
		}
		want := base
		if base == origin {
			want = ""
		}
		if cfg.T3.URL != want {
			cfg.T3.URL = want
			if err := cfg.Save(); err != nil {
				return pairedMsg{err: err}
			}
		}
		p, err := t3.Pair(ctx, label, base, t3.DefaultTTL)
		return pairedMsg{pairing: p, err: err}
	}
}

func (m Model) onT3(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case t3LoadedMsg:
		m.t3, m.t3Err, m.t3Loaded = msg.st, msg.err, true
		if msg.waited {
			m.t3Starting = false
		}
	case t3AccessMsg:
		m.busy = ""
		if msg.err != nil {
			return m.close(msg.err.Error(), true), nil
		}
		m.access, m.svc = msg.list, svcT3
		m.options = make([]option, len(msg.list))
		m.pick, m.top = min(m.pick, max(0, len(msg.list)-1)), 0
		m.dialog = devicesDialog
		m.movePick(0)
	case t3DoneMsg:
		m.busy = ""
		if msg.err != nil {
			return m.close(msg.err.Error(), true), inspectT3(m.ctx)
		}
		if m.dialog == askDialog && m.ask.reopen {
			m.toast, m.toastErr = msg.what, false
			m.busy = "Loading devices"
			return m, listAccess(m.ctx)
		}
		if !msg.wait {
			return m.close(msg.what, false), inspectT3(m.ctx)
		}
		m.t3Starting = true
		return m.close(msg.what, false), waitT3(m.ctx)
	case pairedMsg:
		m.busy = ""
		if msg.err != nil {
			m.formErr = msg.err.Error()
			return m, nil
		}
		m.pairing = msg.pairing
		m.qr, _ = skin.QR(msg.pairing.URL)
		m.dialog = pairDialog
		// The link address may have been saved; load picks it up.
		return m, tea.Batch(tea.SetClipboard(msg.pairing.URL), load)
	}
	return m, nil
}

// t3State is the sidebar's one-word state for T3 Code.
func (m Model) t3State() string {
	st := m.t3
	switch {
	case !m.t3Loaded:
		return "checking"
	case m.t3Starting && !st.Running:
		return "starting"
	case m.t3Err != nil:
		return "error"
	case !st.Installed():
		return "not set up"
	case !st.ServiceInstalled:
		return "no service"
	case !st.Running:
		return "stopped"
	case st.UpdateAvailable():
		return "update"
	}
	return "running"
}

func (m Model) t3LinkBase() string { return t3.LinkBase(m.cfg.T3.URL, m.t3) }

func (m Model) t3Detail(p skin.Palette) detailView {
	st := m.t3
	d := detailView{label: "Server · T3 Code"}
	install := button{"Install", "t3-install", true}
	switch {
	case !m.t3Loaded:
		d.heading = "Checking…"
		return d
	case m.t3Starting && !st.Running:
		d.heading = "Starting…"
		d.body = "Waiting for the server to answer."
		return d
	case m.t3Err != nil:
		d.heading = "Can't read T3 Code's state"
		d.body = m.t3Err.Error()
		return d
	case !st.Installed():
		d.heading = "Not installed"
		d.body = "T3 Code is a web, desktop and phone app for your coding agents. agentctl installs the nightly " +
			"and runs it as a background service, so your devices and desktop-app updates reach it."
		d.buttons = [3]button{install}
		return d
	case !st.ServiceInstalled:
		d.heading = "Not running as a service"
		if st.Running {
			d.heading = "Running by hand, not as a service"
			d.buttons[0] = button{"Devices", "t3-devices", false}
		}
		d.body = "Set up the background service so T3 Code starts at boot and desktop-app updates reach it."
		d.buttons[1] = button{"Set up", "t3-setup", true}
	case !st.Running:
		d.heading = "Not answering"
		d.body = "The service is installed but the server does not answer. Restart it, or reinstall the service to repair it."
		d.buttons = [3]button{{"Devices", "t3-devices", false}, {"Restart", "t3-restart", true}, serviceButton}
	default:
		d.heading = "Running"
		if st.Label != "" {
			d.heading += " as " + st.Label
		}
		d.body = "Pair devices with one-time links. Updates started from a T3 Code desktop app reach this server too."
		d.buttons = [3]button{{"Devices", "t3-devices", true}, {}, serviceButton}
		if st.UpdateAvailable() {
			d.buttons[1] = button{"Update", "t3-update", false}
		}
	}
	if v := st.CurrentVersion(); v != "" {
		d.facts[0] = fact(p, "version", v)
	}
	switch {
	case st.UpdateAvailable():
		d.facts[1] = fact(p, "update", st.Latest)
	case st.LatestErr != nil:
		d.facts[1] = fact(p, "update", "can't check: "+st.LatestErr.Error())
	case st.OnLatest():
		d.facts[1] = fact(p, "update", "on the newest nightly")
	case st.Latest != "":
		d.facts[1] = fact(p, "nightly", st.Latest)
	}
	if st.Update != "" {
		d.facts[2] = fact(p, "last", st.Update)
	}
	if base := m.t3LinkBase(); base != "" {
		where := base
		if t3.LocalOnly(base) {
			where += " (this machine only)"
		}
		d.facts[3] = fact(p, "pair at", where)
	}
	d.facts[4] = fact(p, "data", tildePath(st.Home))
	return d
}

var serviceButton = button{"Service", "t3-service", false}

// serviceOptions are the service dialog's actions, in order.
var serviceOptions = []option{
	{label: "Restart", detail: "stops running agent turns", value: "t3-restart"},
	{label: "Repair", detail: "reinstall the service", value: "t3-repair"},
	{label: "Uninstall", detail: "projects and threads stay", value: "t3-uninstall"},
	{label: "Internet access", detail: "Cloudflare Tunnel · a guide for your agent", value: "guide:tunnel"},
}

// actT3 handles the T3 Code buttons on the home screen.
func (m Model) actT3(verb string) (tea.Model, tea.Cmd) {
	m.svc = svcT3
	switch verb {
	case "t3-install", "t3-repair", "t3-setup":
		body := "Downloads T3 Code's installer from t3.codes for the nightly channel, then installs a background " +
			"service for this user that starts at boot."
		if m.t3.Installed() {
			body = "Installs or repairs T3 Code's background service. If that changes anything, the server restarts " +
				"and running agent turns stop."
		}
		return m.askT3(askSpec{
			heading: "Set up T3 Code?", body: body, button: "Install",
			busy: "Installing T3 Code", run: t3Step(m.ctx, "T3 Code is set up", true, t3.Install),
		}), nil
	case "t3-update":
		return m.askT3(askSpec{
			heading: "Update to " + m.t3.Latest + "?",
			body:    "The service restarts on the new version, which stops running agent turns.",
			button:  "Update", busy: "Updating T3 Code", run: t3Step(m.ctx, "T3 Code updated", true, t3.Update),
		}), nil
	case "t3-restart":
		return m.askT3(askSpec{
			heading: "Restart T3 Code?", body: "Running agent turns stop.",
			button: "Restart", busy: "Restarting T3 Code", run: t3Step(m.ctx, "T3 Code restarted", true, t3.Restart),
		}), nil
	case "t3-uninstall":
		return m.askT3(askSpec{
			heading: "Uninstall T3 Code's background service?",
			body: "The server stops and no longer starts at boot, and desktop-app updates can't reach it. t3, your " +
				"projects and threads stay; Set up brings the service back.",
			button: "Uninstall", busy: "Uninstalling the service", run: t3Step(m.ctx, "Removed T3 Code's service", false, t3.Uninstall),
		}), nil
	case "t3-service":
		m.options, m.pick, m.top = serviceOptions, 0, 0
		m.dialog = serviceDialog
		return m, nil
	case "t3-devices":
		m.pick = 0
		m.busy = "Loading devices"
		return m, listAccess(m.ctx)
	}
	return m, nil
}

func (m Model) askT3(a askSpec) Model {
	a.title = "T3 Code"
	m.ask, m.dialog = a, askDialog
	return m
}

// actDevices handles the devices dialog's buttons.
func (m Model) actDevices(verb string) (tea.Model, tea.Cmd) {
	switch verb {
	case "revoke":
		if len(m.access) == 0 {
			return m, nil
		}
		a := m.access[m.pick]
		what, body := "Sign out "+orUnnamed(a.Label)+"?", revokeNote
		if a.Link {
			what, body = "Cancel this pairing link?", "Nobody has used it yet."
		}
		return m.askT3(askSpec{
			heading: what, body: body, button: "Revoke", busy: "Revoking",
			run: revoke(m.ctx, []t3.Access{a}), reopen: true,
		}), nil
	case "revoke-idle":
		idle := m.idleAccess()
		if len(idle) == 0 {
			m.toast, m.toastErr = "No idle devices", false
			return m, nil
		}
		return m.askT3(askSpec{
			heading: fmt.Sprintf("Sign out %d idle devices?", len(idle)),
			body: "Paired devices that last connected over 7 days ago. One connected the whole time looks the same. " +
				revokeNote,
			button: "Revoke", busy: "Revoking", run: revoke(m.ctx, idle), reopen: true,
		}), nil
	case "pair":
		return m.openPairForm()
	}
	return m, nil
}

const revokeNote = "A revoked device needs a new pairing link to come back. One connected right now may keep " +
	"working until it reconnects or T3 Code restarts."

func (m Model) idleAccess() []t3.Access {
	var idle []t3.Access
	now := time.Now()
	for _, a := range m.access {
		if a.Idle(now) {
			idle = append(idle, a)
		}
	}
	return idle
}

func orUnnamed(label string) string {
	if label == "" {
		return "this device"
	}
	return label
}

func (m Model) openPairForm() (tea.Model, tea.Cmd) {
	values := []string{"", m.t3LinkBase()}
	placeholders := []string{"e.g. phone", "https://this-machine.your-tailnet.ts.net"}
	m.inputs = make([]textinput.Model, len(values))
	for i := range values {
		in := textinput.New()
		in.Placeholder = placeholders[i]
		in.Prompt = ""
		in.SetStyles(textinput.DefaultStyles(m.pal.IsDark))
		in.SetValue(values[i])
		m.inputs[i] = in
	}
	m.field, m.formErr = 0, ""
	m.dialog = pairFormDialog
	m.sizeDialog()
	return m, m.focusInput(0)
}

func (m Model) submitPair() (tea.Model, tea.Cmd) {
	m.pressed = "m:submit-pair"
	if m.svc == svcOpenCode {
		return m.submitOCPair()
	}
	base, err := t3.NormalizeURL(m.inputs[1].Value())
	if err != nil {
		m.formErr = err.Error()
		return m, m.focusInput(1)
	}
	m.formErr = ""
	m.busy = "Making a pairing link"
	return m, pair(m.ctx, m.inputs[0].Value(), base, m.t3.Origin)
}

// Drawing -------------------------------------------------------------------

// devicesChrome is the devices body minus its list: intro, blank, more row,
// blank, and the selected entry's info row.
const devicesChrome = 5

func (m Model) devicesRows() int { return max(1, (m.dialogBody().H-devicesChrome)/2) }

func (m Model) drawDevices(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	idle := len(m.idleAccess())
	m.panel(sc, "T3 Code · devices", "Paired devices and open links",
		[]button{{"Revoke", "m:revoke", false}}, &button{fmt.Sprintf("Revoke idle (%d)", idle), "m:revoke-idle", false},
		&button{"Pair device", "m:pair", true})
	intro := "No devices yet. Pair one with a one-time link."
	if len(m.access) > 0 {
		intro = fmt.Sprintf("%d entries · idle: paired, last connected over 7 days ago", len(m.access))
	}
	sc.add(skin.Cell(skin.Fg(p.Muted).Render(intro), b.W), b.X, b.Y, 53, "")
	rows := m.devicesRows()
	now := time.Now()
	for r := range rows {
		i := m.top + r
		if i >= len(m.access) {
			break
		}
		a := m.access[i]
		icon, detail := "›", a.Device
		if a.Link {
			icon, detail = "◇", "pairing link"
		}
		when := "connected " + t3.Ago(now, a.LastSeen)
		if a.Link {
			when = "expires " + t3.Ago(now, a.Expires)
		}
		if detail != "" {
			when = detail + " · " + when
		}
		if a.Idle(now) {
			when += " · idle"
		}
		id := "m:opt:" + strconv.Itoa(i)
		sc.add(p.Tall(icon, orUnnamed(a.Label), when, "", b.W, i == m.pick, m.hover == id), b.X, b.Y+2+2*r, 53, id)
	}
	m.drawMore(sc, b.X, b.Y+2+2*rows, rows)
	var info string
	if m.pick < len(m.access) {
		a := m.access[m.pick]
		info = fact(p, "id", a.ID)
		if a.IP != "" {
			info += skin.Fg(p.Dim).Render(" · ") + skin.Fg(p.Muted).Render(a.IP)
		}
	}
	sc.add(skin.Cell(info, b.W), b.X, b.Y+b.H-1, 53, "m:info")
}

func (m Model) drawPairForm(sc *scene) {
	m.panel(sc, m.svcTitle()+" · pair a device", "Make a one-time pairing link", nil,
		&button{"Cancel", "m:cancel", false}, &button{"Make link", "m:submit-pair", true})
	labels, hints := []string{"Device", "Address"}, []string{"shown in the devices list", "how the new device reaches this machine"}
	if m.svc == svcOpenCode { // OpenCode doesn't name devices
		labels, hints = labels[1:], hints[1:]
	}
	m.drawFields(sc, labels, hints)
}

func (m Model) drawPairing(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	m.panel(sc, m.svcTitle()+" · pair a device", "Open this link on the new device",
		[]button{{"Copy link", "m:copy", false}}, nil, &button{"Done", "m:close-btn", true})
	// The link is a terminal hyperlink, wrapped rather than cut, and the
	// dialog leaves the mouse to the terminal (see View), so it can be
	// selected, clicked or copied from the terminal's own menu.
	link := skin.Fg(p.Text).Hyperlink(m.pairing.URL)
	rows := m.linkRows()
	for i, row := range rows {
		id := ""
		if i == 0 {
			id = "m:url"
		}
		sc.add(skin.Cell(link.Render(row), b.W), b.X, b.Y+i, 53, id)
	}
	note := "One device · expires " + t3.Ago(time.Now(), m.pairing.Expires) + " · select it, or c to copy"
	if t3.LocalOnly(m.pairing.URL) {
		note = "Only works on this machine: set the server address other devices use"
	}
	qrTop := m.qrTop()
	sc.add(skin.Cell(skin.Fg(p.Muted).Render(note), b.W), b.X, b.Y+len(rows), 53, "")
	if len(m.qr) == 0 {
		return
	}
	if m.noColor {
		// Uncolored, the code would come out inverted on dark terminals.
		sc.add(skin.Cell(skin.Fg(p.Dim).Render("This terminal has no color, so no QR code; open or paste the link."), b.W), b.X, b.Y+qrTop+1, 53, "")
		return
	}
	if len(m.qr) > b.H-qrTop {
		msg := "The link is too long for a QR code here; open or paste it."
		if bodyH := len(m.qr) + qrTop; bodyH <= maxDialogH-dialogChrome {
			msg = fmt.Sprintf("A terminal at least %d rows tall shows a QR code too.", bodyH+dialogChrome+2)
		}
		sc.add(skin.Cell(skin.Fg(p.Dim).Render(msg), b.W), b.X, b.Y+qrTop+1, 53, "")
		return
	}
	x := b.X + max(0, (b.W-lipgloss.Width(m.qr[0]))/2)
	for i, row := range m.qr {
		sc.add(row, x, b.Y+qrTop+i, 53, "")
	}
}

// maxLinkRows caps the rows a pairing link wraps over; a longer one is cut.
const maxLinkRows = 3

// linkRows is the pairing link cut into rows of the dialog's width.
func (m Model) linkRows() []string {
	w := max(1, m.dialogBody().W)
	r := []rune(m.pairing.URL)
	var rows []string
	for len(r) > 0 && len(rows) < maxLinkRows {
		n := min(w, len(r))
		rows = append(rows, string(r[:n]))
		r = r[n:]
	}
	if len(r) > 0 {
		last := []rune(rows[len(rows)-1])
		rows[len(rows)-1] = string(last[:len(last)-1]) + "…"
	}
	if len(rows) == 0 {
		rows = []string{""}
	}
	return rows
}

// qrTop is the pairing body's first QR row, under the link and its note.
func (m Model) qrTop() int { return len(m.linkRows()) + 1 }
