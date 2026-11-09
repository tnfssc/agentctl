package dash

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/t3"
)

// accessList is n entries: a pairing link, the desktop app's own session
// (old, but never idle), then paired devices, every third idle.
func accessList(n int) []t3.Access {
	now := time.Now()
	var list []t3.Access
	for i := range n {
		a := t3.Access{
			ID: fmt.Sprintf("id-%03d", i), Paired: true, Label: fmt.Sprintf("device %d", i), Device: "Chrome · macOS",
			IP: "100.64.0.1", Issued: now.Add(-30 * 24 * time.Hour), Expires: now.Add(time.Hour), LastSeen: now.Add(-time.Hour),
		}
		switch {
		case i == 0:
			a.Link, a.Paired, a.Device, a.IP = true, false, "", ""
		case i == 2:
			a.Paired, a.LastSeen = false, now.Add(-30*24*time.Hour)
		case i%3 == 0:
			a.LastSeen = now.Add(-10 * 24 * time.Hour)
		}
		list = append(list, a)
	}
	return list
}

func t3States() map[string]t3LoadedMsg {
	running := t3.Status{
		Home: "/home/u/.t3", CLI: "/home/u/.local/bin/t3", ServiceSupported: true, ServiceInstalled: true,
		ServiceVersion: "0.1.8-nightly.20261106.4262", Running: true, Version: "0.1.8-nightly.20261106.4262",
		Label: "minipc", Origin: "http://127.0.0.1:3773", Latest: "0.1.8-nightly.20261106.4262",
	}
	update := running
	update.Latest = "0.1.8-nightly.20261108.4380"
	update.Update = "0.1.7 → 0.1.8 committed"
	stopped := running
	stopped.Running, stopped.Version = false, ""
	noService := running
	noService.ServiceInstalled, noService.Running = false, false
	offline := running
	offline.Latest, offline.LatestErr = "", errors.New(strings.Repeat("dial tcp: no route ", 8))
	return map[string]t3LoadedMsg{
		"not installed": {st: t3.Status{Home: "/home/u/.t3"}},
		"no service":    {st: noService},
		"stopped":       {st: stopped},
		"running":       {st: running},
		"update":        {st: update},
		"offline":       {st: offline},
		"error":         {err: errors.New(strings.Repeat("t3 status: broken\n", 5))},
	}
}

