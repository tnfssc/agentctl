package dash

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/cliproxy"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

func cpStates() map[string]cpLoadedMsg {
	running := cliproxy.Status{
		Installed: true, Version: "8.0.67", Latest: "8.0.67", Service: true, Enabled: true, Running: true,
		Host: "127.0.0.1", Port: 8317,
		APIKeys:       []string{"sk-3d1ac1ad84fe6fb8ef6dcbf9d107a1493c67f4c9abaf41df", "sk-ffff1ad84fe6fb8ef6dcbf9d107a1493c67f4c9abaf4aaaa"},
		ManagementKey: "8f2c1d9e0a7b4c3d2e1f0a9b8c7d6e5f4a3b2c1d0e9f8a7b", ManagementSet: true,
		AuthDir: "/home/me/.cli-proxy-api",
		Logins:  []cliproxy.Login{{Provider: "codex", Account: "a@example.com"}, {Provider: "xai", Account: "grok"}},
	}
	update := running
	update.Latest = "8.0.70"
	starting := running
	starting.Running = false
	stopped := starting
	stopped.Enabled = false
	everywhere := running
	everywhere.Host = ""
	noCopy := running
	noCopy.ManagementKey = ""
	claude := running
	claude.Logins = append(claude.Logins, cliproxy.Login{Provider: "claude", Account: "me@example.com"})
	offline := running
	offline.Latest, offline.LatestErr = "", errors.New(strings.Repeat("dial tcp: no route ", 8))
	return map[string]cpLoadedMsg{
		"not installed": {},
		"stopped":       {st: stopped},
		"starting":      {st: starting},
		"running":       {st: running},
		"update":        {st: update},
		"everywhere":    {st: everywhere},
		"no mgmt copy":  {st: noCopy},
		"claude login":  {st: claude},
		"offline":       {st: offline},
		"error":         {err: errors.New(strings.Repeat("config.yaml: yaml: line 3\n", 5))},
	}
}

func TestCLIProxyHomeHasNoLayoutShift(t *testing.T) {
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		m = focusOn(m, "cliproxy")
		m.page = pageDetail
		states := []state{{"checking", m}}
		for name, msg := range cpStates() {
			states = append(states, state{name, send(t, m, msg)})
		}
		sameRects(t, states, "nav:t3", "nav:opencode", "nav:cliproxy", "nav:behavior", "nav:proxy:spark", "nav:add",
			"fact:0", "fact:1", "fact:2", "fact:3", "fact:4", "status", "hints")
		for _, slot := range [][]string{{"cp-install", "cp-start", "cp-keys"}, {"cp-login"}, {"cp-server"}} {
			var want *rect
			for _, s := range states {
				for _, id := range slot {
					got, ok := rectsOf(s.m)[id]
					if !ok {
						continue
					}
					if want == nil {
						want = &got
					} else if got != *want {
						t.Errorf("%s: %q at %+v in %q, slot elsewhere %+v", sizeName(m), id, got, s.name, *want)
					}
				}
			}
			if want == nil {
				t.Errorf("%s: no state shows %v", sizeName(m), slot)
			}
		}
	}
}

func TestCLIProxyRowOpensItsPrimaryAction(t *testing.T) {
	msgs := cpStates()
	m := focusOn(send(t, base(t, 120, 40), msgs["not installed"]), "cliproxy")
	m = send(t, m, keyPress("enter"))
	if m.dialog != pairFormDialog || m.svc != svcCLIProxy || m.inputs[0].Value() != "8317" || m.inputs[1].Value() != "cliproxy" {
		t.Fatalf("enter on a missing server opens the install form: dialog %v svc %q", m.dialog, m.svc)
	}
	m.inputs[1].SetValue("local")
	if m = send(t, m, keyPress("enter"), keyPress("enter")); m.busy != "" || !strings.Contains(m.formErr, "reserved") {
		t.Fatalf("bad name: busy %q err %q", m.busy, m.formErr)
	}
	if m = send(t, m, keyPress("esc")); m.dialog != noDialog {
		t.Fatalf("esc closes the install form: dialog %v", m.dialog)
	}

	m = send(t, m, msgs["stopped"], keyPress("enter"))
	if m.busy != "Starting CLIProxyAPI" {
		t.Fatalf("enter on a stopped server starts it, busy %q", m.busy)
	}
	m = send(t, m, cpDoneMsg{what: "CLIProxyAPI is running"}, msgs["running"], keyPress("enter"))
	if m.dialog != cpKeysDialog {
		t.Fatalf("enter on a running server shows its keys: dialog %v", m.dialog)
	}
}

