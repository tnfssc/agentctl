package cliproxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Label names the launchd agent; Unit names the systemd user unit.
const (
	Label = "io.github.tnfssc.agentctl.cliproxyapi"
	Unit  = "agentctl-cliproxyapi.service"
)

// service is the platform's service manager. Both run the server at login
// and restart it when it exits; stopping is kept across logins.
type service interface {
	path() (string, error)
	file(bin, cfg, dir, log string) string
	installed() bool
	enabled(ctx context.Context) bool
	start(ctx context.Context) error
	stop(ctx context.Context) error
	restart(ctx context.Context) error
	noLinger(ctx context.Context) bool
}

func manager() service {
	if runtime.GOOS == "darwin" {
		return launchd{}
	}
	return systemd{}
}

// ServicePath is the launchd plist or systemd unit agentctl writes.
func ServicePath() (string, error) { return manager().path() }

func writeService() error {
	svc := manager()
	path, err := svc.path()
	if err != nil {
		return err
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	bin, _ := Bin()
	cfg, _ := ConfigPath()
	log, _ := LogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // the user's own service dir
		return err
	}
	return os.WriteFile(path, []byte(svc.file(bin, cfg, dir, log)), 0o644) //nolint:gosec // service files are not secret
}

func removeService(ctx context.Context) error {
	svc := manager()
	if !svc.installed() {
		return nil
	}
	if err := svc.stop(ctx); err != nil {
		return err
	}
	path, err := svc.path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if s, ok := svc.(systemd); ok {
		return s.reload(ctx)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// run executes a service manager command and returns its output; failures
// carry its message.
func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // launchctl, systemctl or loginctl with agentctl's own arguments
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), text)
	}
	return text, nil
}

type launchd struct{}

func (launchd) path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

func (launchd) target() string { return "gui/" + strconv.Itoa(os.Getuid()) + "/" + Label }
func (launchd) domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func (launchd) file(bin, cfg, dir, log string) string {
	esc := func(s string) string {
		return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<!-- Managed by agentctl: agentctl cliproxy ... -->
<plist version="1.0">
<dict>
  <key>Label</key><string>` + Label + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + esc(bin) + `</string>
    <string>-config</string>
    <string>` + esc(cfg) + `</string>
  </array>
  <key>WorkingDirectory</key><string>` + esc(dir) + `</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>5</integer>
  <key>StandardOutPath</key><string>` + esc(log) + `</string>
  <key>StandardErrorPath</key><string>` + esc(log) + `</string>
</dict>
</plist>
`
}

func (l launchd) installed() bool {
	p, err := l.path()
	return err == nil && fileExists(p)
}

func (l launchd) enabled(ctx context.Context) bool {
	_, err := run(ctx, "launchctl", "print", l.target())
	return err == nil
}

// start enables the agent, so it also comes back at the next login, and
// loads it.
func (l launchd) start(ctx context.Context) error {
	if _, err := run(ctx, "launchctl", "enable", l.target()); err != nil {
		return err
	}
	if l.enabled(ctx) {
		_, err := run(ctx, "launchctl", "kickstart", l.target())
		return err
	}
	p, err := l.path()
	if err != nil {
		return err
	}
	_, err = run(ctx, "launchctl", "bootstrap", l.domain(), p)
	return err
}

// stop unloads the agent and disables it, so it stays stopped after the
// next login too.
func (l launchd) stop(ctx context.Context) error {
	if l.enabled(ctx) {
		if _, err := run(ctx, "launchctl", "bootout", l.target()); err != nil {
			return err
		}
	}
	_, err := run(ctx, "launchctl", "disable", l.target())
	return err
}

func (l launchd) restart(ctx context.Context) error {
	if !l.enabled(ctx) {
		return l.start(ctx)
	}
	_, err := run(ctx, "launchctl", "kickstart", "-k", l.target())
	return err
}

func (launchd) noLinger(context.Context) bool { return false }

type systemd struct{}

func (systemd) path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "systemd", "user", Unit), nil
}

func (systemd) file(bin, cfg, dir, log string) string {
	// systemd splits ExecStart on spaces unless quoted.
	q := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }
	return `# Managed by agentctl: agentctl cliproxy ...
[Unit]
Description=CLIProxyAPI (agentctl)
After=network-online.target

[Service]
ExecStart=` + q(bin) + ` -config ` + q(cfg) + `
WorkingDirectory=` + q(dir) + `
Restart=always
RestartSec=5
StandardOutput=append:` + log + `
StandardError=append:` + log + `

[Install]
WantedBy=default.target
`
}

func (s systemd) installed() bool {
	p, err := s.path()
	return err == nil && fileExists(p)
}

func (systemd) enabled(ctx context.Context) bool {
	out, _ := run(ctx, "systemctl", "--user", "is-active", Unit)
	return out == "active" || out == "activating" || out == "reloading"
}

func (systemd) reload(ctx context.Context) error {
	_, err := run(ctx, "systemctl", "--user", "daemon-reload")
	return err
}

func (s systemd) start(ctx context.Context) error {
	if err := s.reload(ctx); err != nil {
		return err
	}
	_, err := run(ctx, "systemctl", "--user", "enable", "--now", Unit)
	return err
}

func (systemd) stop(ctx context.Context) error {
	_, err := run(ctx, "systemctl", "--user", "disable", "--now", Unit)
	return err
}

func (s systemd) restart(ctx context.Context) error {
	if err := s.reload(ctx); err != nil {
		return err
	}
	if _, err := run(ctx, "systemctl", "--user", "enable", Unit); err != nil {
		return err
	}
	_, err := run(ctx, "systemctl", "--user", "restart", Unit)
	return err
}

// noLinger reports whether systemd stops this user's services at logout,
// which takes the server down when the last SSH session ends.
func (systemd) noLinger(ctx context.Context) bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	out, err := run(ctx, "loginctl", "show-user", u.Username, "--property=Linger", "--value")
	return err == nil && out == "no"
}
