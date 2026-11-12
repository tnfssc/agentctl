package testutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tnfssc/agentctl/internal/cliproxy"
)

// CLIProxyTag is the newest release the fake GitHub offers.
const CLIProxyTag = "v8.0.67"

// FakeCLIProxy is a fake GitHub release of CLIProxyAPI and fake launchctl,
// systemctl and loginctl on PATH. The "server" is a file: it runs while
// Dir/running exists, and only then answers /healthz.
type FakeCLIProxy struct {
	Dir    string // state; service manager and server calls are appended to Dir/calls
	Server *httptest.Server
}

// fakeExample is a cut-down v8 config.example.yaml.
const fakeExample = `config-version: 8

# Listener
server:
  # Server host/interface to bind to.
  host: ""
  # Server port
  port: 8317

management:
  allow-remote: false
  # Management key. If a plaintext value is provided here, it will be hashed on startup.
  secret-key: ""

# Client authentication for the proxy API.
access:
  api-keys:
    - "your-api-key-1"
    - "your-api-key-2"

oauth:
  auth-dir: "~/.cli-proxy-api"
`

// CLIProxy installs the fakes. Releases come from Server; the service
// managers and the server binary record their calls.
func CLIProxy(t *testing.T) *FakeCLIProxy {
	t.Helper()
	f := &FakeCLIProxy{Dir: t.TempDir()}
	binScript := "#!/bin/sh\necho \"CLIProxyAPI Version: 8.0.67\"\necho \"cli-proxy-api $*\" >> " + filepath.Join(f.Dir, "calls") +
		"\necho 'Visit the following URL to continue authentication:'\necho 'https://auth.example/login'\n"
	archive := releaseTarball(t, map[string]string{"cli-proxy-api": binScript, "config.example.yaml": fakeExample, "README.md": "x"})
	sum := sha256.Sum256(archive)
	mux := http.NewServeMux()
	mux.HandleFunc("HEAD /releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://github.com/router-for-me/CLIProxyAPI/releases/tag/"+CLIProxyTag)
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("GET /releases/download/"+CLIProxyTag+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		asset, _ := cliproxy.Asset(strings.TrimPrefix(CLIProxyTag, "v"))
		_, _ = w.Write([]byte("ffff  other.tar.gz\n" + hex.EncodeToString(sum[:]) + "  " + asset + "\n"))
	})
	mux.HandleFunc("GET /releases/download/"+CLIProxyTag+"/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) })
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)

	prevRepo, prevHealthy := cliproxy.Repo, cliproxy.SetHealthy(func(context.Context, string) bool {
		_, err := os.Stat(filepath.Join(f.Dir, "running"))
		return err == nil
	})
	cliproxy.Repo = f.Server.URL
	t.Cleanup(func() {
		cliproxy.Repo = prevRepo
		cliproxy.SetHealthy(prevHealthy)
	})

	bin := filepath.Join(f.Dir, "bin")
	script := strings.ReplaceAll(fakeServiceScript, "@DIR@", f.Dir)
	for _, name := range []string{"launchctl", "systemctl", "loginctl"} {
		write(t, filepath.Join(bin, name), script, 0o755)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	return f
}

// Running reports whether the fake server is up.
func (f *FakeCLIProxy) Running() bool {
	_, err := os.Stat(filepath.Join(f.Dir, "running"))
	return err == nil
}

// Calls returns each recorded call, one line per call.
func (f *FakeCLIProxy) Calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.Dir, "calls"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func releaseTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
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

// One script plays launchctl, systemctl and loginctl; the server runs while
// @DIR@/running exists.
const fakeServiceScript = `#!/bin/sh
cmd="$(basename "$0") $*"
echo "$cmd" >> @DIR@/calls
case "$cmd" in
"launchctl print "*) [ -f @DIR@/running ] || { echo "Could not find service" >&2; exit 113; } ;;
"launchctl bootstrap "*|"launchctl kickstart "*) touch @DIR@/running ;;
"launchctl bootout "*) rm -f @DIR@/running ;;
"launchctl enable "*|"launchctl disable "*) : ;;
"systemctl --user is-active "*) if [ -f @DIR@/running ]; then echo active; else echo inactive; exit 3; fi ;;
"systemctl --user enable --now "*|"systemctl --user restart "*) touch @DIR@/running ;;
"systemctl --user disable --now "*) rm -f @DIR@/running ;;
"systemctl --user daemon-reload"|"systemctl --user enable "*) : ;;
"loginctl show-user "*) echo yes ;;
*) echo "fake: unexpected $cmd" >&2; exit 1 ;;
esac
`
