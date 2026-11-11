// Package tools installs the command line tools the shared Web and MCP
// instructions point agents at: Tavily's `tvly` for web search and `mcpc`
// for MCP servers, each with its skill. Only the CLI from its official
// package and one skill folder each, never the vendors' init scripts, which
// add plugins and MCP servers. The skills go to the shared
// ~/.agents/skills, which Codex, Pi and OpenCode read; Claude Code gets a
// link. API keys stay the user's to enter (`tvly login`); Tavily searches
// without one, rate-capped. `agentctl guide tools` covers the rest
// (Firecrawl, keys, other skill folders) and `tools-undo` removes it all.
package tools

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/tnfssc/agentctl/internal/agent"
	"github.com/tnfssc/agentctl/internal/guide"
)

// Name is the plan name for installing them alone, Title its name for
// people.
const (
	Name  = "tools"
	Title = "Web & MCP tools"
)

// Codeload serves GitHub repositories as tarballs; tests point it at a fake.
var Codeload = "https://codeload.github.com"

var client = &http.Client{Timeout: 60 * time.Second}

// skill is a skill folder in a GitHub repository.
type skill struct{ name, repo, dir string }

var (
	tavilySkill = skill{"tavily-search", "tavily-ai/skills", "skills/core/tavily-search"}
	mcpcSkill   = skill{"mcpc", "apify/mcpc", "skills/mcpc"}
)

// Status is what is installed.
type Status struct {
	Web    string // tvly or firecrawl, "" when neither is on PATH
	MCPC   bool
	Skills []string // skills Claude Code doesn't have, for the installed or planned CLIs
}

// Ready reports whether agents can follow the Web and MCP instructions.
func (s Status) Ready() bool { return s.Web != "" && s.MCPC && len(s.Skills) == 0 }

// Summary says it in a few words.
func (s Status) Summary() string {
	var missing []string
	if s.Web == "" {
		missing = append(missing, "web search CLI")
	}
	if !s.MCPC {
		missing = append(missing, "mcpc")
	}
	if len(s.Skills) > 0 {
		missing = append(missing, "skills")
	}
	if len(missing) == 0 {
		return "installed"
	}
	return "missing " + strings.Join(missing, ", ")
}

// Inspect is Detect; tests replace it so nothing on the real PATH gets
// planned, let alone run.
var Inspect = Detect

// Detect looks at PATH and the skill folders. It changes nothing.
func Detect() Status {
	var s Status
	for _, bin := range []string{"tvly", "firecrawl"} {
		if _, err := exec.LookPath(bin); err == nil {
			s.Web = bin
			break
		}
	}
	_, err := exec.LookPath("mcpc")
	s.MCPC = err == nil
	webSkill := tavilySkill.name
	if s.Web == "firecrawl" {
		webSkill = "firecrawl"
	}
	// Claude Code's folder is the one to check: Codex, Pi and OpenCode read
	// the shared one, which Claude Code gets a link to.
	_, claude, err := skillDirs()
	if err != nil {
		return s
	}
	for _, name := range []string{webSkill, mcpcSkill.name} {
		if !lexists(filepath.Join(claude, name)) {
			s.Skills = append(s.Skills, name)
		}
	}
	return s
}

// Plan lists what installing would do. Warnings say what can't be done
// here, e.g. no uv or pipx for Tavily's CLI.
func Plan() (runs []agent.Run, warnings []string) {
	s := Inspect()
	if s.Web == "" {
		switch run, err := pkgRun("tavily-cli", []installer{
			{"uv", []string{"tool", "install", "tavily-cli"}}, {"pipx", []string{"install", "tavily-cli"}},
		}); {
		case err != nil:
			warnings = append(warnings, "Tavily's CLI needs uv or pipx; install uv (https://docs.astral.sh/uv), then apply again")
		default:
			runs = append(runs, run)
		}
	}
	if !s.MCPC {
		var opts []installer
		if runtime.GOOS == "darwin" {
			opts = append(opts, installer{"brew", []string{"install", "mcpc"}})
		}
		opts = append(opts, installer{"npm", []string{"install", "-g", "@apify/mcpc"}})
		switch run, err := pkgRun("mcpc", opts); {
		case err != nil:
			warnings = append(warnings, "mcpc needs Homebrew or npm; install one, then apply again")
		default:
			runs = append(runs, run)
		}
	}
	for _, sk := range []skill{tavilySkill, mcpcSkill} {
		// Firecrawl's skill comes with `agentctl guide tools`.
		if slices.Contains(s.Skills, sk.name) {
			if r, err := skillRun(sk); err == nil {
				runs = append(runs, r)
			}
		}
	}
	return runs, warnings
}