func TestT3HomeHasNoLayoutShift(t *testing.T) {
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		m.focus = idx(m, "t3")
		starting := send(t, m, t3States()["stopped"])
		starting.t3Starting = true
		states := []state{{"checking", m}, {"starting", starting}}
		for name, msg := range t3States() {
			states = append(states, state{name, send(t, m, msg)})
			other := send(t, m, msg)
			other.focus = 0
			states = append(states, state{name + " unfocused", other})
		}
		sameRects(t, states, "nav:agent:claude", "nav:agent:codex", "nav:t3", "nav:behavior", "nav:proxy:spark", "nav:add",
			"fact:0", "fact:1", "fact:2", "fact:3", "fact:4", "status", "hints", "pill")
		for _, slot := range [][]string{{"t3-devices", "t3-install"}, {"t3-update", "t3-setup", "t3-restart"}, {"t3-service"}} {
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

func TestT3RowOpensItsPrimaryAction(t *testing.T) {
	msgs := t3States()
	m := send(t, base(t, 120, 40), msgs["not installed"])
	m.focus = idx(m, "t3")
	if m = send(t, m, keyPress("enter")); m.dialog != askDialog || m.ask.button != "Install" {
		t.Fatalf("enter on an uninstalled T3 Code should ask to install: dialog %v ask %+v", m.dialog, m.ask)
	}
	if m = send(t, m, keyPress("n")); m.dialog != noDialog {
		t.Fatal("n should cancel")
	}
	m = send(t, m, msgs["running"], keyPress("enter"))
	if m.busy != "Loading devices" {
		t.Fatalf("enter on a running server should load its devices, busy %q", m.busy)
	}
}

func TestDevicesRevokeFlow(t *testing.T) {
	m := send(t, base(t, 120, 40), t3States()["running"], t3AccessMsg{list: accessList(4)})
	if m.dialog != devicesDialog {
		t.Fatalf("dialog = %v", m.dialog)
	}
	m = send(t, m, keyPress("down"), keyPress("r"))
	if m.dialog != askDialog || !m.ask.reopen || !strings.Contains(m.ask.heading, "device 1") {
		t.Fatalf("revoke should ask about the selected device: %+v", m.ask)
	}
	if m = send(t, m, keyPress("esc")); m.dialog != devicesDialog || m.pick != 1 {
		t.Fatalf("cancel should return to the list where it was: dialog %v pick %d", m.dialog, m.pick)
	}
	m = send(t, m, keyPress("i"))
	if !strings.Contains(m.ask.heading, "1 idle") {
		t.Fatalf("idle ask = %q", m.ask.heading)
	}
	m = send(t, m, keyPress("y"))
	if m.busy != "Revoking" {
		t.Fatalf("busy = %q", m.busy)
	}
	m = send(t, m, t3DoneMsg{what: "Revoked 1 device"})
	if m.busy != "Loading devices" || m.toast != "Revoked 1 device" {
		t.Fatalf("after revoking, the list reloads: busy %q toast %q", m.busy, m.toast)
	}
	m = send(t, m, t3AccessMsg{list: accessList(3)})
	if m.dialog != devicesDialog || m.pick != 1 {
		t.Fatalf("reloaded list: dialog %v pick %d", m.dialog, m.pick)
	}
}

func TestPairFormChecksTheAddress(t *testing.T) {
	m := send(t, base(t, 120, 40), t3States()["running"], t3AccessMsg{}, keyPress("p"))
	if m.dialog != pairFormDialog || m.inputs[1].Value() != "http://127.0.0.1:3773" {
		t.Fatalf("form should prefill the listen address: dialog %v %q", m.dialog, m.inputs[1].Value())
	}
	m.inputs[1].SetValue("box.ts.net")
	m = send(t, m, click("m:submit-pair"))
	if m.busy != "" || !strings.Contains(m.formErr, "not an http(s) URL") {
		t.Fatalf("bad address: busy %q err %q", m.busy, m.formErr)
	}
	if m = send(t, m, keyPress("esc")); m.dialog != devicesDialog {
		t.Fatal("esc returns to the devices list")
	}
}

func TestPairingShowsQRWhenItFits(t *testing.T) {
	link := t3.Pairing{URL: "https://box.tail035df.ts.net/pair#token=TOKEN12CHARS", Expires: time.Now().Add(5 * time.Minute)}
	for _, c := range []struct {
		w, h int
		qr   bool
	}{{120, 40, true}, {120, 30, true}, {120, 29, false}, {80, 24, false}} {
		m := send(t, base(t, c.w, c.h), t3PairedMsg{pairing: link})
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, link.URL) {
			t.Fatalf("%dx%d: link not shown", c.w, c.h)
		}
		if got := strings.Contains(view, "█"); got != c.qr {
			t.Errorf("%dx%d: QR shown = %v, want %v", c.w, c.h, got, c.qr)
		}
		if !c.qr && !strings.Contains(view, "at least 30 rows tall") {
			t.Errorf("%dx%d: should say how tall the terminal must be", c.w, c.h)
		}
	}
}

func TestRestartWaitsForTheServer(t *testing.T) {
	msgs := t3States()
	m := send(t, base(t, 120, 40), msgs["running"])
	m.focus = idx(m, "t3")
	m = send(t, m, click("t3-service"), keyPress("enter"))
	if m.dialog != askDialog || m.ask.button != "Restart" {
		t.Fatalf("Service then enter should ask to restart: dialog %v ask %+v", m.dialog, m.ask)
	}
	m = send(t, m, keyPress("y"), t3DoneMsg{what: "T3 Code restarted", wait: true})
	if m.toast != "T3 Code restarted" || !m.t3Starting {
		t.Fatalf("after a restart: toast %q starting %v", m.toast, m.t3Starting)
	}
	// systemd returns before the server is up; the first look finds it down.
	m = send(t, m, msgs["stopped"])
	if m.t3State() != "starting" || m.t3Detail(m.pal).heading != "Starting…" {
		t.Fatalf("still starting: state %q", m.t3State())
	}
	waited := msgs["running"]
	waited.waited = true
	if m = send(t, m, waited); m.t3Starting || m.t3State() != "running" {
		t.Fatalf("once it answers: starting %v state %q", m.t3Starting, m.t3State())
	}
}

func TestNoQRWithoutColor(t *testing.T) {
	m := base(t, 120, 40)
	m.noColor = true
	m = send(t, m, t3PairedMsg{pairing: t3.Pairing{URL: "https://box.ts.net/pair#token=TOKEN12CHARS", Expires: time.Now().Add(time.Minute)}})
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "█") || !strings.Contains(view, "no color") {
		t.Fatal("without color the dialog should explain the missing QR code, not draw an inverted one")
	}
}

func TestUninstallFromServiceMenu(t *testing.T) {
	m := send(t, base(t, 120, 40), t3States()["running"])
	m.focus = idx(m, "t3")
	m = send(t, m, click("t3-service"), click("m:opt:2"))
	if m.dialog != askDialog || m.ask.button != "Uninstall" {
		t.Fatalf("dialog %v ask %+v", m.dialog, m.ask)
	}
	if m = send(t, m, keyPress("n")); m.dialog != noDialog {
		t.Fatal("n cancels")
	}
	m = send(t, m, click("t3-service"), click("m:opt:2"), keyPress("y"), t3DoneMsg{what: "Removed T3 Code's service"})
	if m.t3Starting || m.toast != "Removed T3 Code's service" {
		t.Fatalf("after uninstalling nothing restarts: starting %v toast %q", m.t3Starting, m.toast)
	}
}
