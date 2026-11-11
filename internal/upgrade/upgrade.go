// Package upgrade keeps the coding agents' CLIs current through their own
// updaters (`claude update`, `codex update`, `pi update --all`), which know
// how each was installed. The newest release comes from npm's registry.
// OpenCode (ocweb), T3 Code (t3) and agentctl (selfupdate) have their own
// packages.
package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/semver"
)

// CLI is one agent's command line tool.
type CLI struct {
	Name, Title string
	Package     string   // npm package whose latest dist-tag is the newest release
	Args        []string // its own update command
	// Extras means the update covers more than the CLI (Pi's extensions),
	// so it is worth running even when the CLI itself is current.
	Extras string
}

// CLIs are the agents updated here, in dashboard order.
var CLIs = []CLI{
	{Name: "claude", Title: "Claude Code", Package: "@anthropic-ai/claude-code", Args: []string{"update"}},
	{Name: "codex", Title: "Codex", Package: "@openai/codex", Args: []string{"update"}},
	// --no-approve: never trust project-local files from wherever agentctl runs.
	{Name: "pi", Title: "Pi", Package: "@earendil-works/pi-coding-agent", Args: []string{"update", "--all", "--no-approve"}, Extras: "extensions"},
}

// Find returns the CLI called name.
func Find(name string) (CLI, bool) {
	for _, c := range CLIs {
		if c.Name == name {
			return c, true
		}
	}
	return CLI{}, false
}

// Registry is npm's registry; tests point it at a fake.
var Registry = "https://registry.npmjs.org"

var client = &http.Client{Timeout: 5 * time.Second}

// Status is what is installed and what is newest.
type Status struct {
	CLI
	Path      string // "" when not installed
	Version   string
	Latest    string // "" when unknown
	LatestErr error
}

// Installed reports whether the CLI is on PATH.
func (s Status) Installed() bool { return s.Path != "" }

// UpdateAvailable reports whether a newer release than the installed one
// exists.
func (s Status) UpdateAvailable() bool {
	return s.Installed() && s.Latest != "" && s.Version != "" && semver.Compare(s.Latest, s.Version) > 0
}

// Worth reports whether running the update can change anything: a newer
// release, or extras that may have their own.
func (s Status) Worth() bool { return s.UpdateAvailable() || (s.Installed() && s.Extras != "") }

var versionRE = regexp.MustCompile(`\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?`)

// Inspect reads the installed version and asks npm for the newest one. It
// changes nothing.
func Inspect(ctx context.Context, c CLI) Status {
	s := Status{CLI: c}
	bin, err := exec.LookPath(c.Name)
	if err != nil {
		return s
	}
	s.Path = bin
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output() //nolint:gosec // an agent CLI from PATH, fixed arguments
	if err == nil {
		s.Version = versionRE.FindString(ansi.Strip(string(out)))
	}
	s.Latest, s.LatestErr = Latest(ctx, c.Package)
	return s
}

// InspectAll inspects every CLI at once.
func InspectAll(ctx context.Context) []Status {
	out := make([]Status, len(CLIs))
	var wg sync.WaitGroup
	for i, c := range CLIs {
		wg.Go(func() { out[i] = Inspect(ctx, c) })
	}
	wg.Wait()
	return out
}

// Latest is an npm package's latest dist-tag.
func Latest(ctx context.Context, pkg string) (string, error) {
	u := Registry + "/-/package/" + url.PathEscape(pkg) + "/dist-tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking for updates: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var tags map[string]string
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&tags) != nil || tags["latest"] == "" {
		return "", fmt.Errorf("checking for updates: npm has no latest version of %s (%s)", pkg, resp.Status)
	}
	return tags["latest"], nil
}

// Update runs the CLI's own updater, its output going to out. Failures carry
// the updater's last lines even when out discards them.
func Update(ctx context.Context, out io.Writer, c CLI) error {
	bin, err := exec.LookPath(c.Name)
	if err != nil {
		return fmt.Errorf("%s is not installed", c.Title)
	}
	cmd := exec.CommandContext(ctx, bin, c.Args...) //nolint:gosec // an agent CLI from PATH, fixed arguments
	var buf bytes.Buffer
	w := io.MultiWriter(out, &buf)
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %s", c.Name, strings.Join(c.Args, " "), lastLines(ansi.Strip(buf.String()), err))
	}
	return nil
}

// lastLines is the end of a command's output, or err when it printed
// nothing.
func lastLines(out string, err error) string {
	var lines []string
	for l := range strings.SplitSeq(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return err.Error()
	}
	return strings.Join(lines[max(0, len(lines)-2):], " · ")
}