// installer is a package manager and its install arguments.
type installer struct {
	bin  string
	args []string
}

// pkgRun installs a CLI with the first package manager on PATH.
func pkgRun(what string, opts []installer) (agent.Run, error) {
	for _, o := range opts {
		bin, args := o.bin, o.args
		path, err := exec.LookPath(bin)
		if err != nil {
			continue
		}
		title := bin + " " + strings.Join(args, " ")
		return agent.Run{Title: title, Do: func(ctx context.Context, out io.Writer) error {
			if err := command(ctx, out, path, args...); err != nil {
				return fmt.Errorf("installing %s: %s: %w", what, title, err)
			}
			return record("- " + what + ": `" + title + "`")
		}}, nil
	}
	return agent.Run{}, errors.New("no package manager")
}

// skillRun links a skill from the shared folder for Claude Code, first
// putting it there when it's missing. A skill already in the shared folder
// is the user's and is never replaced.
func skillRun(sk skill) (agent.Run, error) {
	shared, claude, err := skillDirs()
	if err != nil {
		return agent.Run{}, err
	}
	dst, link := filepath.Join(shared, sk.name), filepath.Join(claude, sk.name)
	haveShared := exists(dst)
	title := "link " + agent.Tilde(link)
	if !haveShared {
		title = "skill " + sk.name + " from github.com/" + sk.repo + " into " + agent.Tilde(dst) + ", " + title
	}
	return agent.Run{Title: title, Do: func(ctx context.Context, _ io.Writer) error {
		if !haveShared {
			if err := fetchSkill(ctx, sk, dst); err != nil {
				return fmt.Errorf("skill %s: %w", sk.name, err)
			}
			if err := record("- skill `" + agent.Tilde(dst) + "` from github.com/" + sk.repo + " `" + sk.dir + "`"); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(claude, 0o750); err != nil {
			return err
		}
		if err := os.Symlink(dst, link); err != nil {
			return err
		}
		return record("- link `" + agent.Tilde(link) + "` -> `" + agent.Tilde(dst) + "`")
	}}, nil
}

// skillDirs are the shared skills folder and Claude Code's.
func skillDirs() (shared, claude string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		claudeDir = filepath.Join(home, ".claude")
	}
	return filepath.Join(home, ".agents", "skills"), filepath.Join(claudeDir, "skills"), nil
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func lexists(p string) bool { _, err := os.Lstat(p); return err == nil }

// fetchSkill extracts the skill folder from the repository's default branch
// into dst, through a temp dir so a failure leaves nothing half-written.
func fetchSkill(ctx context.Context, sk skill, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Codeload+"/"+sk.repo+"/tar.gz/HEAD", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github.com/%s: %s", sk.repo, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dst), "."+sk.name+"-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	found := false
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		// Entries are "<repo>-<sha>/<path>"; keep those under the skill's dir.
		_, rel, _ := strings.Cut(h.Name, "/")
		rel, ok := strings.CutPrefix(path.Clean(rel), sk.dir+"/")
		if !ok || h.Typeflag != tar.TypeReg || !filepath.IsLocal(rel) {
			continue
		}
		p := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec // inside our temp dir, checked IsLocal
		if err != nil {
			return err
		}
		_, err = io.Copy(f, io.LimitReader(tr, 10<<20))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		found = found || rel == "SKILL.md"
	}
	if !found {
		return fmt.Errorf("no %s/SKILL.md in github.com/%s", sk.dir, sk.repo)
	}
	return os.Rename(tmp, dst)
}

// command runs a package manager, its output going to out. Failures carry
// its last lines even when out discards them.
func command(ctx context.Context, out io.Writer, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // a package manager from PATH, fixed arguments
	var buf strings.Builder
	w := io.MultiWriter(out, &buf)
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		var lines []string
		for l := range strings.SplitSeq(buf.String(), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) == 0 {
			return err
		}
		return errors.New(strings.Join(lines[max(0, len(lines)-2):], " · "))
	}
	return nil
}

// record adds a line to the guide record `agentctl guide tools-undo` reads,
// starting it when there is none.
func record(line string) error {
	dir, err := guide.RecordsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p := filepath.Join(dir, "tools.md")
	var head string
	if !exists(p) {
		head = "# Web search and MCP tools\n\nInstalled by agentctl, " +
			time.Now().Format("2006-01-02") + ". No sign-ins set up.\n\n"
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // agentctl's own records dir
	if err != nil {
		return err
	}
	_, err = f.WriteString(head + line + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
