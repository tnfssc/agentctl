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
	m := send(t, New(context.Background()), tea.WindowSizeMsg{Width: 120, Height: 40}, loaded())
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
	frames["proxy"] = send(t, m, keyPress("down"), keyPress("down"), keyPress("enter"))
	add, _ := m.openAdd()
	frames["add"] = add.(Model)
	hover := m
	hover.hover = "switch:claude"
	frames["hover"] = hover

	for name, f := range frames {
		out := f.View().Content
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(out), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
