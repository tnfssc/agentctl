package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/selfupdate"
	"github.com/tnfssc/agentctl/internal/testutil"
)

// fakeSelf stands in for the running binary, started through a link the way
// a version manager installs it.
func fakeSelf(t *testing.T) (link, target string) {
	t.Helper()
	dir := t.TempDir()
	target, link = filepath.Join(dir, "agentctl-v1"), filepath.Join(dir, "agentctl")
	if err := os.WriteFile(target, []byte("bin"), 0o755); err != nil { //nolint:gosec // an executable
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	target, _ = filepath.EvalSymlinks(target) // macOS temp dirs sit behind /var → /private/var
	prev := selfupdate.Executable
	selfupdate.Executable = func() (string, error) { return link, nil }
	t.Cleanup(func() { selfupdate.Executable = prev })
	return link, target
}

func TestUninstallListsWhatStaysAndRemovesItself(t *testing.T) {
	home, url := setup(t)
	testutil.T3(t, home)
	link, target := fakeSelf(t)
	must(t, "proxy", "add", "spark", "--url", url+"/v1", "--key", testutil.Key)
	must(t, "use", "codex", "spark", "--yes")

	if _, err := execute(t, "uninstall"); err == nil {
		t.Fatal("without a terminal, uninstalling needs --yes")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("nothing may be removed without confirmation")
	}

	out := must(t, "uninstall", "--yes")
	for _, want := range []string{
		"Codex routes through spark", "agentctl use codex local",
		"T3 Code's background service", "agentctl t3 uninstall",
		"removed " + target, "Kept ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, p := range []string{target, link} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists", p)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "agentctl", "config.toml")); err != nil {
		t.Fatal("without --purge the config stays")
	}
}

func TestUninstallPurgeRemovesTheConfig(t *testing.T) {
	home, url := setup(t)
	fakeSelf(t)
	must(t, "proxy", "add", "spark", "--url", url+"/v1", "--key", testutil.Key)
	must(t, "uninstall", "--yes", "--purge")
	if _, err := os.Stat(filepath.Join(home, ".config", "agentctl")); !os.IsNotExist(err) {
		t.Fatal("--purge should delete agentctl's config directory")
	}
}
