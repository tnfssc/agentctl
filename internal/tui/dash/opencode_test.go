package dash

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/ocweb"
	"github.com/tnfssc/agentctl/internal/t3"
)

func ocStates() map[string]ocLoadedMsg {
	running := ocweb.Status{CLI: "/opt/homebrew/bin/opencode", Version: "2.2.6", Running: true, URL: "http://127.0.0.1:49374"}
	running.Latest = "2.2.6"
	serving := running
	serving.Serving = "2.2.5"
	update := running
	update.Latest = "2.2.7"
	offline := running
	offline.Latest, offline.LatestErr = "", errors.New(strings.Repeat("dial tcp: no route ", 8))
	stopped := running
	stopped.Running, stopped.URL = false, ""
	return map[string]ocLoadedMsg{
		"not installed": {},
		"v1":            {st: ocweb.Status{CLI: "/usr/local/bin/opencode", Version: "1.1.4"}},
		"stopped":       {st: stopped},
		"running":       {st: running},
		"serving old":   {st: serving},
		"update":        {st: update},
		"offline":       {st: offline},
		"error":         {err: errors.New(strings.Repeat("opencode service status: broken\n", 5))},
	}
}

func TestOpenCodeHomeHasNoLayoutShift(t *testing.T) {
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		m = focusOn(m, "opencode")
		m.page = pageDetail // compact layouts show buttons on the item's page
		states := []state{{"checking", m}}
		for name, msg := range ocStates() {
			states = append(states, state{name, send(t, m, msg)})
		}
		sameRects(t, states, "nav:t3", "nav:opencode", "nav:behavior", "nav:proxy:spark", "nav:add",
			"fact:0", "fact:1", "fact:2", "fact:3", "fact:4", "status", "hints")
		for _, slot := range [][]string{{"oc-start", "oc-pair"}, {"oc-update"}, {"oc-server"}} {
			var want *rect
			for _, s := range states {
				for _, id := range slot {
					if got, ok := rectsOf(s.m)[id]; ok {
						if want == nil {
							want = &got
						} else if got != *want {
							t.Errorf("%s: %q at %+v in %q, slot elsewhere %+v", sizeName(m), id, got, s.name, *want)
						}
					}
				}
			}
			if want == nil {
				t.Errorf("%s: no state shows %v", sizeName(m), slot)
			}
		}
	}
}

func TestOpenCodeRowOpensItsPrimaryAction(t *testing.T) {
	msgs := ocStates()
	m := focusOn(send(t, base(t, 120, 40), msgs["stopped"]), "opencode")
	if m = send(t, m, keyPress("enter")); m.busy != "Starting OpenCode" {
		t.Fatalf("enter on a stopped server should start it, busy %q", m.busy)
	}
	m = send(t, m, ocDoneMsg{what: "OpenCode's server is running"}, msgs["running"], keyPress("enter"))
	if m.dialog != pairFormDialog || len(m.inputs) != 1 || m.inputs[0].Value() != "http://127.0.0.1:49374" {
		t.Fatalf("enter on a running server should open the pair form with its address: dialog %v", m.dialog)
	}
	m.inputs[0].SetValue("https://box.ts.net/web")
	if m = send(t, m, keyPress("enter")); m.busy != "" || !strings.Contains(m.formErr, "has a path") {
		t.Fatalf("bad address: busy %q err %q", m.busy, m.formErr)
	}
	if m = send(t, m, keyPress("esc")); m.dialog != noDialog {
		t.Fatalf("esc closes OpenCode's form; it has no devices list: dialog %v", m.dialog)
	}
}

func TestOpenCodeServerMenu(t *testing.T) {
	m := focusOn(send(t, base(t, 120, 40), ocStates()["running"]), "opencode")
	m = send(t, m, click("oc-server"))
	if m.dialog != serviceDialog || m.options[3].value != "oc-signout" {
		t.Fatalf("server menu: dialog %v options %v", m.dialog, m.options)
	}
	m = send(t, m, click("m:opt:3"))
	if m.dialog != askDialog || m.ask.title != "OpenCode web" || m.ask.button != "Sign out" {
		t.Fatalf("sign out should ask first: %+v", m.ask)
	}
	m = send(t, m, keyPress("y"))
	if m.busy != "Signing out devices" {
		t.Fatalf("busy = %q", m.busy)
	}
	m = send(t, m, ocDoneMsg{err: errors.New("opencode service start: boom")})
	if m.dialog != noDialog || !m.toastErr || m.busy != "" {
		t.Fatalf("a failed step closes with its error: dialog %v toast %q", m.dialog, m.toast)
	}
}

func TestOpenCodePairingClosesOnDone(t *testing.T) {
	m := focusOn(send(t, base(t, 120, 40), ocStates()["running"]), "opencode")
	link := t3.Pairing{URL: "https://box.ts.net/auth/connect/I0wuzPtW0raVKaWaAoHj_A", Expires: time.Now().Add(5 * time.Minute)}
	m = send(t, m, keyPress("enter"), pairedMsg{pairing: link})
	if m.dialog != pairDialog {
		t.Fatalf("dialog = %v", m.dialog)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "OPENCODE WEB · PAIR A DEVICE") {
		t.Fatal("the pairing dialog should be OpenCode's")
	}
	if m = send(t, m, keyPress("enter")); m.dialog != noDialog || m.busy != "" {
		t.Fatalf("done closes, without loading T3 devices: dialog %v busy %q", m.dialog, m.busy)
	}
}

func TestPairingLeavesTheMouseToTheTerminal(t *testing.T) {
	m := send(t, base(t, 40, 16), ocStates()["running"])
	if m.View().MouseMode == tea.MouseModeNone {
		t.Fatal("the dashboard should track the mouse")
	}
	long := "https://minipc.tail035df.ts.net:8443/auth/connect/I0wuzPtW0raVKaWaAoHj_A"
	m = send(t, m, pairedMsg{pairing: t3.Pairing{URL: long, Expires: time.Now().Add(5 * time.Minute)}})
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("the pairing dialog should leave the mouse to the terminal so the link can be selected")
	}
	if got := strings.Join(m.linkRows(), ""); got != long {
		t.Fatalf("the link should wrap whole on a phone, got %q", got)
	}
	if !strings.Contains(m.View().Content, "\x1b]8;") {
		t.Fatal("the link should be a terminal hyperlink")
	}
}
