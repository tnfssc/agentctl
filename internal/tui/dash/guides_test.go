package dash

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/guide"
	"github.com/tnfssc/agentctl/internal/testutil"
)

func TestGuideOffersUndoOnceSetUp(t *testing.T) {
	testutil.Home(t)
	svc := base(t, 120, 40)
	svc.t3, svc.t3Loaded = t3States()["running"].st, true
	m := send(t, svc, click("t3-service"))
	var opened Model
	for i, o := range m.options {
		if o.value == "guide:tunnel" {
			opened = send(t, m, click("m:opt:"+string(rune('0'+i))))
		}
	}
	if opened.dialog != guideDialog || opened.guide != "tunnel" {
		t.Fatalf("T3 Code's service menu should open the tunnel guide, dialog %v", opened.dialog)
	}
	view := opened.View().Content
	if !strings.Contains(view, "agentctl guide tunnel") || strings.Contains(view, "tunnel-undo") || strings.Contains(view, "Copy undo") {
		t.Fatal("before it is set up, only the setup command shows")
	}

	dir, _ := guide.RecordsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tunnel.md"), []byte("tunnel agentctl-box"), 0o600); err != nil {
		t.Fatal(err)
	}
	msg, ok := load().(loadedMsg)
	if !ok || !slices.Equal(msg.applied, []string{"tunnel"}) {
		t.Fatalf("loading should find the record: %+v", msg.applied)
	}
	set := loaded()
	set.applied = msg.applied
	m = send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 40}, set, click("guide:tunnel"))
	if view = m.View().Content; !strings.Contains(view, "agentctl guide tunnel-undo") || !strings.Contains(view, "Copy undo") {
		t.Fatal("once set up, the undo command shows too")
	}
	if _, cmd := m.actDialog("copy-undo"); cmd == nil {
		t.Fatal("copying the undo command should set the clipboard")
	}
}

func TestMemoryIsABehaviorItem(t *testing.T) {
	m := focusOn(base(t, 120, 40), "memory")
	if d := m.detail(m.pal); d.buttons[0].id != "guide:memory" || !strings.Contains(d.facts[0], "not set up") {
		t.Fatalf("detail %+v", d)
	}
	if m = send(t, m, keyPress("enter")); m.dialog != guideDialog || m.guide != "memory" {
		t.Fatalf("opening the row should open the guide, dialog %v", m.dialog)
	}
	for _, o := range send(t, base(t, 120, 40), click("t3-service")).options {
		if o.value == "guide:memory" {
			t.Fatal("memory limits cover every agent, so they don't belong in T3 Code's menu")
		}
	}
	set := m
	set.applied = []string{"memory"}
	if d := set.memoryDetail(set.pal); !strings.Contains(d.facts[0], "memory-undo") || set.memoryState() != "limits set up" {
		t.Fatalf("set up: %+v", d.facts)
	}
}