func TestCLIProxyKeysDialog(t *testing.T) {
	st := cpStates()["running"].st
	m := focusOn(send(t, base(t, 120, 40), cpStates()["running"]), "cliproxy")
	m = send(t, m, click("cp-keys"))
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{st.APIKeys[0], st.ManagementKey, "API key 1 of 2 · ←→ more", "/management.html"} {
		if !strings.Contains(view, want) {
			t.Fatalf("keys dialog missing %q:\n%s", want, view)
		}
	}
	if m.View().MouseMode != 0 {
		t.Fatal("the keys dialog leaves the mouse to the terminal so the keys can be selected")
	}
	m = send(t, m, keyPress("right"))
	if !strings.Contains(ansi.Strip(m.View().Content), st.APIKeys[1]) {
		t.Fatal("→ shows the second key")
	}
	m = send(t, m, keyPress("m"))
	if m.toast != "Copied the management key" {
		t.Fatalf("toast = %q", m.toast)
	}
	if m = send(t, m, keyPress("enter")); m.dialog != noDialog {
		t.Fatal("enter closes")
	}
}

// On a phone the dialog is too short for both keys: one at a time, never
// cut, with ←→ reaching the management key too.
func TestCLIProxyKeysPageOnSmallScreens(t *testing.T) {
	st := cpStates()["running"].st
	m := focusOn(send(t, base(t, 40, 16), cpStates()["running"]), "cliproxy")
	m.page = pageDetail
	m = send(t, m, keyPress("1"))
	if m.dialog != cpKeysDialog || !m.cpKeysPaged() {
		t.Fatalf("dialog %v paged %v", m.dialog, m.cpKeysPaged())
	}
	flat := func() string { return strings.ReplaceAll(ansi.Strip(m.View().Content), "\n", "") }
	seen := map[string]bool{}
	for range m.cpPages() {
		v := strings.NewReplacer(" ", "", "│", "").Replace(flat())
		for _, k := range append(slices.Clone(st.APIKeys), st.ManagementKey) {
			if strings.Contains(v, k) {
				seen[k] = true
			}
		}
		m = send(t, m, keyPress("right"))
	}
	if len(seen) != 3 {
		t.Fatalf("every key should be shown whole on some page: %v", seen)
	}
}

