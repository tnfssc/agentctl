package demo

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func statuses(m Model) []status {
	out := make([]status, len(m.upstreams))
	for i, u := range m.upstreams {
		out[i] = u.status
	}
	return out
}

func TestScriptRunsToReady(t *testing.T) {
	m := New(1)
	for i, u := range m.upstreams {
		m = update(t, m, connectMsg{gen: m.gen, i: i})
		m = update(t, m, handshakeMsg{gen: m.gen, i: i})
		if u.flaky {
			if got := m.upstreams[i].status; got != retrying {
				t.Fatalf("%s after first handshake: status %v, want retrying", u.name, got)
			}
			m = update(t, m, connectMsg{gen: m.gen, i: i})
			m = update(t, m, handshakeMsg{gen: m.gen, i: i})
		}
		if got := m.upstreams[i].status; got != ready {
			t.Fatalf("%s: status %v, want ready", u.name, got)
		}
	}
	if !m.done() {
		t.Fatalf("done() = false with statuses %v", statuses(m))
	}
}

func TestReplayIgnoresStaleTimers(t *testing.T) {
	m := New(1)
	stale := m.gen
	m = update(t, m, connectMsg{gen: stale, i: 0})
	m = update(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})

	if m.gen == stale {
		t.Fatal("replay did not start a new generation")
	}
	m = update(t, m, handshakeMsg{gen: stale, i: 0})
	if got := m.upstreams[0].status; got != queued {
		t.Fatalf("stale handshake changed status to %v", got)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: tea.KeyEscape},
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		_, cmd := New(1).Update(k)
		if cmd == nil {
			t.Fatalf("%q: no command", k.String())
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%q: command did not quit", k.String())
		}
	}
}

func TestViewFitsTerminal(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 50, Height: 30}} {
		m := update(t, New(1), size)
		v := m.View()
		if !v.AltScreen {
			t.Error("view should use the alternate screen")
		}
		if w := lipgloss.Width(v.Content); w > size.Width {
			t.Errorf("%dx%d: content is %d cells wide", size.Width, size.Height, w)
		}
		for _, u := range script {
			if !strings.Contains(v.Content, u.name) {
				t.Errorf("%dx%d: view is missing upstream %q", size.Width, size.Height, u.name)
			}
		}
	}
}

func TestBannerRowsAreEven(t *testing.T) {
	for y, row := range bannerRows {
		if len(row) != bannerWidth {
			t.Errorf("row %d is %d cells, want %d", y, len(row), bannerWidth)
		}
	}
}
