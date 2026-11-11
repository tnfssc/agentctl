package app

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/tnfssc/agentctl/internal/ocweb"
	"github.com/tnfssc/agentctl/internal/selfupdate"
	"github.com/tnfssc/agentctl/internal/t3"
	"github.com/tnfssc/agentctl/internal/upgrade"
)

// Update is one tool agentctl can bring up to date, each through its own
// updater. The CLI and the dashboard build the same list.
type Update struct {
	Name, Title     string
	Current, Latest string // "" when unknown
	Err             error  // why the newest release is unknown, if it is
	Installed       bool
	// Worth means running it can change something: a newer release, or
	// extras (Pi's extensions) that may have their own.
	Worth bool
	// Note says what else happens, e.g. a server restart.
	Note string
	Run  func(ctx context.Context, out io.Writer) error
}

// UpdateNames are the tools `agentctl update` knows, in the order "all"
// updates them: agents, then servers, then agentctl itself, which replaces
// the running binary.
var UpdateNames = []string{"claude", "codex", "opencode", "pi", "t3", "agentctl"}

// CLIUpdate describes an agent CLI that updates itself.
func CLIUpdate(s upgrade.Status) Update {
	u := Update{
		Name: s.Name, Title: s.Title, Current: s.Version, Latest: s.Latest, Err: s.LatestErr,
		Installed: s.Installed(), Worth: s.Worth(),
		Run: func(ctx context.Context, out io.Writer) error { return upgrade.Update(ctx, out, s.CLI) },
	}
	if s.Extras != "" {
		u.Title += " and its " + s.Extras
	}
	return u
}

// OpenCodeUpdate describes OpenCode, whose server restarts on the new
// version if it runs.
func OpenCodeUpdate(st ocweb.Status) Update {
	u := Update{
		Name: "opencode", Title: "OpenCode", Current: st.Version, Latest: st.Latest, Err: st.LatestErr,
		Installed: st.Supported(), Worth: st.Supported() && st.UpdateAvailable(), Run: ocweb.Update,
	}
	if st.Running {
		u.Note = "its server restarts, stopping running turns"
	}
	return u
}

// T3Update describes T3 Code, whose service restarts on the new nightly.
func T3Update(st t3.Status) Update {
	return Update{
		Name: "t3", Title: "T3 Code", Current: st.CurrentVersion(), Latest: st.Latest, Err: st.LatestErr,
		Installed: st.Installed(), Worth: st.Installed() && st.UpdateAvailable(), Run: t3.Update,
		Note: "its service restarts, stopping running turns",
	}
}

// SelfUpdate describes agentctl. A source build never updates itself: a
// release would silently replace it.
func SelfUpdate(version, latest string, err error) Update {
	u := Update{
		Name: "agentctl", Title: "agentctl", Current: version, Latest: latest, Err: err,
		Installed: true, Worth: selfupdate.Newer(version, latest),
		Run: func(ctx context.Context, _ io.Writer) error {
			_, err := selfupdate.Install(ctx, latest)
			return err
		},
	}
	if !strings.HasPrefix(version, "v") {
		u.Note = "built from source, so it doesn't update itself"
	}
	return u
}

// CheckUpdates looks up the named tools at once, in UpdateNames order. It
// changes nothing.
func CheckUpdates(ctx context.Context, version string, names []string) []Update {
	out := map[string]Update{}
	var mu sync.Mutex
	set := func(u Update) { mu.Lock(); out[u.Name] = u; mu.Unlock() }
	var wg sync.WaitGroup
	for _, c := range upgrade.CLIs {
		if slices.Contains(names, c.Name) {
			wg.Go(func() { set(CLIUpdate(upgrade.Inspect(ctx, c))) })
		}
	}
	if slices.Contains(names, "opencode") {
		wg.Go(func() {
			st, err := ocweb.Inspect(ctx)
			u := OpenCodeUpdate(st)
			if err != nil {
				u.Err = err
			}
			set(u)
		})
	}
	if slices.Contains(names, "t3") {
		wg.Go(func() {
			st, err := t3.Inspect(ctx)
			u := T3Update(st)
			if err != nil {
				u.Err = err
			}
			set(u)
		})
	}
	if slices.Contains(names, "agentctl") {
		wg.Go(func() {
			var latest string
			var err error
			if strings.HasPrefix(version, "v") {
				latest, err = selfupdate.Latest(ctx)
			}
			set(SelfUpdate(version, latest, err))
		})
	}
	wg.Wait()
	var list []Update
	for _, n := range UpdateNames {
		if u, ok := out[n]; ok {
			list = append(list, u)
		}
	}
	return list
}
