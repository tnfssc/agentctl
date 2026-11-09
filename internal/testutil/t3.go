package testutil

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T3Version is the version the fake T3 Code service runs.
const T3Version = "0.1.8-nightly.20261106.4262"

// T3Nightly is the newest nightly the fake registry offers.
const T3Nightly = "0.1.8-nightly.20261108.4380"

// FakeT3 is a T3 Code install under the test home: a service runtime whose
// t3 answers from canned JSON, and a server answering the environment probe
// and npm's dist-tags.
type FakeT3 struct {
	Home   string // T3CODE_HOME
	Dir    string // canned replies; calls are appended to Dir/calls
	Server *httptest.Server
}

// T3 installs the fake under home (from Home) and hides any real t3 on
// PATH. Sessions and links start empty; write Dir/sessions.json and
// Dir/links.json to change that.
func T3(t *testing.T, home string) *FakeT3 {
	t.Helper()
	f := &FakeT3{Home: filepath.Join(home, ".t3"), Dir: t.TempDir()}
	t.Setenv("T3CODE_HOME", f.Home)
	t.Setenv("PATH", "/usr/bin:/bin")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/t3/environment", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"label":"testbox","serverVersion":"` + T3Version + `"}`))
	})
	mux.HandleFunc("GET /dist-tags", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"latest":"0.1.7","nightly":"` + T3Nightly + `"}`))
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)

	f.writeCLI(t, filepath.Join(f.Home, "runtime", "versions", T3Version, "t3"), T3Version, "")
	write(t, filepath.Join(f.Home, "runtime", "service-state.json"),
		`{"protocol":3,"activeVersion":"`+T3Version+`"}`, 0o600)
	write(t, filepath.Join(f.Home, "userdata", "server-runtime.json"),
		`{"version":1,"pid":1,"origin":"`+f.Server.URL+`","serviceManaged":true}`, 0o600)
	write(t, filepath.Join(f.Dir, "status.json"),
		`{"service":{"supported":true,"installed":true,"installedVersion":"`+T3Version+`"}}`, 0o600)
	write(t, filepath.Join(f.Dir, "links.json"), "[]", 0o600)
	write(t, filepath.Join(f.Dir, "sessions.json"), "[]", 0o600)
	return f
}

// AddCLI puts another fake t3 at path, e.g. a newer one on PATH. Its calls
// are logged as "<tag>: <args>".
func (f *FakeT3) AddCLI(t *testing.T, path, version, tag string) {
	t.Helper()
	f.writeCLI(t, path, version, tag+": ")
}

func (f *FakeT3) writeCLI(t *testing.T, path, version, tag string) {
	t.Helper()
	script := strings.NewReplacer("@DIR@", f.Dir, "@VERSION@", version, "@TAG@", tag).Replace(fakeT3Script)
	write(t, path, script, 0o755)
}

// Calls returns each t3 invocation's arguments, one line per call.
func (f *FakeT3) Calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.Dir, "calls"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// Write replaces one canned reply. Writing "fail" makes every call fail
// with that message, printed the way t3 prints user errors; writing
// "nostatus" makes `status` unknown, as before t3 0.1.8.
func (f *FakeT3) Write(t *testing.T, name, content string) {
	t.Helper()
	write(t, filepath.Join(f.Dir, name), content, 0o600)
}

func write(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}

const fakeT3Script = `#!/bin/sh
if [ "$*" = "--version" ]; then echo "t3 v@VERSION@"; exit 0; fi
echo "@TAG@$*" >> @DIR@/calls
if [ -f @DIR@/nostatus ] && [ "$1" = "status" ]; then
  printf '\nERROR\n  Unrecognized flag: --json in command t3\n' >&2
  exit 1
fi
if [ -f @DIR@/fail ]; then
  # How t3 prints a user error.
  printf '\nERROR\n  %s\n' "$(cat @DIR@/fail)" >&2
  exit 1
fi
case "$*" in
"status --json") cat @DIR@/status.json ;;
"auth pairing list --json") cat @DIR@/links.json ;;
"auth session list --json") cat @DIR@/sessions.json ;;
"auth pairing create"*)
  base=""
  while [ $# -gt 0 ]; do
    if [ "$1" = "--base-url" ]; then base="$2"; fi
    shift
  done
  printf '{"id":"new-link","credential":"TOKEN12CHARS","expiresAt":"2030-01-01T00:05:00Z","pairUrl":"%s/pair#token=TOKEN12CHARS"}\n' "$base" ;;
"auth "*" revoke "*) echo "Revoked." ;;
"update"*|"service "*) echo "ok: $*" ;;
*) echo "fake t3: unexpected $*" >&2; exit 1 ;;
esac
`
