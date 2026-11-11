package dash

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// TestPressedButtonSpinsInPlace: the button that started an action shows a
// spinner, the others dim, and none of them changes size or moves.
func TestPressedButtonSpinsInPlace(t *testing.T) {
	for _, sz := range sizes {
		m := base(t, sz[0], sz[1])
		m.version = "v1.4.0"
		ask := send(t, m, selfCheckedMsg{latest: "v1.5.0"}, click("self-update"))
		busy := send(t, ask, keyPress("y"))
		if busy.busy == "" || busy.pressed != "m:yes" {
			t.Fatalf("%s: yes should start the update: busy %q pressed %q", sizeName(m), busy.busy, busy.pressed)
		}
		sameRects(t, []state{{"ask", ask}, {"busy", busy}}, "m:yes", "m:cancel", "m:panel", "hints", "status")

		spin := skin.Spin(busy.frame)
		for _, b := range []button{{"Update", "m:yes", true}, {"Cancel", "m:cancel", false}} {
			idle, working := ask.button(ask.pal, b, dialogSlot), busy.button(busy.pal, b, dialogSlot)
			if lipgloss.Width(idle) != lipgloss.Width(working) || lipgloss.Height(idle) != lipgloss.Height(working) {
				t.Errorf("%s: %q changes size while busy", sizeName(m), b.id)
			}
			if got := strings.Contains(working, spin); got != (b.id == "m:yes") {
				t.Errorf("%s: %q spinner = %v", sizeName(m), b.id, got)
			}
		}
		if busy.hints() == ask.hints() {
			t.Errorf("%s: hints should stop offering keys that do nothing while busy", sizeName(m))
		}
	}
}

func TestBackgroundChecksSpinTheirButton(t *testing.T) {
	m := focusOn(base(t, 120, 40), "proxy:spark")
	if !m.working("test:spark") {
		t.Fatal("Test spins until the first check answers")
	}
	m = send(t, m, probedMsg{name: "spark", probe: live(false)})
	if m.working("test:spark") || m.working("refresh") {
		t.Fatal("nothing spins once the check is in")
	}
	if m = send(t, m, click("test:spark")); !m.working("test:spark") || m.busy != "" {
		t.Fatal("testing again spins Test without blocking the rest")
	}
}
