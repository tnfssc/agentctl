package dash

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/tune"
)

// These tests enforce the "no layout shift" rule: an element that exists in
// two states must have the same rectangle in both.

var sizes = [][2]int{{40, 16}, {44, 30}, {60, 20}, {100, 20}, {80, 19}, {80, 24}, {100, 30}, {120, 40}, {200, 60}}

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
		byFocus := map[int][]state{}
		for focus := range 5 {
			for _, base := range []state{{"local", m}, {"routed", mr}} {
				s := focusOn(base.m, base.m.homeIDs()[focus])
				s.page = pageDetail // compact layouts show buttons on the item's page
				for _, ps := range probeStates(t, s) {
					st := state{fmt.Sprintf("%s focus=%d %s", base.name, focus, ps.name), ps.m}
					states = append(states, st)
					byFocus[focus] = append(byFocus[focus], st)
				}
			}
		}
		busy := m
		busy.busy = "Planning"
		toast := m
		toast.toast, toast.toastErr = strings.Repeat("something failed ", 20), true
		states = append(states, state{"busy", busy}, state{"toast", toast})
		refreshing, _ := focusOn(send(t, m, probedMsg{name: "spark", probe: live(false)}), "proxy:spark").act("refresh")
		r := refreshing.(Model)
		r.page = pageDetail
		states = append(states, state{"refreshing", r})
		for _, applied := range [][]string{nil, {"memory"}} {
			s := m
			s.applied = applied
			s = focusOn(s, "memory")
			s.page = pageDetail
			states = append(states, state{fmt.Sprintf("memory %v", applied), s})
		}
		for _, c := range []struct {
			name    string
			version string
			msg     tea.Msg
		}{
			{"self checking", "v1.4.0", nil},
			{"self update", "v1.4.0", selfCheckedMsg{latest: "v1.5.0"}},
			{"self latest", "v1.5.0", selfCheckedMsg{latest: "v1.5.0"}},
			{"self offline", "v1.5.0", selfCheckedMsg{err: errors.New(strings.Repeat("offline ", 20))}},
			{"self source", "dev", nil},
		} {
			s := m
			s.version = c.version
			if c.msg != nil {
				s = send(t, s, c.msg)
			}
			s = focusOn(s, "self")
			s.page = pageDetail
			states = append(states, state{c.name, s})
		}

		// The frame never moves, whatever is selected.
		sameRects(t, states, "status", "hints", "more",
			"fact:0", "fact:1", "fact:2", "fact:3", "fact:4", "back")
		// Items move only when a new selection scrolls the list.
		for _, group := range byFocus {
			sameRects(t, group, "nav:agent:claude", "nav:agent:codex", "nav:t3", "nav:behavior", "nav:git", "nav:proxy:spark", "nav:add")
		}
		// Button slots: whatever button a slot holds, the slot never moves.
		for _, slot := range [][]string{
			{"switch:claude", "switch:codex", "test:spark", "add-btn", "tune:instructions", "self-update", "guide:memory"},
			{"local:codex", "refresh", "self-check"},
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
	allow := send(t, m, probedMsg{name: "spark", probe: mixed}, click("details:spark"), click("m:allow"))
	add("allow", allow)
	add("allow toggled", send(t, allow, keyPress("space"), keyPress("down")))
	add("allow empty", send(t, m, click("details:spark"), click("m:allow")))
	many := live(true)
	for i := range many.Models {
		many.Models[i].OwnedBy = "Writer"
	}
	allowMany := send(t, m, probedMsg{name: "spark", probe: many}, click("details:spark"), click("m:allow"))
	add("allow many", allowMany)
	for range 40 {
		allowMany = send(t, allowMany, keyPress("down"))
	}
	add("allow many scrolled", allowMany)
	self := m
	self.version = "v1.4.0"
	askSelf := send(t, self, selfCheckedMsg{latest: "v1.5.0"}, click("self-update"))
	add("ask self", askSelf)
	add("ask self busy", send(t, askSelf, keyPress("y")))
	add("allow busy", send(t, allow, keyPress("space"), keyPress("y")))
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

	for _, n := range []int{0, 3, 60} {
		dev := send(t, m, t3AccessMsg{list: accessList(n)})
		add(fmt.Sprintf("devices %d", n), dev)
		if n > 0 {
			add(fmt.Sprintf("devices %d revoke", n), send(t, dev, click("m:revoke")))
		}
		for range 40 {
			dev = send(t, dev, keyPress("down"))
		}
		add(fmt.Sprintf("devices %d scrolled", n), dev)
	}
	add("signing", send(t, m, click("guide:signing")))
	remote := m
	remote.remote = true
	add("signing over ssh", send(t, remote, click("guide:signing")))
	signed := m
	signed.signed, signed.signingKey = true, "~/.ssh/id_ed25519_signing.pub"
	add("signing done", send(t, signed, click("guide:signing")))
	add("guide memory", send(t, m, click("guide:memory")))
	tunnel := send(t, m, click("guide:tunnel"))
	tunnel.applied = []string{"tunnel"}
	add("guide tunnel set up", tunnel)
	remoteTunnel := tunnel
	remoteTunnel.remote = true
	add("guide tunnel set up over ssh", remoteTunnel)
	svc := m
	svc.t3, svc.t3Loaded = t3States()["running"].st, true
	add("service", send(t, svc, click("t3-service")))
	pf := send(t, m, t3AccessMsg{list: accessList(3)}, click("m:pair"))
	add("pair form", pf)
	add("pair form error", send(t, pf, pairedMsg{err: errors.New(strings.Repeat("no ", 50))}))
	add("pairing", send(t, m, pairedMsg{pairing: t3.Pairing{URL: "https://box.tail035df.ts.net/pair#token=TOKEN12CHARS", Expires: time.Now().Add(5 * time.Minute)}}))
	add("pairing local", send(t, m, pairedMsg{pairing: t3.Pairing{URL: "http://127.0.0.1:3773/pair#token=TOKEN12CHARS", Expires: time.Now().Add(5 * time.Minute)}}))
	oc := m
	oc.oc, oc.ocLoaded = ocStates()["running"].st, true
	oc = focusOn(oc, "opencode")
	add("service opencode", send(t, oc, click("oc-server")))
	add("ask opencode", send(t, oc, click("oc-server"), click("m:opt:2")))
	ocForm := send(t, oc, click("oc-pair"))
	add("pair form opencode", ocForm)
	add("pair form opencode error", send(t, ocForm, pairedMsg{err: errors.New(strings.Repeat("no ", 50))}))
	add("pairing opencode", send(t, ocForm, pairedMsg{pairing: t3.Pairing{URL: "https://box.tail035df.ts.net/auth/connect/I0wuzPtW0raVKaWaAoHj_A", Expires: time.Now().Add(5 * time.Minute)}}))
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
		sameRects(t, states, "m:panel", "m:close", "hints", "status",
			"m:input:0", "m:input:1", "m:input:2", "m:error", "m:info", "m:url")
		// Each slot holds different buttons per dialog.
		for _, slot := range [][]string{
			{"m:ack", "m:apply", "m:submit", "m:tune-review", "m:remove", "m:close-btn", "m:yes", "m:pair", "m:submit-pair", "m:allow-save"},
			{"m:cancel", "m:revoke-idle"},
			{"m:back", "m:test", "m:tune-reset", "m:revoke", "m:copy", "m:copy-guide"},
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
		var lists, bodies, devices []state
		for _, s := range states {
			if strings.HasPrefix(s.name, "route") || strings.HasPrefix(s.name, "model") || strings.HasPrefix(s.name, "service") ||
				strings.HasPrefix(s.name, "allow ") || s.name == "allow" {
				lists = append(lists, s)
			}
			if s.m.dialog == devicesDialog {
				devices = append(devices, s)
			}
			if strings.HasPrefix(s.name, "proxy") {
				bodies = append(bodies, s)
			}
		}
		sameRects(t, bodies, "m:body")
		sameRects(t, lists, "m:opt:0")
		// Items scroll inside the list; its first row and the rows around it stay.
		sameRects(t, devices, "m:opt:0", "m:info", "m:revoke", "m:pair")
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

func TestGroupsAreSpacedAndAddEndsTheProxies(t *testing.T) {
	msg := loaded()
	msg.cfg.Proxies["zeta"] = config.Proxy{URL: "http://x"}
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 60}, msg)
	ids := m.homeIDs()
	if i := slices.Index(ids, "add"); i < 1 || ids[i-1] != "proxy:zeta" || ids[i+1] != "behavior" {
		t.Fatalf("Add a proxy should follow the last proxy and come before Settings: %v", ids)
	}
	if ids[len(ids)-1] != "self" {
		t.Fatalf("agentctl's updates close Settings: %v", ids)
	}
	var sections []string
	for i, l := range navLines(m.navEntries(m.pal)) {
		if l.header == "" {
			continue
		}
		sections = append(sections, l.header)
		if i > 0 && navLines(m.navEntries(m.pal))[i-1].entry != -1 {
			t.Errorf("%s should have a blank row above it", l.header)
		}
	}
	if !slices.Equal(sections, []string{"Agents", "Servers", "Proxies", "Settings"}) {
		t.Fatalf("sections = %v", sections)
	}
	r := rectsOf(m)
	if r["nav:add"].Y != r["nav:proxy:zeta"].Y+2 {
		t.Errorf("Add sits right under the last proxy: %+v vs %+v", r["nav:add"], r["nav:proxy:zeta"])
	}
}

