package dash

import (
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/testutil"
	"github.com/tnfssc/agentctl/internal/tune"
)

func TestRecommendedAsksAboutRiskyItemsThenCelebrates(t *testing.T) {
	testutil.Home(t)
	m := focusOn(send(t, base(t, 120, 40), probedMsg{name: "spark", probe: live(false)}), "recommended")
	m = send(t, m, keyPress("enter"))
	if m.dialog != askDialog || m.ask.no != "Leave off" || !strings.Contains(m.ask.body, "Claude runs every tool") {
		t.Fatalf("risky items need their own yes: dialog %v ask %+v", m.dialog, m.ask)
	}
	if left := send(t, m, click("m:no")); left.busy != "Planning" {
		t.Fatal("Leave off goes on without them")
	}
	if cancelled := send(t, m, keyPress("esc")); cancelled.dialog != noDialog || cancelled.busy != "" {
		t.Fatal("Cancel stops altogether")
	}
	done := send(t, m, appliedMsg{plan: agent.Plan{Agent: tune.All}})
	if done.confetti != confettiFrames || !strings.Contains(done.toast, "recommended setup is on") {
		t.Fatalf("confetti %d toast %q", done.confetti, done.toast)
	}
	if !done.needsFrames() {
		t.Fatal("the burst animates")
	}
	for range confettiFrames {
		done = send(t, done, frameMsg{})
	}
	if done.confetti != 0 || done.needsFrames() {
		t.Fatal("then everything is still again")
	}
	remote := send(t, func() Model { r := m; r.remote = true; return r }(), appliedMsg{plan: agent.Plan{Agent: tune.All}})
	if remote.confetti != 0 {
		t.Fatal("no animation over SSH")
	}
}

func TestTuneReviewAsksBeforeRiskyItems(t *testing.T) {
	testutil.Home(t)
	items, err := tune.Inspect("claude")
	if err != nil {
		t.Fatal(err)
	}
	m := send(t, base(t, 120, 40), tuneLoadedMsg{agent: "claude", items: items})
	if !m.tuneWant["no-approvals"] {
		t.Fatal("recommended ticks everything, risky items too")
	}
	m = send(t, m, click("m:tune-review"))
	if m.dialog != askDialog || m.ask.back != tuneDialog {
		t.Fatalf("dialog %v", m.dialog)
	}
	if back := send(t, m, keyPress("esc")); back.dialog != tuneDialog || !back.tuneWant["no-approvals"] {
		t.Fatal("Cancel returns to the list as it was")
	}
}
