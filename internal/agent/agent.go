// Package agent holds what the per-agent packages share: a planned file
// change that can be shown as a diff, backed up and applied.
package agent

import (
	"bytes"
	"errors"
	"fmt"
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

// Change is one file rewrite. Before is nil when the file does not exist.
type Change struct {
	Path   string
	Before []byte
	After  []byte
	Perm   fs.FileMode // used only when creating the file
	Quiet  bool        // generated file; summarized instead of diffed
	// Link, when set, makes Path a symlink to Link instead of writing it.
	// Path must not exist yet.
	Link string
}

func (c Change) noop() bool { return c.Link == "" && bytes.Equal(c.Before, c.After) }

// Plan is everything one `use` would do to an agent, shown before applying.
type Plan struct {
	Agent    string
	Route    string
	Changes  []Change
	Warnings []string
}

// Empty reports whether applying the plan would change nothing.
func (p Plan) Empty() bool {
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
		if c.Link != "" {
			fmt.Fprintf(&b, "link %s -> %s\n", c.Path, c.Link)
			continue
		}
		if c.Quiet {
			fmt.Fprintf(&b, "write %s (generated, %d bytes)\n", c.Path, len(c.After))
			continue
		}
		b.WriteString(udiff.Unified(c.Path, c.Path, string(c.Before), string(c.After)))
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
			if err := backup(p.Agent, c.Path, c.Before); err != nil {
				return fmt.Errorf("backup %s: %w", c.Path, err)
			}
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
