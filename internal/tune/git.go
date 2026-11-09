package tune

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Git is the tune target for git settings that make agents' git work go
// smoothly. They go in the user's global git config, which every agent's git
// reads.
const Git = "git"

// GitTitle names the target for people.
const GitTitle = "Git"

// GitConfigPath is the global git config file git itself writes:
// $GIT_CONFIG_GLOBAL, else ~/.gitconfig, unless only the XDG one exists.
func GitConfigPath() (string, error) {
	if p := os.Getenv("GIT_CONFIG_GLOBAL"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dot := filepath.Join(home, ".gitconfig")
	if _, err := os.Stat(dot); err == nil {
		return dot, nil
	}
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	if p := filepath.Join(xdg, "git", "config"); fileExists(p) {
		return p, nil
	}
	return dot, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p) //nolint:gosec // the user's own config paths
	return err == nil
}

// gitDoc edits a copy of the git config with `git config --file`, so git
// keeps its own format; reading the copy back gives the new contents.
type gitDoc struct{ path string }

func newGitDoc(src []byte) (*gitDoc, error) {
	f, err := os.CreateTemp("", "agentctl-gitconfig-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(src); err != nil {
		return nil, err
	}
	return &gitDoc{path: f.Name()}, nil
}

func (d *gitDoc) git(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"config", "--file", d.path}, args...)...) //nolint:gosec // fixed git config arguments
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("git is not installed")
		}
		return "", fmt.Errorf("git config %s: %w %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func (d *gitDoc) holds(op Op) bool {
	v, err := d.git("--get", op.Path)
	return err == nil && v == fmt.Sprint(op.Value)
}

func (d *gitDoc) apply(op Op) error {
	_, err := d.git(op.Path, fmt.Sprint(op.Value))
	return err
}

// reset unsets the key, and drops its section if that left it empty, so a
// file agentctl added to goes back to what it was.
func (d *gitDoc) reset(op Op) error {
	if v, _ := d.git("--get", op.Path); v == "" {
		return nil // not set
	}
	if _, err := d.git("--unset-all", op.Path); err != nil {
		return err
	}
	section := op.Path[:strings.LastIndexByte(op.Path, '.')]
	if rest, _ := d.git("--get-regexp", "^"+strings.ReplaceAll(section, ".", `\.`)+`\.`); rest == "" {
		_, _ = d.git("--remove-section", section)
	}
	return nil
}

func (d *gitDoc) bytes() []byte {
	b, _ := os.ReadFile(d.path)
	return b
}

func (d *gitDoc) close() { _ = os.Remove(d.path) }

// SigningStatus reports whether commits are signed and with which key, from
// the git config git itself sees.
func SigningStatus() (signed bool, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	get := func(k string) string {
		out, err := exec.CommandContext(ctx, "git", "config", "--global", "--get", k).Output() //nolint:gosec // fixed keys
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return get("commit.gpgsign") == "true", get("user.signingkey")
}

var gitItems = []Item{
	{
		ID: "merge-pull", Title: "Merge when pulling", Recommended: true,
		Why:  "git pull fast-forwards when it can and merges when it can't, so an agent never ends up halfway through a rebase.",
		Cost: "merge commits on branches that diverged",
		Ops:  []Op{set("pull.rebase", "false"), set("pull.ff", "true")},
	},
	{
		ID: "push-upstream", Title: "First push just works", Recommended: true,
		Why:  "Pushing a new branch sets its upstream, so an agent's first push needs no -u.",
		Cost: "none",
		Ops:  []Op{set("push.autoSetupRemote", "true")},
	},
	{
		ID: "zdiff3", Title: "Conflicts show the common base", Recommended: true,
		Why:  "Conflict markers include what both sides started from, so agents resolve them by intent, not by guessing.",
		Cost: "longer conflict blocks",
		Ops:  []Op{set("merge.conflictStyle", "zdiff3")},
	},
	{
		ID: "rerere", Title: "Remember conflict resolutions", Recommended: true,
		Why:  "A conflict resolved once is resolved again the same way, e.g. when main is merged in again.",
		Cost: "a wrong resolution repeats until `git rerere forget <path>`",
		Ops:  []Op{set("rerere.enabled", "true")},
	},
	{
		ID: "histogram", Title: "Clearer diffs", Recommended: true,
		Why:  "Diffs line up around lines that occur once, so moved or rewritten code reads as it changed.",
		Cost: "none",
		Ops:  []Op{set("diff.algorithm", "histogram")},
	},
	{
		ID: "prune", Title: "Forget deleted remote branches", Recommended: true,
		Why:  "Fetching drops branches deleted on the remote, so agents don't pick up stale ones.",
		Cost: "none; your local branches stay",
		Ops:  []Op{set("fetch.prune", "true")},
	},
}
