package dash

import (
	"context"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/models"
	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// Allowing models whose vendor agentctl can't prove, like `agentctl proxy
// allow`: the proxy dialog's Allow models opens a checklist of them, and
// saving rewrites the proxy's allow list and plans refreshing the agents'
// model lists. Anything that looks like Claude never shows up here.

// openAllow lists the proxy's chat models that are neither Claude nor from a
// known vendor, ticked when they are already allowed.
func (m Model) openAllow() Model {
	allowed := m.cfg.Proxies[m.proxy].AllowModels
	m.options, m.allowWant = nil, map[string]bool{}
	for _, pm := range m.probes[m.proxy].Models {
		if models.Classify(pm.ID, pm.OwnedBy, nil) != models.Unknown || !models.Chat(pm.ID) {
			continue
		}
		m.options = append(m.options, option{label: pm.ID, detail: pm.OwnedBy, value: pm.ID})
		m.allowWant[pm.ID] = slices.Contains(allowed, pm.ID)
	}
	m.pick, m.top = 0, 0
	m.dialog = allowDialog
	return m
}

// toggleAllow flips the highlighted model. The map is replaced, not mutated,
// because Bubble Tea copies the model by value.
func (m *Model) toggleAllow() {
	if len(m.options) == 0 {
		return
	}
	id := m.options[m.pick].value
	want := make(map[string]bool, len(m.allowWant))
	for k, v := range m.allowWant {
		want[k] = v
	}
	want[id] = !want[id]
	m.allowWant = want
}

// submitAllow saves the ticks, or just closes when nothing changed.
func (m Model) submitAllow() (tea.Model, tea.Cmd) {
	allowed := m.cfg.Proxies[m.proxy].AllowModels
	shown := make([]string, len(m.options))
	changed := false
	for i, o := range m.options {
		shown[i] = o.value
		changed = changed || m.allowWant[o.value] != slices.Contains(allowed, o.value)
	}
	if !changed {
		return m.close("", false), nil
	}
	m.busy = "Saving allowed models"
	return m, saveAllow(m.ctx, m.proxy, shown, m.allowWant, m.st)
}

// saveAllow rewrites a proxy's allow list for the models shown, keeping
// entries for models the proxy no longer serves, then plans the refresh
// that puts the change into agents' model lists.
func saveAllow(ctx context.Context, name string, shown []string, want map[string]bool, st *config.State) tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return plannedMsg{err: err}
		}
		p, err := cfg.Lookup(name)
		if err != nil {
			return plannedMsg{err: err}
		}
		allow := slices.DeleteFunc(slices.Clone(p.AllowModels), func(id string) bool { return slices.Contains(shown, id) })
		for _, id := range shown {
			if want[id] {
				allow = append(allow, id)
			}
		}
		p.AllowModels = allow
		cfg.Proxies[name] = p
		if err := cfg.Save(); err != nil {
			return plannedMsg{err: err}
		}
		plan, err := app.Refresh(ctx, cfg, st)
		return plannedMsg{plan: plan, err: err, refresh: true, saved: "Saved. "}
	}
}

func (m Model) drawAllow(sc *scene) {
	p := m.pal
	b := m.dialogBody()
	m.panel(sc, "Models · "+m.proxy, "Which may every agent use?", nil,
		&button{"Cancel", "m:cancel", false}, &button{"Save", "m:allow-save", true})
	if len(m.options) == 0 {
		why := "Nothing to allow: every chat model on " + m.proxy + " is from a known vendor or Claude's."
		if r, ok := m.probes[m.proxy]; !ok || r.Err() != nil {
			why = "Test " + m.proxy + " first; its models aren't loaded."
		}
		for i, l := range skin.Clip(why, b.W, 2) {
			sc.add(skin.Cell(skin.Fg(p.Muted).Render(l), b.W), b.X, b.Y+i, 53, "")
		}
		return
	}
	sc.add(skin.Cell(skin.Fg(p.Muted).Render("Tick models you know aren't Claude."), b.W), b.X, b.Y, 53, "")
	allowed := m.cfg.Proxies[m.proxy].AllowModels
	rows := m.listRows()
	for r := range rows {
		i := m.top + r
		if i >= len(m.options) {
			break
		}
		o := m.options[i]
		box, was := "[ ]", slices.Contains(allowed, o.value)
		if m.allowWant[o.value] {
			box = "[x]"
		}
		detail := o.detail
		switch {
		case was && !m.allowWant[o.value]:
			detail += " · will hide"
		case !was && m.allowWant[o.value]:
			detail += " · will allow"
		case was:
			detail += " · allowed"
		}
		id := "m:opt:" + strconv.Itoa(i)
		sc.add(p.Tall(box, o.label, detail, "", b.W, i == m.pick, m.hover == id), b.X, b.Y+2+2*r, 53, id)
	}
	m.drawMore(sc, b.X, b.Y+2+2*rows, rows)
}
