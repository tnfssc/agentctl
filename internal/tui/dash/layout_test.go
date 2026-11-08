package dash

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/tune"
)

// These tests enforce the "no layout shift" rule: an element that exists in
// two states must have the same rectangle in both.

var sizes = [][2]int{{80, 24}, {100, 30}, {120, 40}, {200, 60}}

type state struct {
	name string
	m    Model
}

func rectsOf(m Model) map[string]rect { return m.scene().rects }

// sameRects fails when any id shared by two states sits elsewhere.
func sameRects(t *testing.T, states []state, ids ...string) {
	t.Helper()
	first := map[string]state{}
	for _, s := range states {
		r := rectsOf(s.m)
		for _, id := range ids {
			got, ok := r[id]
			if !ok {
				continue
			}
			if f, seen := first[id]; seen {
				if want := rectsOf(f.m)[id]; got != want {
					t.Errorf("%s: %q is %+v in %q but %+v in %q", sizeName(s.m), id, got, s.name, want, f.name)
				}
			} else {
				first[id] = s
			}
		}
	}
}

func sizeName(m Model) string { return fmt.Sprintf("%dx%d", m.width, m.height) }

func base(t *testing.T, w, h int) Model {
	t.Helper()
	return send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: w, Height: h}, loaded())
}

func live(long bool) app.Probe {
	r := app.Probe{Latency: 23 * time.Millisecond}
	for i := range 40 {
		id := fmt.Sprintf("gpt-%d", i)
		if long && i == 3 {
			id = strings.Repeat("very-long-model-name-", 4)
		}
		r.Models = append(r.Models, proxy.Model{ID: id, OwnedBy: "openai"})
	}
	return r
}

func probeStates(t *testing.T, m Model) []state {
	t.Helper()
	down := app.Probe{HealthErr: errors.New(strings.Repeat("connection refused ", 10))}
	key := app.Probe{ModelsErr: errors.New("the proxy rejected the API key")}
	warned := live(true)
	warned.Disguised = []string{"claude-fable-5-dd-x"}
	return []state{
		{"checking", m},
		{"live", send(t, m, probedMsg{name: "spark", probe: live(false)})},
		{"live long ids + warning", send(t, m, probedMsg{name: "spark", probe: warned})},
		{"down", send(t, m, probedMsg{name: "spark", probe: down})},
		{"key rejected", send(t, m, probedMsg{name: "spark", probe: key})},
	}
}

func TestHomeHasNoLayoutShift(t *testing.T) {
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		routed := loaded()
		routed.statuses[1].status = app.Status{Route: "spark", URL: "https://spark.example:8445", Model: "gpt-6.1-sol", Notes: []string{"a note"}}
		mr := send(t, m, routed)

		var states []state
		for focus := range 4 {
			for _, base := range []state{{"local", m}, {"routed", mr}} {
				s := base.m
				s.focus = focus
				for _, ps := range probeStates(t, s) {
					states = append(states, state{fmt.Sprintf("%s focus=%d %s", base.name, focus, ps.name), ps.m})
				}
			}
		}
		busy := m
		busy.busy = "Planning"
		toast := m
		toast.toast, toast.toastErr = strings.Repeat("something failed ", 20), true
		states = append(states, state{"busy", busy}, state{"toast", toast})

		sameRects(t, states,
			"pill", "status", "hints", "nav:agent:claude", "nav:agent:codex", "nav:proxy:spark", "nav:add",
			"fact:0", "fact:1", "fact:2", "fact:3", "fact:4", "add", "testall", "refresh", "quit")
		// Button slots: whatever button a slot holds, the slot never moves.
		for _, slot := range [][]string{
			{"switch:claude", "switch:codex", "test:spark", "add-btn"},
			{"local:codex", "details:spark"},
			{"tune:claude", "tune:codex", "rm:spark"},
		} {
			var placed []state
			for _, s := range states {
				r := rectsOf(s.m)
				for _, id := range slot {
					if got, ok := r[id]; ok {
						placed = append(placed, s)
						if want := rectsOf(placed[0].m); want != nil {
							for _, id0 := range slot {
								if w0, ok := want[id0]; ok && w0 != got {
									t.Errorf("%s: slot %v holds %q at %+v in %q but %q at %+v in %q",
										sizeName(s.m), slot, id, got, s.name, id0, w0, placed[0].name)
								}
							}
						}
					}
				}
			}
		}
	}
}