func TestCLIProxyServerMenuAndLogins(t *testing.T) {
	m := focusOn(send(t, base(t, 120, 40), cpStates()["running"]), "cliproxy")
	m = send(t, m, click("cp-server"))
	if m.dialog != serviceDialog || m.cpMenu != cpMenuServer || m.options[3].value != "cp-listen-all" {
		t.Fatalf("server menu: %v %q %v", m.dialog, m.cpMenu, m.options)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "CLIPROXYAPI · SERVER") {
		t.Fatal("the menu should be CLIProxyAPI's")
	}
	m = send(t, m, click("m:opt:7"))
	if m.dialog != askDialog || m.ask.title != "CLIProxyAPI" || m.ask.button != "Stop" {
		t.Fatalf("stop asks first: %+v", m.ask)
	}
	m = send(t, m, keyPress("n"))

	m = send(t, m, click("cp-login"))
	if m.cpMenu != cpMenuLogin || len(m.options) != len(cliproxy.Providers) {
		t.Fatalf("login menu: %q %v", m.cpMenu, m.options)
	}
	last := len(m.options) - 1
	if m.options[last].value != "cp-login:claude" || !strings.Contains(m.options[last].detail, "against Anthropic's terms") {
		t.Fatalf("Claude comes last, marked: %+v", m.options[last])
	}
	m = send(t, m, click("m:opt:"+strconv.Itoa(last)))
	if m.dialog != askDialog || m.ask.button != "Sign in anyway" || m.ask.body != cliproxy.ClaudeRisk {
		t.Fatalf("Claude asks first: %+v", m.ask)
	}
	if m = send(t, m, keyPress("n")); m.dialog != serviceDialog {
		t.Fatalf("no goes back to the list: %v", m.dialog)
	}

	m = send(t, m, keyPress("esc"), click("cp-server"), click("m:opt:5"))
	if m.cpMenu != cpMenuRmKey || len(m.options) != 2 {
		t.Fatalf("remove key list: %v", m.options)
	}
	m = send(t, m, click("m:opt:1"))
	if m.dialog != askDialog || !strings.Contains(m.ask.heading, "sk-…aaaa") {
		t.Fatalf("remove asks with the masked key: %+v", m.ask)
	}
	if m = send(t, m, keyPress("esc")); m.dialog != serviceDialog {
		t.Fatalf("cancel goes back to the key list: %v", m.dialog)
	}
}

func TestCLIProxyUninstallRefusedWhileInUse(t *testing.T) {
	m := send(t, base(t, 120, 40), cpStates()["running"])
	m.cfg.CLIProxy.Proxy = "spark"
	inUse := loaded()
	inUse.cfg = m.cfg
	inUse.statuses[1].status.Route = "spark"
	m = focusOn(send(t, m, inUse), "cliproxy")
	m = send(t, m, click("cp-server"), click("m:opt:8"))
	if m.dialog != noDialog || !m.toastErr || !strings.Contains(m.toast, "routes through spark") {
		t.Fatalf("dialog %v toast %q", m.dialog, m.toast)
	}
}

// The Claude warning is read in full before saying yes, on a phone too.
func TestClaudeRiskFitsTheAskDialog(t *testing.T) {
	for _, sz := range sizes {
		m := focusOn(send(t, base(t, sz[0], sz[1]), cpStates()["running"]), "cliproxy")
		m = send(t, m, click("cp-login:claude"))
		b := m.dialogBody()
		if n := len(skin.Clip(m.ask.body, b.W, 1000)); n > b.H {
			t.Errorf("%s: the warning needs %d rows, the dialog has %d", sizeName(m), n, b.H)
		}
	}
}

func TestCLIProxyWarnings(t *testing.T) {
	if w := cpWarning(cpStates()["claude login"].st); !strings.Contains(w, "Claude") {
		t.Fatalf("warning = %q", w)
	}
	if w := cpWarning(cpStates()["no mgmt copy"].st); !strings.Contains(w, "Rotate") {
		t.Fatalf("warning = %q", w)
	}
	if w := cpWarning(cpStates()["running"].st); w != "" {
		t.Fatalf("warning = %q", w)
	}
}

// Installing attaches a proxy; the reload that follows must check it rather
// than leave it "checking" forever.
func TestReloadProbesNewProxies(t *testing.T) {
	m := send(t, base(t, 120, 40), probedMsg{name: "spark", probe: live(false)})
	again := loaded()
	again.cfg.Proxies["cliproxy"] = config.Proxy{URL: "http://127.0.0.1:8317", Key: "sk-x"}
	next, cmd := m.Update(again)
	m = next.(Model)
	if _, ok := m.probes["spark"]; !ok {
		t.Fatal("an unchanged proxy keeps its result")
	}
	if _, ok := m.probes["cliproxy"]; ok || cmd == nil || m.proxyState("cliproxy") != "checking" {
		t.Fatal("a new proxy is checked")
	}
}
