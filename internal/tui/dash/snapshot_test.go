package dash

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/t3"
)

// TestSnapshots writes rendered frames as ANSI files for eyeballing, e.g.
// with `freeze`. It only runs when AGENTCTL_SNAPSHOTS names a directory.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("AGENTCTL_SNAPSHOTS")
	if dir == "" {
		t.Skip("set AGENTCTL_SNAPSHOTS=<dir> to write frames")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 40}, loaded())
	r := app.Probe{Latency: 23 * time.Millisecond, Models: []proxy.Model{
		{ID: "claude-opus-5-5", OwnedBy: "anthropic"}, {ID: "gpt-6.1-sol", OwnedBy: "openai"}, {ID: "palmyra-x6", OwnedBy: "WRITER"},
	}}
	r.Counts.Claude, r.Counts.Other, r.Counts.Unknown = 1, 1, 1
	m = send(t, m, probedMsg{name: "spark", probe: r})
	m.frame = 7
	frames := map[string]Model{"home": m}

	routes := send(t, m, keyPress("down"), keyPress("enter"))
	frames["routes"] = routes
	frames["models"] = send(t, routes, keyPress("down"), keyPress("enter"), modelsMsg{route: "spark", list: r.Models})
	proxyFocus := m
	proxyFocus.focus = idx(proxyFocus, "proxy:spark")
	frames["proxy"] = send(t, proxyFocus, keyPress("enter"))
	add, _ := m.openAdd()
	frames["add"] = add.(Model)
	hover := m
	hover.hover = "switch:claude"
	frames["hover"] = hover
	selected := m
	selected.focus = idx(selected, "proxy:spark")
	frames["selected-proxy"] = selected
	routed := send(t, m, loadedMsg{cfg: m.cfg, st: m.st, statuses: []agentStatus{
		m.statuses[0], {agent: m.statuses[1].agent, status: app.Status{Route: "spark", URL: "https://spark.example:8445", Model: "gpt-6.1-sol"}},
	}})
	routed.focus = 1
	frames["routed-codex"] = routed

	withT3 := send(t, m, t3States()["update"])
	withT3.focus = idx(withT3, "t3")
	frames["t3"] = withT3
	frames["t3-service"] = send(t, withT3, click("t3-service"))
	devices := send(t, withT3, t3AccessMsg{list: accessList(12)}, keyPress("down"))
	frames["t3-devices"] = devices
	pairForm, _ := devices.openPairForm()
	frames["t3-pair-form"] = pairForm.(Model)
	frames["t3-pairing"] = send(t, withT3, t3PairedMsg{pairing: t3.Pairing{
		URL: "https://minipc.tail035df.ts.net/pair#token=P3BRPMVCDGYE", Expires: time.Now().Add(5 * time.Minute),
	}})

	for name, f := range frames {
		out := f.View().Content
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(out), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