func TestHoverChangesNothing(t *testing.T) {
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		want := rectsOf(m)
		for id := range want {
			h := m
			h.hover = id
			for k, r := range rectsOf(h) {
				if want[k] != r {
					t.Errorf("%s: hovering %q moves %q", sizeName(m), id, k)
				}
			}
		}
	}
}

func dialogStates(t *testing.T, m Model) []state {
	t.Helper()
	var out []state
	add := func(name string, s Model) { out = append(out, state{name, s}) }

	routes := send(t, m, click("switch:codex"))
	add("route", routes)
	short := send(t, routes, click("m:opt:1"), modelsMsg{route: "spark", list: []proxy.Model{{ID: "gpt-6", OwnedBy: "openai"}}})
	add("model short", short)
	long := send(t, routes, click("m:opt:1"), modelsMsg{route: "spark", list: live(true).Models})
	add("model long", long)
	scrolled := long
	for range 30 {
		scrolled = send(t, scrolled, keyPress("down"))
	}
	add("model scrolled", scrolled)

	ack := send(t, m, click("switch:claude"), click("m:opt:1"))
	add("ack", ack)

	small := agent.Plan{Agent: "codex", Route: "spark", Changes: []agent.Change{{Path: "/x", Before: []byte("a\n"), After: []byte("b\n")}}}
	big := small
	big.Changes = []agent.Change{{Path: "/x", After: []byte(strings.Repeat("line\n", 200))}}
	big.Warnings = []string{"one", "two", "three"}
	add("confirm small", send(t, m, plannedMsg{plan: small}))
	add("confirm big + warnings", send(t, m, plannedMsg{plan: big}))

	proxyDialog := send(t, m, click("details:spark"))
	for _, ps := range probeStates(t, proxyDialog) {
		add("proxy "+ps.name, ps.m)
	}
	add("remove", send(t, m, click("rm:spark")))
	inUse := loaded()
	inUse.statuses[1].status.Route = "spark"
	add("remove in use", send(t, send(t, m, inUse), click("rm:spark")))

	form, _ := m.openAdd()
	add("add", form.(Model))
	add("add error", send(t, form.(Model), click("m:submit")))
	add("add async error", send(t, form.(Model), savedMsg{name: "x", err: errors.New(strings.Repeat("bad ", 40))}))

	items, err := tune.Inspect("claude")
	if err != nil {
		t.Fatal(err)
	}
	items[len(items)-1].Notes = []string{"a note"}
	td := send(t, m, tuneLoadedMsg{agent: "claude", items: items})
	for i := range items {
		add(fmt.Sprintf("tune pick %d", i), td)
		td = send(t, td, keyPress("down"))
	}
	add("tune toggled", send(t, td, keyPress("space")))
	return out
}

func TestDialogsHaveNoLayoutShift(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		m = send(t, m, probedMsg{name: "spark", probe: live(false)})
		states := dialogStates(t, m)
		for _, s := range states {
			if s.m.dialog == noDialog {
				t.Fatalf("%s: %q did not open a dialog", sizeName(m), s.name)
			}
			if r := rectsOf(s.m)["m:panel"]; r.Y+r.H > sz[1] || r.X+r.W > sz[0] {
				t.Errorf("%s: %q dialog %+v leaves the screen", sizeName(m), s.name, r)
			}
		}
		sameRects(t, states, "m:panel", "m:close", "m:cancel", "hints", "status",
			"m:input:0", "m:input:1", "m:input:2", "m:error")
		// The primary and first extra slots hold different buttons per dialog.
		for _, slot := range [][]string{
			{"m:ack", "m:apply", "m:submit", "m:tune-review", "m:remove", "m:close-btn"},
			{"m:back", "m:test", "m:tune-reset"},
		} {
			var want *rect
			for _, s := range states {
				for _, id := range slot {
					if got, ok := rectsOf(s.m)[id]; ok {
						if want == nil {
							want = &got
						} else if got != *want {
							t.Errorf("%s: slot %v holds %q at %+v in %q, elsewhere %+v", sizeName(m), slot, id, got, s.name, *want)
						}
					}
				}
			}
		}
		var lists, bodies []state
		for _, s := range states {
			if strings.HasPrefix(s.name, "route") || strings.HasPrefix(s.name, "model") {
				lists = append(lists, s)
			}
			if strings.HasPrefix(s.name, "proxy") {
				bodies = append(bodies, s)
			}
		}
		sameRects(t, bodies, "m:body")
		sameRects(t, lists, "m:opt:0")
	}
}

