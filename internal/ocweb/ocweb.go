// Package ocweb runs OpenCode's background server on this machine, the one
// that also serves OpenCode's web app, by driving the upstream `opencode`
// CLI: start, restart, stop, upgrade and pairing links. OpenCode owns the
// process and its settings; agentctl reads what `opencode` prints and the
// registration file the server writes, and changes nothing itself.
//
// The server is not a system service: `opencode service start` spawns it
// detached, every opencode command starts it on demand, and it does not
// come back after a reboot until something starts it again.
package ocweb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/tnfssc/agentctl/internal/semver"
)

// DistTagsURL is OpenCode v2's npm dist-tags; a variable so tests can point
// it at a fake.
var DistTagsURL = "https://registry.npmjs.org/-/package/@opencode%2Fcli/dist-tags"

var client = &http.Client{Timeout: 5 * time.Second}

// ErrNotInstalled means there is no opencode on PATH.
var ErrNotInstalled = errors.New("OpenCode is not installed; agentctl needs OpenCode v2's `opencode` on PATH")

// SessionTTL is how long a paired browser or app stays signed in.
const SessionTTL = 30 * 24 * time.Hour

// Status is what agentctl shows about OpenCode's server on this machine.
type Status struct {
	CLI     string // opencode on PATH, "" when missing
	Version string // the CLI's version

	Running bool
	URL     string // where the server listens, "" when stopped
	// Serving is the version the running server reports when it differs
	// from the CLI's, e.g. after an upgrade without a restart.
	Serving string

	Latest    string // newest release, "" if unknown
	LatestErr error
}

// Installed reports whether an opencode CLI exists.
func (s Status) Installed() bool { return s.CLI != "" }

// Supported reports whether the CLI has a background server: v2 and later.
func (s Status) Supported() bool { return s.Installed() && semver.Compare(s.Version, "2.0.0") >= 0 }

// UpdateAvailable reports whether a newer release exists than the CLI.
func (s Status) UpdateAvailable() bool {
	return s.Latest != "" && s.Version != "" && semver.Compare(s.Latest, s.Version) > 0
}

// OnLatest reports whether the CLI is known to be the newest release.
func (s Status) OnLatest() bool {
	return s.Latest != "" && s.Version != "" && !s.UpdateAvailable()
}

// Inspect reads the server's state. It never starts or changes anything.
func Inspect(ctx context.Context) (Status, error) {
	var s Status
	if s.CLI, _ = cli(); s.CLI == "" {
		return s, nil // not installed is a state, not an error
	}
	out, err := run(ctx, s.CLI, "--version")
	if err != nil {
		return s, err
	}
	// "opencode v2.2.6"; v1 prints the bare version.
	v := strings.TrimSpace(string(out))
	s.Version = strings.TrimPrefix(v[strings.LastIndex(v, " ")+1:], "v")
	if !s.Supported() {
		return s, nil
	}
	s.Latest, s.LatestErr = latest(ctx)
	if out, err = run(ctx, s.CLI, "service", "status"); err != nil {
		return s, err
	}
	// The URL it listens on, or "stopped".
	if u := strings.TrimSpace(string(out)); strings.HasPrefix(u, "http") {
		s.Running, s.URL = true, u
		if reg := registration(); reg.URL == u && reg.Version != s.Version {
			s.Serving = reg.Version
		}
	}
	return s, nil
}

// registration is the file a running server advertises itself in. Only
// its url and version are read; it also holds the server's password.
func registration() (reg struct{ URL, Version string }) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	b, err := os.ReadFile(filepath.Join(dir, "opencode", "service.json")) //nolint:gosec // opencode's own state dir
	if err == nil {
		_ = json.Unmarshal(b, &reg)
	}
	return reg
}

// latest is the newest release on npm's latest tag. v2 is @opencode/cli;
// opencode-ai is v1.
func latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DistTagsURL, nil)
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
		return "", fmt.Errorf("checking for updates: %s gave no latest version", DistTagsURL)
	}
	return tags["latest"], nil
}

func cli() (string, error) {
	bin, err := exec.LookPath("opencode")
	if err != nil {
		return "", ErrNotInstalled
	}
	return bin, nil
}

// run executes opencode and returns stdout. The server it may start
// inherits this environment, so nothing is added to it.
func run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // opencode from PATH, with fixed arguments
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("opencode %s: %s", strings.Join(args, " "), errorMessage(stderr.String()+"\n"+stdout.String(), err))
	}
	return []byte(ansi.Strip(stdout.String())), nil
}

