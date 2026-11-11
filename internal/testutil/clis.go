package testutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/upgrade"
)

// FakeCLIs are fake claude, codex and pi commands and npm dist-tags for
// them: Claude Code has a newer release, Codex and Pi are current, and Codex's
// updater fails.
type FakeCLIs struct{ Dir string }

// AgentCLIs puts the fakes first on PATH and points upgrade at a fake
// registry.
func AgentCLIs(t *testing.T) *FakeCLIs {
	t.Helper()
	f := &FakeCLIs{Dir: t.TempDir()}
	versions := map[string]string{"claude": "2.1.310 (Claude Code)", "codex": "codex-cli 0.164.0", "pi": "1.4.0"}
	for name, v := range versions {
		script := "#!/bin/sh\n[ \"$1\" = --version ] && { echo '" + v + "'; exit 0; }\necho \"" + name + " $*\" >> " +
			filepath.Join(f.Dir, "calls") + "\n"
		if name == "codex" {
			script += "echo 'npm ERR! EACCES: permission denied'; exit 1\n"
		}
		write(t, filepath.Join(f.Dir, name), script, 0o755)
	}
	t.Setenv("PATH", f.Dir+":"+os.Getenv("PATH"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		latest := map[string]string{
			"/-/package/@anthropic-ai%2Fclaude-code/dist-tags":       "2.1.320",
			"/-/package/@openai%2Fcodex/dist-tags":                   "0.164.0",
			"/-/package/@earendil-works%2Fpi-coding-agent/dist-tags": "1.4.0",
		}[r.URL.EscapedPath()]
		if latest == "" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"latest":"`+latest+`"}`)
	}))
	t.Cleanup(srv.Close)
	prev := upgrade.Registry
	upgrade.Registry = srv.URL
	t.Cleanup(func() { upgrade.Registry = prev })
	return f
}

// Calls returns each updater run, one line per call.
func (f *FakeCLIs) Calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.Dir, "calls"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}
