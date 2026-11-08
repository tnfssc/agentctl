package dash

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/proxy"
)

func send(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func loaded() loadedMsg {
	cfg := &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: "https://spark.example:8445"}}}
	msg := loadedMsg{cfg: cfg, st: &config.State{}}
	for _, a := range app.Agents {
		msg.statuses = append(msg.statuses, agentStatus{agent: a, status: app.Status{Route: "local"}})
	}
	return msg
}

func TestCodexModelPickerHidesClaude(t *testing.T) {
	m := send(t, New(context.Background()), tea.WindowSizeMsg{Width: 100, Height: 30}, loaded())
	m = send(t, m, keyPress("down"), keyPress("enter")) // Codex
	if m.screen != routeScreen || m.req.Agent != "codex" {
		t.Fatalf("screen %v agent %q", m.screen, m.req.Agent)
	}
	m = send(t, m, keyPress("down"), keyPress("enter")) // proxy spark
	if m.busy == "" {
		t.Fatal("choosing a proxy should load its models")
	}
	m = send(t, m, modelsMsg{route: "spark", list: []proxy.Model{
		{ID: "claude-opus-5-5", OwnedBy: "anthropic"}, {ID: "gpt-6.1-sol", OwnedBy: "openai"},
	}})
	if m.screen != modelScreen {
		t.Fatalf("screen = %v", m.screen)
	}
	for _, o := range m.options {
		if strings.Contains(o.value, "claude") {
			t.Fatalf("Codex was offered %q", o.value)
		}
	}
}

func TestClaudeModelPickerShowsEverything(t *testing.T) {
	m := send(t, New(context.Background()), loaded(), keyPress("enter"), keyPress("down"), keyPress("enter"))
	m = send(t, m, modelsMsg{route: "spark", list: []proxy.Model{
		{ID: "claude-opus-5-5", OwnedBy: "anthropic"}, {ID: "gpt-6.1-sol", OwnedBy: "openai"},
	}})
	if len(m.options) != 3 {
		t.Fatalf("want keep-current plus 2 models, got %d", len(m.options))
	}
	m = send(t, m, keyPress("down"), keyPress("enter"))
	if m.screen != ackScreen {
		t.Fatalf("first Claude proxy route must show the policy warning, screen = %v", m.screen)
	}
	m = send(t, m, keyPress("n"))
	if m.screen != homeScreen {
		t.Fatalf("declining should go home, screen = %v", m.screen)
	}
}

func TestViewFitsTerminal(t *testing.T) {
	for _, w := range []int{60, 120} {
		m := send(t, New(context.Background()), tea.WindowSizeMsg{Width: w, Height: 30}, loaded())
		if got := lipgloss.Width(m.View().Content); got > w {
			t.Errorf("width %d: view is %d cells wide", w, got)
		}
		if !strings.Contains(m.View().Content, "Claude Code") {
			t.Errorf("width %d: home is missing agents", w)
		}
	}
}

func TestRemoveRefusedWhileInUse(t *testing.T) {
	msg := loaded()
	msg.statuses[1].status.Route = "spark"
	m := send(t, New(context.Background()), msg, keyPress("down"), keyPress("down"), keyPress("d"))
	if m.screen != removeScreen || m.usedBy("spark") != "Codex" {
		t.Fatalf("screen %v usedBy %q", m.screen, m.usedBy("spark"))
	}
	m = send(t, m, keyPress("y"))
	if m.busy != "" || m.screen != homeScreen {
		t.Fatal("removing a proxy in use must not start")
	}
}
