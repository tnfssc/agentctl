package dash

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/tune"
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

func click(id string) pointerMsg { return pointerMsg{id: id, kind: pointerClick} }

func loaded() loadedMsg {
	cfg := &config.Config{Proxies: map[string]config.Proxy{"spark": {URL: "https://spark.example:8445"}}}
	msg := loadedMsg{cfg: cfg, st: &config.State{}}
	for _, a := range app.Agents {
		msg.statuses = append(msg.statuses, agentStatus{agent: a, status: app.Status{Route: "local"}})
	}
	return msg
}

var served = []proxy.Model{{ID: "claude-opus-5-5", OwnedBy: "anthropic"}, {ID: "gpt-6.1-sol", OwnedBy: "openai"}}

func TestCodexModelPickerHidesClaude(t *testing.T) {
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 100, Height: 30}, loaded())
	m = send(t, m, click("switch:codex"))
	if m.dialog != routeDialog || m.req.Agent != "codex" {
		t.Fatalf("dialog %v agent %q", m.dialog, m.req.Agent)
	}
	m = send(t, m, click("m:opt:1"))
	if m.busy == "" {
		t.Fatal("choosing a proxy should load its models")
	}
	m = send(t, m, modelsMsg{route: "spark", list: served})
	if m.dialog != modelDialog {
		t.Fatalf("dialog = %v", m.dialog)
	}
	for _, o := range m.options {
		if strings.Contains(o.value, "claude") {
			t.Fatalf("Codex was offered %q", o.value)
		}
	}
}

func TestClaudeRouteSkipsModelPicker(t *testing.T) {
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 100, Height: 30}, loaded(),
		keyPress("enter"), keyPress("down"), keyPress("enter"))
	if m.dialog != ackDialog {
		t.Fatalf("Claude proxy route must go straight to the policy warning, dialog = %v", m.dialog)
	}
	if m.req.Model != "" {
		t.Fatalf("route change must not pin a model, got %q", m.req.Model)
	}
	m = send(t, m, keyPress("n"))
	if m.dialog != noDialog {
		t.Fatalf("declining should close, dialog = %v", m.dialog)
	}
	m.st.ClaudeProxyAcknowledged = true
	m = send(t, m, keyPress("enter"), keyPress("down"), keyPress("enter"))
	if m.dialog != routeDialog || m.busy != "Planning" {
		t.Fatalf("acknowledged route should plan without a model picker, dialog %v busy %q", m.dialog, m.busy)
	}
}

func TestClickOutsideDialogCloses(t *testing.T) {
	m := send(t, New(context.Background(), Options{}), loaded(), click("agent:claude"))
	if m.dialog != routeDialog {
		t.Fatalf("dialog = %v", m.dialog)
	}
	if m = send(t, m, click("proxy:spark")); m.dialog != noDialog {
		t.Fatal("clicking the dimmed page behind a dialog should close it")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 60}} {
		m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: size[0], Height: size[1]}, loaded())
		content := m.View().Content
		if w, h := lipgloss.Size(content); w > size[0] || h > size[1] {
			t.Errorf("%v: view is %dx%d", size, w, h)
		}
		if !strings.Contains(content, "Claude Code") {
			t.Errorf("%v: home is missing agents", size)
		}
	}
}

func TestRemoveRefusedWhileInUse(t *testing.T) {
	msg := loaded()
	msg.statuses[1].status.Route = "spark"
	m := send(t, New(context.Background(), Options{}), msg, click("rm:spark"))
	if m.dialog != removeDialog || m.usedBy("spark") != "Codex" {
		t.Fatalf("dialog %v usedBy %q", m.dialog, m.usedBy("spark"))
	}
	m = send(t, m, keyPress("y"))
	if m.busy != "" || m.dialog != noDialog {
		t.Fatal("removing a proxy in use must not start")
	}
}

func TestAddFormValidatesBeforeChecking(t *testing.T) {
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 100, Height: 40}, loaded(), click("add"))
	if m.dialog != addDialog {
		t.Fatalf("dialog = %v", m.dialog)
	}
	m = send(t, m, click("m:submit"))
	if m.busy != "" || m.formErr == "" {
		t.Fatalf("empty form must not be checked: busy %q err %q", m.busy, m.formErr)
	}
	// Name and URL are enough; the key is optional.
	m.inputs[0].SetValue("home")
	m.inputs[1].SetValue("http://127.0.0.1:8317")
	if m = send(t, m, click("m:submit")); m.busy == "" || m.formErr != "" {
		t.Fatalf("a form without a key should be checked: busy %q err %q", m.busy, m.formErr)
	}
}

// TestMouseHitsElements drives OnMouse with real click coordinates, so the
// layout and the hit testing stay in agreement.
func TestMouseHitsElements(t *testing.T) {
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 40}, loaded())
	v := m.View()
	found := map[string]bool{}
	for y := range 40 {
		for x := range 120 {
			cmd := v.OnMouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
			if cmd == nil {
				continue
			}
			if p, ok := cmd().(pointerMsg); ok && p.id != "" {
				found[p.id] = true
			}
		}
	}
	for _, id := range []string{"nav:agent:claude", "nav:agent:codex", "nav:proxy:spark", "nav:add", "switch:claude"} {
		if !found[id] {
			t.Errorf("no screen cell resolves to %q", id)
		}
	}
	// Clicking the Codex switch button opens its route dialog.
	if m = send(t, m, click("switch:codex")); m.dialog != routeDialog {
		t.Fatalf("dialog = %v", m.dialog)
	}
}

