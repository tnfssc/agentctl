package dash

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/ocweb"
	"github.com/tnfssc/agentctl/internal/upgrade"
)

// withUpdates loads every tool's version: Claude Code and agentctl have
// newer releases, Codex is current, Pi is current but has extensions, and
// OpenCode and T3 Code aren't installed.
func withUpdates(t *testing.T, m Model) Model {
	t.Helper()
	cli := func(name, version, latest string) upgrade.Status {
		c, _ := upgrade.Find(name)
		return upgrade.Status{CLI: c, Path: "/usr/local/bin/" + name, Version: version, Latest: latest}
	}
	m.version = "v1.4.0"
	return send(t, m,
		clisMsg{[]upgrade.Status{cli("claude", "2.1.310", "2.1.320"), cli("codex", "0.164.0", "0.164.0"), cli("pi", "1.4.0", "1.4.0")}},
		ocLoadedMsg{st: ocweb.Status{}}, t3States()["not installed"], selfCheckedMsg{latest: "v1.5.0"})
}

func TestAgentPagesOfferTheirUpdate(t *testing.T) {
	m := withUpdates(t, base(t, 120, 40))
	claude := focusOn(m, "agent:claude").detail(m.pal)
	if claude.buttons[2].id != "upd:claude" || !strings.Contains(claude.facts[3], "2.1.310 → 2.1.320") {
		t.Fatalf("claude %+v", claude)
	}
	if codex := focusOn(m, "agent:codex").detail(m.pal); codex.buttons[2].id != "" || !strings.Contains(codex.facts[3], "newest") {
		t.Fatalf("a current CLI has no Update button: %+v", codex)
	}
	// Pi's update also covers its extensions, so it's always offered.
	if pi := focusOn(m, "agent:pi").detail(m.pal); pi.buttons[2].id != "upd:pi" {
		t.Fatalf("pi %+v", pi.buttons)
	}
	if d := focusOn(m, "settings:claude").detail(m.pal); d.buttons[0].id != "tune:claude" {
		t.Fatalf("Tune lives under Settings now: %+v", d.buttons)
	}
}

func TestUpdateAllRunsOneAfterAnother(t *testing.T) {
	m := focusOn(withUpdates(t, base(t, 120, 40)), "updates")
	if e := m.updatesEntry(m.pal); e.detail != "2 updates" {
		t.Fatalf("Claude Code and agentctl have news: %q", e.detail)
	}
	m = send(t, m, keyPress("enter"))
	if m.dialog != askDialog || !slices.Equal(m.updQueue, []string{"claude", "pi", "agentctl"}) {
		t.Fatalf("Update all asks first, for every tool worth running: dialog %v queue %v", m.dialog, m.updQueue)
	}
	m = send(t, m, keyPress("y"))
	if m.busy != "Updating Claude Code (1 of 3)" {
		t.Fatalf("busy %q", m.busy)
	}
	m = send(t, m, updStepMsg{name: "claude", title: "Claude Code"})
	if m.busy != "Updating Pi and its extensions (2 of 3)" || m.dialog != askDialog {
		t.Fatalf("the next tool starts at once: busy %q", m.busy)
	}
	m = send(t, m, updStepMsg{name: "pi", title: "Pi and its extensions", err: errors.New("pi update: EACCES")})
	m = send(t, m, updStepMsg{name: "agentctl", title: "agentctl"})
	if m.busy != "" || m.dialog != noDialog || !m.toastErr || !strings.Contains(m.toast, "EACCES") || m.selfUpdated != "v1.5.0" {
		t.Fatalf("done: busy %q toast %q selfUpdated %q", m.busy, m.toast, m.selfUpdated)
	}
	if m.clisLoaded {
		t.Fatal("versions are checked again afterwards")
	}
}
