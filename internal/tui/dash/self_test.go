package dash

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func withVersion(t *testing.T, version string, msgs ...tea.Msg) Model {
	t.Helper()
	m := send(t, New(context.Background(), Options{Version: version}), tea.WindowSizeMsg{Width: 120, Height: 40}, loaded())
	return focusOn(send(t, m, msgs...), "self")
}

func TestSelfUpdateOffersTheNewRelease(t *testing.T) {
	m := withVersion(t, "v1.4.0", selfCheckedMsg{latest: "v1.5.0"})
	d := m.selfDetail(m.pal)
	if d.buttons[0].id != "self-update" || !strings.Contains(d.heading, "v1.5.0") {
		t.Fatalf("detail %+v", d)
	}
	m = send(t, m, keyPress("enter"))
	if m.dialog != askDialog || !strings.Contains(m.ask.heading, "v1.4.0 → v1.5.0") {
		t.Fatalf("opening the row should ask first: dialog %v heading %q", m.dialog, m.ask.heading)
	}
	m = send(t, m, selfUpdatedMsg{tag: "v1.5.0", path: "/opt/bin/agentctl"})
	if m.dialog != noDialog || m.toastErr || m.selfState() != "reopen" {
		t.Fatalf("after updating: dialog %v toast %q state %q", m.dialog, m.toast, m.selfState())
	}
}

func TestSelfUpdateStates(t *testing.T) {
	for _, c := range []struct {
		name    string
		m       Model
		state   string
		primary string
	}{
		{"latest", withVersion(t, "v1.5.0", selfCheckedMsg{latest: "v1.5.0"}), "latest", "self-check"},
		{"offline", withVersion(t, "v1.5.0", selfCheckedMsg{err: errors.New("checking for a new release: offline")}), "can't check", "self-check"},
		{"checking", withVersion(t, "v1.5.0"), "checking", ""},
		{"source", withVersion(t, "dev", selfCheckedMsg{latest: "v1.5.0"}), "source build", ""},
	} {
		if got := c.m.selfState(); got != c.state {
			t.Errorf("%s: state %q, want %q", c.name, got, c.state)
		}
		var primary string
		for _, b := range c.m.selfDetail(c.m.pal).buttons {
			if b.primary {
				primary = b.id
			}
		}
		if primary != c.primary {
			t.Errorf("%s: primary button %q, want %q", c.name, primary, c.primary)
		}
	}
}

func TestSourceBuildNeverUpdates(t *testing.T) {
	m := withVersion(t, "dev", selfCheckedMsg{latest: "v9.0.0"})
	if _, cmd := m.act("self-check"); cmd != nil {
		t.Fatal("a source build must not look for releases")
	}
	if m = send(t, m, keyPress("enter")); m.dialog != noDialog {
		t.Fatal("a source build has nothing to update")
	}
}
