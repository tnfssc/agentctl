// Package t3 manages a T3 Code server on this machine by driving the
// upstream `t3` CLI: its background service, updates on the nightly channel,
// pairing links and sessions. agentctl adds nothing T3 Code doesn't already
// do; it reads the state `t3` leaves on disk and calls `t3` for every change,
// so desktop-triggered updates and upstream fixes keep working.
package t3

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
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Channel is the release train agentctl installs and updates to.
const Channel = "nightly"

// IdleAfter is how long a session can go unseen before it counts as idle.
const IdleAfter = 7 * 24 * time.Hour

// Endpoints, variables so tests can point them at fakes.
var (
	InstallScriptURL = "https://t3.codes/install.sh"
	DistTagsURL      = "https://registry.npmjs.org/-/package/t3/dist-tags"
	client           = &http.Client{Timeout: 5 * time.Second}
)

// ErrNotInstalled means there is no t3 CLI on this machine.
var ErrNotInstalled = errors.New("T3 Code is not installed; run `agentctl t3 install`")

// Home is T3 Code's data directory: $T3CODE_HOME, else ~/.t3.
func Home() (string, error) {
	if h := strings.TrimSpace(os.Getenv("T3CODE_HOME")); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".t3"), nil
}

// serviceState is <home>/runtime/service-state.json, written by t3's
// service launcher.
type serviceState struct {
	ActiveVersion string `json:"activeVersion"`
	Update        *struct {
		FromVersion   string `json:"fromVersion"`
		TargetVersion string `json:"targetVersion"`
		Status        string `json:"status"`
		Reason        string `json:"reason"`
	} `json:"update"`
}

// serverRuntime is <home>/userdata/server-runtime.json, written by a
// running server.
type serverRuntime struct {
	PID            int    `json:"pid"`
	Origin         string `json:"origin"`
	ServiceManaged bool   `json:"serviceManaged"`
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// runtimeBinary is the service's own runtime, if the service has one.
func runtimeBinary(home string) string {
	var st serviceState
	if readJSON(filepath.Join(home, "runtime", "service-state.json"), &st) != nil || st.ActiveVersion == "" {
		return ""
	}
	bin := filepath.Join(home, "runtime", "versions", st.ActiveVersion, "t3")
	if isExec(bin) {
		return bin
	}
	return ""
}

func isExec(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// cli is the t3 to run: the service's own runtime first, then the one on
// PATH or where the installer puts it, then the desktop app's shim.
//
// The service's runtime comes first because the others fall behind it: a
// desktop-triggered update moves only the service. An older CLI refuses
// `service install` as a downgrade, and `t3 auth` migrates the database to
// its own version, so every call should be the service's version.
func cli(home string) (string, error) {
	if bin := runtimeBinary(home); bin != "" {
		return bin, nil
	}
	if p, err := exec.LookPath("t3"); err == nil {
		return p, nil
	}
	userHome, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(userHome, ".local", "bin", "t3"), filepath.Join(home, "bin", "t3")} {
		if isExec(p) {
			return p, nil
		}
	}
	return "", ErrNotInstalled
}

// run executes t3 with T3CODE_HOME pinned and returns stdout.
func run(ctx context.Context, bin, home string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // t3 found by cli, with fixed arguments
	cmd.Env = append(os.Environ(), "T3CODE_HOME="+home, "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("t3 %s: %s", strings.Join(args, " "), errorMessage(stderr.String()+"\n"+stdout.String(), err))
	}
	return stdout.Bytes(), nil
}

// stream executes t3 with its output going to out, for long steps a person
// may watch. Failures carry t3's message even when out discards it.
func stream(ctx context.Context, out io.Writer, bin, home string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // t3 found by cli, with fixed arguments
	cmd.Env = append(os.Environ(), "T3CODE_HOME="+home)
	var buf bytes.Buffer
	w := io.MultiWriter(out, &buf)
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("t3 %s: %s", strings.Join(args, " "), errorMessage(ansi.Strip(buf.String()), err))
	}
	return nil
}