func TestSidebarStaysInItsArea(t *testing.T) {
	for _, sz := range sizes {
		msg := loaded()
		for i := range 65 {
			msg.cfg.Proxies[fmt.Sprintf("a%02d", i)] = config.Proxy{URL: "http://x"}
		}
		m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: sz[0], Height: sz[1]}, msg)
		check := func(m Model, what string) {
			for id, r := range rectsOf(m) {
				if strings.HasPrefix(id, "nav:") && r.Y >= sz[1]-2 {
					t.Errorf("%s %s: %q at row %d overlaps the footer", sizeName(m), what, id, r.Y)
				}
			}
			sel := "nav:" + m.homeIDs()[m.focus]
			if _, ok := rectsOf(m)[sel]; !ok {
				t.Errorf("%s %s: selected %q is not visible", sizeName(m), what, sel)
			}
		}
		check(m, "top")
		for range 70 {
			m = send(t, m, keyPress("down"))
			check(m, fmt.Sprintf("focus %d", m.focus))
		}
	}
}

func TestAddRowIsFixedWhateverTheProxyCount(t *testing.T) {
	for _, sz := range sizes {
		var states []state
		for _, n := range []int{0, 1, 2, 70} {
			msg := loaded()
			msg.cfg.Proxies = map[string]config.Proxy{}
			for i := range n {
				msg.cfg.Proxies[fmt.Sprintf("p%02d", i)] = config.Proxy{URL: "http://x"}
			}
			m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: sz[0], Height: sz[1]}, msg)
			states = append(states, state{fmt.Sprintf("%d proxies", n), m})
		}
		sameRects(t, states, "nav:add", "nav:p00")
	}
}

func TestMultilineTextStaysOneRow(t *testing.T) {
	multi := "first line\nsecond line\nthird line"
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		toast := m
		toast.toast, toast.toastErr = multi, true
		down := send(t, m, probedMsg{name: "spark", probe: app.Probe{HealthErr: errors.New(multi)}})
		down.focus = 2
		form, _ := m.openAdd()
		withErr := send(t, form.(Model), savedMsg{name: "x", err: errors.New(multi)})
		picker := send(t, m, click("switch:codex"), click("m:opt:1"), modelsMsg{route: "spark", list: []proxy.Model{{ID: multi, OwnedBy: "openai"}}})
		for _, c := range []struct {
			m  Model
			id string
		}{{toast, "status"}, {down, "fact:2"}, {withErr, "m:error"}, {picker, "m:opt:1"}} {
			if r := rectsOf(c.m)[c.id]; r.H != 1 {
				t.Errorf("%s: %q is %d rows tall", sizeName(m), c.id, r.H)
			}
		}
		report := send(t, m, probedMsg{name: "spark", probe: app.Probe{Models: []proxy.Model{{ID: multi}, {ID: "next"}}}}, click("details:spark"))
		if lines := strings.Split(report.proxyReport("spark"), "\n"); len(lines) != 9 {
			t.Errorf("%s: report has %d rows, want 7 header rows + 2 models", sizeName(m), len(lines))
		}
	}
}

func TestPillTextDoesNotMove(t *testing.T) {
	m := base(t, 120, 40)
	two := loaded()
	two.cfg.Proxies["zeta"] = config.Proxy{URL: "http://z"}
	m2 := send(t, m, two)
	var at []int
	for _, s := range []Model{
		New(context.Background(), Options{}), m,
		send(t, m, probedMsg{name: "spark", probe: live(false)}),
		send(t, m2, probedMsg{name: "spark", probe: live(false)}, probedMsg{name: "zeta", probe: app.Probe{HealthErr: errors.New("x")}}),
	} {
		s.width, s.height = 120, 40
		pill := ansi.Strip(s.pill(s.pal))
		at = append(at, strings.Index(strings.Split(pill, "\n")[1], " / "))
	}
	for _, x := range at[1:] {
		if x != at[0] {
			t.Fatalf("pill separator moves: %v", at)
		}
	}
}

func TestProbeResultKeepsReportScroll(t *testing.T) {
	m := send(t, base(t, 120, 40), probedMsg{name: "spark", probe: live(false)}, click("details:spark"))
	for range 10 {
		m = send(t, m, pointerMsg{kind: pointerWheel, wheel: 1})
	}
	off := m.vp.YOffset()
	if off == 0 {
		t.Fatal("report did not scroll")
	}
	if m = send(t, m, probedMsg{name: "spark", probe: live(false)}); m.vp.YOffset() != off {
		t.Fatalf("new probe result moved the report from %d to %d", off, m.vp.YOffset())
	}
}