func TestSidebarClickSelectsThenOpens(t *testing.T) {
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 40}, loaded())
	m = send(t, m, click("nav:proxy:spark"))
	if m.dialog != noDialog || m.homeIDs()[m.focus] != "proxy:spark" {
		t.Fatalf("first click should select: focus %d dialog %v", m.focus, m.dialog)
	}
	if m = send(t, m, click("nav:proxy:spark")); m.dialog != proxyDialog {
		t.Fatalf("second click should open details, dialog = %v", m.dialog)
	}
}

// TestIdleDashboardStopsTicking guards SSH performance: once nothing is
// loading, frames stop and the dashboard sends nothing.
func TestIdleDashboardStopsTicking(t *testing.T) {
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 40})
	if !m.ticking {
		t.Fatal("loading should animate")
	}
	msg := loaded()
	msg.cfg.Proxies = map[string]config.Proxy{}
	m = send(t, m, msg)
	next, cmd := m.Update(frameMsg{})
	if next.(Model).ticking || cmd != nil {
		t.Fatal("an idle dashboard must not schedule frames")
	}
}

func TestRemoteSessionsSkipHover(t *testing.T) {
	m := send(t, New(context.Background(), Options{Remote: true}), tea.WindowSizeMsg{Width: 120, Height: 40}, loaded())
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("over SSH, only clicks and wheel should be reported")
	}
}

func TestTuneDialog(t *testing.T) {
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 40}, loaded(), click("tune:codex"))
	if m.busy == "" {
		t.Fatal("tune should read settings first")
	}
	items := []tune.Status{
		{Item: tune.Item{ID: "analytics", Title: "Turn off analytics", Recommended: true}},
		{Item: tune.Item{ID: "no-approvals", Title: "Never ask"}, Applied: true},
	}
	m = send(t, m, tuneLoadedMsg{agent: "codex", items: items})
	if m.dialog != tuneDialog || !m.tuneWant["analytics"] || !m.tuneWant["no-approvals"] {
		t.Fatalf("dialog %v want %v", m.dialog, m.tuneWant)
	}
	if !strings.Contains(m.View().Content, "Reset to default") {
		t.Fatal("dialog needs a reset button")
	}
	m = send(t, m, click("m:opt:1"))
	if m.tuneWant["no-approvals"] || m.dialog != tuneDialog {
		t.Fatal("clicking a row should untick it and keep the dialog open")
	}
	if m = send(t, m, click("m:tune-reset")); m.busy != "Planning" {
		t.Fatal("reset should plan")
	}
}

// idx is the focus index of a sidebar id.
func idx(m Model, id string) int {
	i := slices.Index(m.homeIDs(), id)
	if i < 0 {
		panic("no sidebar item " + id)
	}
	return i
}

func TestSigningShowsTheCommandAndCopiesOnlyLocally(t *testing.T) {
	m := base(t, 120, 40)
	m.focus = idx(m, "git")
	if d := m.gitDetail(m.pal); d.buttons[1].id != "signing" {
		t.Fatalf("git pane buttons = %+v", d.buttons)
	}
	open := send(t, m, click("signing"))
	if open.dialog != signingDialog || !strings.Contains(open.View().Content, "agentctl guide signing") {
		t.Fatal("the signing dialog should show the command to paste into an agent")
	}
	if _, ok := rectsOf(open)["m:copy-guide"]; !ok {
		t.Fatal("local sessions get a copy button")
	}
	m.remote = true
	remote := send(t, m, click("signing"))
	if _, ok := rectsOf(remote)["m:copy-guide"]; ok || remote.hints() != "enter done" {
		t.Fatal("over SSH there is no clipboard: no copy button, no copy hint")
	}
}

// focusOn selects a home item the way keys and clicks do, scrolling the
// list to it.
func focusOn(m Model, id string) Model {
	m.focus = idx(m, id)
	m.keepNavVisible()
	return m
}

func TestTappingMorePagesThroughTheList(t *testing.T) {
	msg := loaded()
	for i := range 6 {
		msg.cfg.Proxies[fmt.Sprintf("p%d", i)] = config.Proxy{URL: "http://x"}
	}
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 40, Height: 16}, msg)
	seen := map[string]bool{}
	for range 20 {
		m = send(t, m, click("more"))
		seen[m.homeIDs()[m.focus]] = true
		if _, ok := rectsOf(m)["nav:"+m.homeIDs()[m.focus]]; !ok {
			t.Fatalf("tapping more should bring %q into view", m.homeIDs()[m.focus])
		}
	}
	if !seen["proxy:p5"] || !seen["agent:claude"] {
		t.Fatalf("more should reach the end and wrap to the top: %v", seen)
	}
}