// step runs opencode commands in order, with their output going to out.
// Failures carry opencode's message even when out discards it.
func step(ctx context.Context, out io.Writer, args ...[]string) error {
	bin, err := cli()
	if err != nil {
		return err
	}
	for _, a := range args {
		cmd := exec.CommandContext(ctx, bin, a...) //nolint:gosec // opencode from PATH, with fixed arguments
		var buf bytes.Buffer
		w := io.MultiWriter(out, &buf)
		cmd.Stdout, cmd.Stderr = w, w
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("opencode %s: %s", strings.Join(a, " "), errorMessage(ansi.Strip(buf.String()), err))
		}
	}
	return nil
}

// errorMessage finds opencode's error in its output: "Error: message"
// followed by a stack. Without one, the last line is the best guess, then
// the exit status.
func errorMessage(output string, err error) string {
	var last string
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if msg, ok := strings.CutPrefix(line, "Error: "); ok {
			return msg
		}
		if line != "" && !strings.HasPrefix(line, "at ") {
			last = line
		}
	}
	if last != "" {
		return last
	}
	return err.Error()
}

// Start starts the server and returns once it answers.
func Start(ctx context.Context, out io.Writer) error {
	return step(ctx, out, []string{"service", "start"})
}

// Restart restarts the server, which stops running agent turns.
func Restart(ctx context.Context, out io.Writer) error {
	return step(ctx, out, []string{"service", "restart"})
}

// Stop stops the server. The next opencode command starts it again.
func Stop(ctx context.Context, out io.Writer) error {
	return step(ctx, out, []string{"service", "stop"})
}

// Update upgrades opencode the way it was installed, then restarts the
// server on the new version if it was running.
func Update(ctx context.Context, out io.Writer) error {
	st, err := Inspect(ctx)
	if err != nil {
		return err
	}
	if !st.Running {
		return step(ctx, out, []string{"upgrade"})
	}
	return step(ctx, out, []string{"upgrade"}, []string{"service", "restart"})
}

// SignOutAll drops the server's password and starts it again with a new
// one. Paired sessions are signed with the password, so every paired
// browser and app needs a new link; opencode on this machine reads the new
// password from the registration file and reconnects.
func SignOutAll(ctx context.Context, out io.Writer) error {
	return step(ctx, out, []string{"service", "unset", "password"}, []string{"service", "start"})
}

// ConnectClaude turns on OpenCode's claude-agent provider, which runs the
// real `claude` for Claude models. Until it is connected the provider never
// asks `claude` for its models, so OpenCode lists none. Connecting records
// only that the user chose Claude Code's login; no token is stored. It
// reports whether anything changed; without OpenCode v2 it does nothing.
func ConnectClaude(ctx context.Context) (bool, error) {
	st, err := Inspect(ctx)
	if err != nil || !st.Supported() {
		return false, err
	}
	out, err := run(ctx, st.CLI, "auth", "list")
	if err != nil {
		return false, err
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Claude Agent") {
			return false, nil
		}
	}
	if _, err := run(ctx, st.CLI, "auth", "login", "claude-agent", "--method", "claude-login"); err != nil {
		return false, err
	}
	return true, nil
}

// Pairing is a one-time link that signs in a browser or app.
type Pairing struct {
	URL     string
	Expires time.Time
}

var expiresRE = regexp.MustCompile(`expire in (\d+) minutes?`)

// Pair mints a one-time link on base, the URL devices use to reach the
// server. It starts the server if it isn't running.
func Pair(ctx context.Context, base string) (Pairing, error) {
	bin, err := cli()
	if err != nil {
		return Pairing{}, err
	}
	start := time.Now()
	out, err := run(ctx, bin, "pair", "--url", base)
	if err != nil {
		return Pairing{}, err
	}
	// `opencode pair` prints for people: a sentence with the lifetime,
	// then one link per URL, then a QR code for the OpenCode app.
	p := Pairing{Expires: start.Add(5 * time.Minute)}
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if m := expiresRE.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			p.Expires = start.Add(time.Duration(n) * time.Minute)
		}
		if strings.HasPrefix(line, "http") && strings.Contains(line, "/auth/connect/") && p.URL == "" {
			p.URL = line
		}
	}
	if p.URL == "" {
		return Pairing{}, fmt.Errorf("opencode pair gave no link: %s", errorMessage(string(out), errors.New("no output")))
	}
	return p, nil
}

// LinkBase is the URL pairing links use: the one set in agentctl's config,
// else the address the server listens on.
func LinkBase(configured string, st Status) string {
	if configured != "" {
		return configured
	}
	return st.URL
}