// errorMessage finds t3's error in its output. User errors print "ERROR" on
// a line of its own and the message under it; other errors are logged as
// "[12:00:00.000] ERROR (#2): NoRunningServerError: message". Without
// either, the last line is the best guess, then the exit status.
func errorMessage(output string, err error) string {
	var lines []string
	for line := range strings.SplitSeq(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	for i, line := range lines {
		switch {
		case line == "ERROR" && i+1 < len(lines):
			return lines[i+1]
		case strings.HasPrefix(line, "[") && strings.Contains(line, "] ERROR ("):
			_, msg, _ := strings.Cut(line, "): ")
			if name, rest, ok := strings.Cut(msg, ": "); ok && strings.HasSuffix(name, "Error") && !strings.Contains(name, " ") {
				msg = rest
			}
			return msg
		}
	}
	if len(lines) > 0 {
		return lines[len(lines)-1]
	}
	return err.Error()
}

// Status is what agentctl shows about T3 Code on this machine.
type Status struct {
	Home string
	CLI  string // t3 binary, "" when not installed

	ServiceSupported bool
	ServiceInstalled bool
	ServiceVersion   string // version the service is set to run

	Running bool
	Version string // version actually running
	Label   string // the environment's name
	Origin  string // where the server listens

	// Update is the last update the service launcher ran, e.g. one a
	// desktop app asked for.
	Update, UpdateReason string

	Latest    string // newest nightly, "" if unknown
	LatestErr error
}

// Installed reports whether a t3 CLI exists.
func (s Status) Installed() bool { return s.CLI != "" }

// CurrentVersion is the version running, else the one the service is set
// to run, else "".
func (s Status) CurrentVersion() string {
	if s.Version != "" {
		return s.Version
	}
	return s.ServiceVersion
}

// UpdateAvailable reports whether a newer nightly exists than what runs.
func (s Status) UpdateAvailable() bool {
	cur := s.CurrentVersion()
	return s.Latest != "" && cur != "" && CompareVersions(s.Latest, cur) > 0
}

// OnLatest reports whether what runs is known to be the newest nightly.
func (s Status) OnLatest() bool {
	return s.Latest != "" && s.CurrentVersion() != "" && !s.UpdateAvailable()
}

// Inspect reads T3 Code's state. It never changes anything.
func Inspect(ctx context.Context) (Status, error) {
	home, err := Home()
	if err != nil {
		return Status{}, err
	}
	s := Status{Home: home}
	s.Latest, s.LatestErr = latest(ctx)
	if s.CLI, _ = cli(home); s.CLI == "" {
		return s, nil // not installed is a state, not an error
	}

	out, err := run(ctx, s.CLI, home, "status", "--json")
	if err != nil {
		return s, err
	}
	var st struct {
		Service struct {
			Supported        bool   `json:"supported"`
			Installed        bool   `json:"installed"`
			InstalledVersion string `json:"installedVersion"`
			OtherBaseDir     string `json:"otherBaseDir"`
		} `json:"service"`
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return s, fmt.Errorf("t3 status: %w", err)
	}
	s.ServiceSupported, s.ServiceVersion = st.Service.Supported, st.Service.InstalledVersion
	// t3 reports a service installed for another T3 home as installed; one
	// with no version for this home is not ours.
	s.ServiceInstalled = st.Service.Installed && st.Service.InstalledVersion != "" && st.Service.OtherBaseDir == ""

	var svc serviceState
	if readJSON(filepath.Join(home, "runtime", "service-state.json"), &svc) == nil && svc.Update != nil {
		s.Update = fmt.Sprintf("%s → %s %s", svc.Update.FromVersion, svc.Update.TargetVersion, svc.Update.Status)
		s.UpdateReason = svc.Update.Reason
	}

	var rt serverRuntime
	if err := readJSON(filepath.Join(home, "userdata", "server-runtime.json"), &rt); err == nil && rt.Origin != "" {
		s.Origin = rt.Origin
		s.Running, s.Version, s.Label = probe(ctx, rt.Origin)
	}
	return s, nil
}

// probe asks a server who it is.
func probe(ctx context.Context, origin string) (ok bool, version, label string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(origin, "/")+"/.well-known/t3/environment", nil)
	if err != nil {
		return false, "", ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", ""
	}
	defer func() { _ = resp.Body.Close() }()
	var env struct {
		Label         string `json:"label"`
		ServerVersion string `json:"serverVersion"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&env) != nil {
		return false, "", ""
	}
	return true, env.ServerVersion, env.Label
}

// latest is the newest nightly on npm, where t3's own update check looks.
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
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&tags) != nil || tags[Channel] == "" {
		return "", fmt.Errorf("checking for updates: %s gave no %s version", DistTagsURL, Channel)
	}
	return tags[Channel], nil
}

// Install downloads T3 Code's installer and runs it for the nightly channel,
// then installs the background service. Output goes to out.
func Install(ctx context.Context, out io.Writer) error {
	home, err := Home()
	if err != nil {
		return err
	}
	if _, err := cli(home); errors.Is(err, ErrNotInstalled) {
		if err := runInstaller(ctx, out, home); err != nil {
			return err
		}
	}
	bin, err := cli(home)
	if err != nil {
		return err
	}
	return stream(ctx, out, bin, home, "service", "install")
}

func runInstaller(ctx context.Context, out io.Writer, home string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, InstallScriptURL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("downloading the T3 Code installer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading the T3 Code installer: %s", resp.Status)
	}
	script, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("downloading the T3 Code installer: %w", err)
	}
	cmd := exec.CommandContext(ctx, "sh", "-s")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Env = append(os.Environ(), "T3CODE_CHANNEL="+Channel, "T3CODE_HOME="+home)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("the T3 Code installer failed: %w", err)
	}
	return nil
}

// Update moves t3 and the service to the newest nightly. The service
// restarts, which stops running agent turns.
func Update(ctx context.Context, out io.Writer) error {
	home, bin, err := setupCLI()
	if err != nil {
		return err
	}
	return stream(ctx, out, bin, home, "update", "--channel", Channel, "--yes")
}

// Restart restarts the background service, which stops running agent turns.
func Restart(ctx context.Context, out io.Writer) error {
	home, bin, err := setupCLI()
	if err != nil {
		return err
	}
	return stream(ctx, out, bin, home, "service", "restart")
}

func setupCLI() (home, bin string, err error) {
	if home, err = Home(); err != nil {
		return "", "", err
	}
	bin, err = cli(home)
	return home, bin, err
}