func TestMultilineTextStaysOneRow(t *testing.T) {
	multi := "first line\nsecond line\nthird line"
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		toast := m
		toast.toast, toast.toastErr = multi, true
		down := send(t, m, probedMsg{name: "spark", probe: app.Probe{HealthErr: errors.New(multi)}})
		down = focusOn(down, "proxy:spark")
		down.page = pageDetail
		form, _ := m.openAdd()
		withErr := send(t, form.(Model), savedMsg{name: "x", err: errors.New(multi)})
		picker := send(t, m, click("switch:codex"), click("m:opt:1"), modelsMsg{route: "spark", list: []proxy.Model{{ID: multi, OwnedBy: "openai"}}}, keyPress("down"))
		for _, c := range []struct {
			m  Model
			id string
		}{{toast, "status"}, {down, "fact:2"}, {withErr, "m:error"}, {picker, "m:opt:1"}} {
			want := 1
			if strings.HasPrefix(c.id, "m:opt:") {
				want = 2 // list items are two rows, whatever their text
			}
			r, shown := rectsOf(c.m)[c.id]
			if !shown && m.compact() {
				continue // the smallest pages have no room for it
			}
			if r.H != want {
				t.Errorf("%s: %q is %d rows tall", sizeName(m), c.id, r.H)
			}
		}
		report := send(t, m, probedMsg{name: "spark", probe: app.Probe{Models: []proxy.Model{{ID: multi}, {ID: "next"}}}}, click("details:spark"))
		if lines := strings.Split(report.proxyReport("spark"), "\n"); len(lines) != 9 {
			t.Errorf("%s: report has %d rows, want 7 header rows + 2 models", sizeName(m), len(lines))
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

// TestTouchTargets keeps things a finger can hit on a phone: list items
// and options two rows tall, buttons three, all inside the screen and clear
// of the footer.
func TestTouchTargets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	for _, sz := range sizes {
		m := send(t, base(t, sz[0], sz[1]), probedMsg{name: "spark", probe: live(false)})
		detail := m
		detail.page = pageDetail
		states := append([]state{{"home", m}, {"detail", detail}}, dialogStates(t, m)...)
		for _, s := range states {
			for id, r := range rectsOf(s.m) {
				if r.X < 0 || r.Y < 0 || r.X+r.W > sz[0] || r.Y+r.H > sz[1] {
					t.Errorf("%s %s: %q at %+v leaves the screen", sizeName(s.m), s.name, id, r)
				}
				tall := strings.HasPrefix(id, "nav:") || strings.HasPrefix(id, "m:opt:")
				if tall && (r.H < 2 || r.Y+r.H > sz[1]-2) {
					t.Errorf("%s %s: %q at %+v is under two rows or in the footer", sizeName(s.m), s.name, id, r)
				}
			}
		}
		for _, id := range []string{"switch:claude", "back"} {
			if r, ok := rectsOf(detail)[id]; ok && r.H != 3 {
				t.Errorf("%s: button %q is %d rows tall", sizeName(m), id, r.H)
			}
		}
	}
}
