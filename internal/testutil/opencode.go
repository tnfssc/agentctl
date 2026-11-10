package testutil

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/ocweb"
)

// OpenCodeVersion is the version the fake opencode reports.
const OpenCodeVersion = "2.2.6"

// OpenCodeLatest is the newest release the fake npm registry offers.
const OpenCodeLatest = "2.2.7"

// FakeOpenCode is an `opencode` on PATH whose background server is a file:
// Dir/url holds the address while it runs.
type FakeOpenCode struct {
	Dir   string // state and canned replies; calls are appended to Dir/calls
	State string // XDG_STATE_HOME, where the server's registration goes
}

// OpenCode puts the fake first on a PATH without any real opencode, with
// its server running at http://127.0.0.1:49374, and points the update check
// at a fake npm registry.
func OpenCode(t *testing.T, home string) *FakeOpenCode {
	t.Helper()
	f := &FakeOpenCode{Dir: t.TempDir(), State: filepath.Join(home, ".local", "state")}
	npm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"beta":"0.0.0-beta-19507","latest":"` + OpenCodeLatest + `","dev":"0.0.0-dev-1042"}`))
	}))
	t.Cleanup(npm.Close)
	prev := ocweb.DistTagsURL
	ocweb.DistTagsURL = npm.URL
	t.Cleanup(func() { ocweb.DistTagsURL = prev })
	t.Setenv("XDG_STATE_HOME", f.State)
	bin := filepath.Join(f.Dir, "bin")
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	write(t, filepath.Join(bin, "opencode"), strings.NewReplacer("@DIR@", f.Dir).Replace(fakeOpenCodeScript), 0o755)
	f.Write(t, "version", "opencode v"+OpenCodeVersion)
	f.Write(t, "url", "http://127.0.0.1:49374")
	f.Write(t, "auth", "OpenAI          OAuth                       stored\n")
	f.Register(t, OpenCodeVersion)
	return f
}

// Register writes the running server's registration file as version.
func (f *FakeOpenCode) Register(t *testing.T, version string) {
	t.Helper()
	write(t, filepath.Join(f.State, "opencode", "service.json"),
		`{"id":"x","version":"`+version+`","url":"http://127.0.0.1:49374","pid":1,"password":"secret"}`, 0o600)
}

// Calls returns each opencode invocation's arguments, one line per call.
func (f *FakeOpenCode) Calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.Dir, "calls"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// Write replaces one piece of state: "version" (what --version prints),
// "url" (empty means stopped), "auth" (what `auth list` prints), or "fail", which makes every call but
// --version fail with that message, printed the way opencode prints errors.
func (f *FakeOpenCode) Write(t *testing.T, name, content string) {
	t.Helper()
	write(t, filepath.Join(f.Dir, name), content, 0o600)
}

const fakeOpenCodeScript = `#!/bin/sh
if [ "$*" = "--version" ]; then cat @DIR@/version; echo; exit 0; fi
echo "$*" >> @DIR@/calls
if [ -f @DIR@/fail ]; then
  # How opencode prints an error: the message, then a stack.
  printf 'Error: %s\n    at <anonymous> (/$bunfs/root/chunk.js:3:2011)\n' "$(cat @DIR@/fail)" >&2
  exit 1
fi
url="$(cat @DIR@/url)"
case "$*" in
"service status") echo "${url:-stopped}" ;;
"service start"|"service restart")
  echo "http://127.0.0.1:49374" > @DIR@/url
  cat @DIR@/url ;;
"service stop"|"service unset password") : > @DIR@/url ;;
"auth list") cat @DIR@/auth ;;
"auth login claude-agent --method claude-login")
  echo "Claude Agent    Claude Agent                stored" >> @DIR@/auth
  echo "Connected to Claude Agent" ;;
"upgrade") echo "Upgrade complete" ;;
"pair --url "*)
  echo "http://127.0.0.1:49374" > @DIR@/url
  printf '\n  Open a link to connect. Links work once and expire in 5 minutes.\n\n  %s/auth/connect/CODE22CHARSxxxxxxxxxxx\n\n  ▄▄▄ qr\n\n' "$3" ;;
*) echo "fake opencode: unexpected $*" >&2; exit 1 ;;
esac
`
