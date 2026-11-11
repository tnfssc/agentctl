// Package agent holds what the per-agent packages share: a planned file
// change that can be shown as a diff, backed up and applied.
package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aymanbagabas/go-udiff"

	"github.com/tnfssc/agentctl/internal/config"
)

// Local is the route name for an agent's own login.
const Local = "local"

// Change is one file rewrite. Before is nil when the file does not exist;
// After is nil when the change removes it.
type Change struct {
	Path   string
	Before []byte
	After  []byte
	Perm   fs.FileMode // used only when creating the file
	Quiet  bool        // generated file; summarized instead of diffed
	// Link, when set, makes Path a symlink to Link instead of writing it.
	// Path must not exist yet.
	Link string
	// Agent, when set, files the backup under that agent instead of the
	// plan's, for plans that touch several agents.
	Agent string
}

func (c Change) noop() bool {
	return c.Link == "" && bytes.Equal(c.Before, c.After) && (c.Before == nil) == (c.After == nil)
}

// Run is a step applying a plan takes after writing its files, such as
// installing a tool. Title is what the review shows.
type Run struct {
	Title string
	Do    func(ctx context.Context, out io.Writer) error
}

// Plan is everything one `use` would do to an agent, shown before applying.
type Plan struct {
	Agent    string
	Route    string
	Changes  []Change
	Runs     []Run
	Warnings []string
}

// Empty reports whether applying the plan would change nothing.
func (p Plan) Empty() bool {
	if len(p.Runs) > 0 {
		return false
	}
	for _, c := range p.Changes {
		if !c.noop() {
			return false
		}
	}
	return true
}

// Diff renders a unified diff of every change.
func (p Plan) Diff() string {
	var b strings.Builder
	for _, c := range p.Changes {
		if c.noop() {
			continue
		}
		path := Tilde(c.Path)
		if c.Link != "" {
			fmt.Fprintf(&b, "link %s -> %s\n", path, Tilde(c.Link))
			continue
		}
		if c.After == nil {
			fmt.Fprintf(&b, "remove %s\n", path)
			continue
		}
		if c.Quiet {
			fmt.Fprintf(&b, "write %s (generated, %d bytes)\n", path, len(c.After))
			continue
		}
		b.WriteString(udiff.Unified(path, path, string(c.Before), string(c.After)))
	}
	for _, r := range p.Runs {
		fmt.Fprintf(&b, "run %s\n", r.Title)
	}
	return b.String()
}

// Apply backs up each existing file, then writes the new contents.
func (p Plan) Apply() error {
	for _, c := range p.Changes {
		if c.noop() {
			continue
		}
		if c.Link != "" {
			if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
				return err
			}
			if err := os.Symlink(c.Link, c.Path); err != nil {
				return err
			}
			continue
		}
		if c.Before != nil {
			owner := p.Agent
			if c.Agent != "" {
				owner = c.Agent
			}
			if err := backup(owner, c.Path, c.Before); err != nil {
				return fmt.Errorf("backup %s: %w", c.Path, err)
			}
		}
		if c.After == nil {
			if err := os.Remove(c.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			continue
		}
		perm := c.Perm
		if perm == 0 {
			perm = 0o644
		}
		if err := config.WriteFileAtomic(c.Path, c.After, perm); err != nil {
			return err
		}
	}
	return nil
}

// RunAll takes the plan's steps in order, their output going to out, and
// stops at the first that fails.
func (p Plan) RunAll(ctx context.Context, out io.Writer) error {
	for _, r := range p.Runs {
		if err := r.Do(ctx, out); err != nil {
			return err
		}
	}
	return nil
}

// Tilde shortens a path under the home directory to ~/..., for display.
func Tilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}

// ReadFile returns a file's bytes, or nil if it does not exist.
func ReadFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

const keepBackups = 20

// backup copies data to <config>/backups/<agent>/<timestamp>-<name> and
// prunes all but the newest keepBackups for that agent.
func backup(agentName, path string, data []byte) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "backups", agentName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := time.Now().Format("20060102-150405.000") + "-" + filepath.Base(path)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	for _, e := range entries[min(len(entries), keepBackups):] {
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
	return nil
}

// Executable locates the running binary; tests replace it.
var Executable = os.Executable

// HelperCommand is how agents call back into agentctl for a proxy key, so
// keys never sit in the agents' own config files. It uses the absolute path
// of the running binary because IDEs and services do not share the shell's
// PATH. Symlinks are kept: a version manager's stable link survives upgrades
// where the versioned target would not.
func HelperCommand() (string, error) {
	exe, err := Executable()
	if err != nil {
		return "", err
	}
	if strings.Contains(exe, string(filepath.Separator)+"go-build") {
		return "", errors.New("agentctl is running from `go run`, whose binary is deleted on exit; install it first (`mise run install`)")
	}
	return exe, nil
}
