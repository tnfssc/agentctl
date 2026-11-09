package dash

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tnfssc/agentctl/internal/app"
	"github.com/tnfssc/agentctl/internal/config"
	"github.com/tnfssc/agentctl/internal/proxy"
	"github.com/tnfssc/agentctl/internal/testutil"
)

// mixed is a proxy serving a model of each kind the allow list cares about.
var mixed = app.Probe{Models: []proxy.Model{
	{ID: "claude-opus-5-5", OwnedBy: "anthropic"},
	{ID: "gpt-6.1-sol", OwnedBy: "openai"},
	{ID: "palmyra-x6", OwnedBy: "Writer"},
	{ID: "sonnet-mirror", OwnedBy: "Writer"},
	{ID: "mystery-image-1", OwnedBy: "Writer"},
	{ID: "kimi-k3", OwnedBy: "openai-compatible"},
}}

func allowing(t *testing.T, allowed ...string) Model {
	t.Helper()
	msg := loaded()
	msg.cfg.Proxies["spark"] = config.Proxy{URL: "https://spark.example:8445", AllowModels: allowed}
	return send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 40}, msg,
		probedMsg{name: "spark", probe: mixed}, click("details:spark"), click("m:allow"))
}

func TestAllowListsOnlyUnverifiedChatModels(t *testing.T) {
	m := allowing(t, "kimi-k3")
	if m.dialog != allowDialog {
		t.Fatalf("dialog = %v", m.dialog)
	}
	var ids []string
	for _, o := range m.options {
		ids = append(ids, o.value)
	}
	// Claude-looking names, known vendors and image models never show up.
	if want := []string{"palmyra-x6", "kimi-k3"}; !slices.Equal(ids, want) {
		t.Fatalf("options %v, want %v", ids, want)
	}
	if m.allowWant["palmyra-x6"] || !m.allowWant["kimi-k3"] {
		t.Fatalf("ticks %v should match the allow list", m.allowWant)
	}
	if !strings.Contains(m.View().Content, "allowed") {
		t.Fatal("an allowed model should say so")
	}
}

func TestAllowSavesAndPlansARefresh(t *testing.T) {
	testutil.Home(t)
	cfg := &config.Config{Proxies: map[string]config.Proxy{
		"spark": {URL: "https://spark.example:8445", AllowModels: []string{"kimi-k3", "gone-model"}},
	}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	m := allowing(t, "kimi-k3", "gone-model")
	m = send(t, m, click("m:opt:0"), keyPress("down"), keyPress("space"))
	if !m.allowWant["palmyra-x6"] || m.allowWant["kimi-k3"] {
		t.Fatalf("ticks %v", m.allowWant)
	}
	next, cmd := m.actDialog("allow-save")
	if next.(Model).busy == "" || cmd == nil {
		t.Fatal("saving should run in the background")
	}
	msg, ok := cmd().(plannedMsg)
	if !ok || !errors.Is(msg.err, app.ErrNothingToRefresh) || !msg.refresh || msg.saved == "" {
		t.Fatalf("got %+v; with no agent routed there is nothing to refresh", msg)
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// Models the proxy no longer serves keep their entries.
	if got, want := saved.Proxies["spark"].AllowModels, []string{"gone-model", "palmyra-x6"}; !slices.Equal(got, want) {
		t.Fatalf("allow list %v, want %v", got, want)
	}
	done, reload := next.(Model).Update(msg)
	if d := done.(Model); d.dialog != noDialog || !strings.HasPrefix(d.toast, "Saved. ") || reload == nil {
		t.Fatalf("toast %q dialog %v; the saved config should be reloaded", d.toast, d.dialog)
	}
}

func TestAllowWithoutChangesJustCloses(t *testing.T) {
	m := allowing(t, "kimi-k3")
	next, cmd := m.actDialog("allow-save")
	if next.(Model).dialog != noDialog || cmd != nil {
		t.Fatal("nothing changed, so nothing should be saved")
	}
}

func TestAllowExplainsAnEmptyList(t *testing.T) {
	msg := loaded()
	m := send(t, New(context.Background(), Options{}), tea.WindowSizeMsg{Width: 120, Height: 40}, msg,
		click("details:spark"), click("m:allow"))
	if m.dialog != allowDialog || !strings.Contains(m.View().Content, "Test spark first") {
		t.Fatal("before the proxy answers, the dialog should say why it is empty")
	}
	m = send(t, m, keyPress("space"), keyPress("down"))
	if m.dialog != allowDialog {
		t.Fatal("keys on an empty list must not break the dialog")
	}
}
