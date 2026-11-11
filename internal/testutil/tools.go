package testutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/tools"
)

// FakeTools is a PATH with only the given package managers, each of which
// "installs" by creating the tool's command, and a fake GitHub serving the
// skills' repositories.
type FakeTools struct{ Dir string }

// Tools replaces PATH with fakes of the named package managers (uv, pipx,
// brew, npm) plus the system's basics, and lets tools detect for real. Call
// after Home.
func Tools(t *testing.T, managers ...string) *FakeTools {
	t.Helper()
	f := &FakeTools{Dir: t.TempDir()}
	makes := map[string]string{"uv": "tvly", "pipx": "tvly", "brew": "mcpc", "npm": "mcpc"}
	for _, m := range managers {
		write(t, filepath.Join(f.Dir, m), "#!/bin/sh\necho \""+m+" $*\" >> "+filepath.Join(f.Dir, "calls")+
			"\nprintf '#!/bin/sh\\n' > "+filepath.Join(f.Dir, makes[m])+"\nchmod +x "+filepath.Join(f.Dir, makes[m])+"\n", 0o755)
	}
	t.Setenv("PATH", f.Dir+":/usr/bin:/bin")
	prev, prevURL := tools.Inspect, tools.Codeload
	tools.Inspect = tools.Detect
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		files := map[string]map[string]string{
			"/tavily-ai/skills/tar.gz/HEAD": {
				"skills-abc/README.md":                                     "repo readme",
				"skills-abc/skills/core/tavily-search/SKILL.md":            "# tavily-search",
				"skills-abc/skills/core/tavily-search/references/setup.md": "setup",
				"skills-abc/skills/core/tavily-search/../../../../evil":    "outside",
			},
			"/apify/mcpc/tar.gz/HEAD": {"mcpc-abc/skills/mcpc/SKILL.md": "# mcpc"},
		}[r.URL.Path]
		if files == nil {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(tarball(t, files))
	}))
	t.Cleanup(srv.Close)
	tools.Codeload = srv.URL
	t.Cleanup(func() { tools.Inspect, tools.Codeload = prev, prevURL })
	return f
}

// Calls returns each package manager run, one line per call.
func (f *FakeTools) Calls() []string {
	b, err := os.ReadFile(filepath.Join(f.Dir, "calls"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
