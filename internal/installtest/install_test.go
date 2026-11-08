// Package installtest tests the public installer without accessing GitHub or
// the user's install directory.
package installtest

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstaller(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("installer supports macOS and Linux")
	}
	for _, tt := range []struct {
		name, os, arch, version string
		badChecksum             bool
	}{
		{name: "mac-arm-latest", os: "Darwin", arch: "arm64"},
		{name: "linux-intel-pinned", os: "Linux", arch: "x86_64", version: "v0.1.0"},
		{name: "linux-arm", os: "Linux", arch: "aarch64"},
		{name: "bad-checksum", os: "Darwin", arch: "arm64", badChecksum: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			write := func(path, content string, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(path, []byte(content), mode); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(bin, "uname"), `#!/bin/sh
case "$1" in
  -s) echo "$TEST_OS" ;;
  -m) echo "$TEST_ARCH" ;;
esac
`, 0o700)
			write(filepath.Join(bin, "curl"), `#!/bin/sh
set -eu
url= out=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    --connect-timeout|--max-time|--retry) shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
printf '%s\n' "$url" >> "$FIXTURES/requests"
case "$url" in
  */SHA256SUMS) cp "$FIXTURES/SHA256SUMS" "$out" ;;
  */agentctl-*) cp "$FIXTURES/binary" "$out" ;;
  *) exit 1 ;;
esac
`, 0o700)
			const binary = "#!/bin/sh\necho agentctl-test\n"
			write(filepath.Join(dir, "binary"), binary, 0o600)
			sum := fmt.Sprintf("%x", sha256.Sum256([]byte(binary)))
			if tt.badChecksum {
				sum = strings.Repeat("0", 64)
			}
			osName, arch := "linux", "arm64"
			if tt.os == "Darwin" {
				osName = "darwin"
			}
			if tt.arch == "x86_64" {
				arch = "amd64"
			}
			asset := "agentctl-" + osName + "-" + arch
			write(filepath.Join(dir, "SHA256SUMS"), sum+"  "+asset+"\n", 0o600)
			installDir := filepath.Join(dir, "install with spaces")
			if err := os.Mkdir(installDir, 0o700); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(installDir, "agentctl")
			write(destination, "old binary", 0o700)
			for k, v := range map[string]string{
				"PATH":   bin + string(os.PathListSeparator) + os.Getenv("PATH"),
				"TMPDIR": dir, "FIXTURES": dir, "TEST_OS": tt.os, "TEST_ARCH": tt.arch,
				"AGENTCTL_VERSION": tt.version, "AGENTCTL_INSTALL_DIR": installDir,
			} {
				t.Setenv(k, v)
			}
			cmd := exec.CommandContext(t.Context(), "sh", "../../install.sh")
			out, err := cmd.CombinedOutput()
			got, readErr := os.ReadFile(destination)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tt.badChecksum {
				if err == nil || !strings.Contains(string(out), "checksum mismatch") || string(got) != "old binary" {
					t.Fatalf("checksum failure must leave existing binary intact: %v, %s", err, out)
				}
				return
			}
			if err != nil || string(got) != binary {
				t.Fatalf("install failed: %v, %s", err, out)
			}
			info, err := os.Stat(destination)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o755 {
				t.Fatalf("binary mode = %v", info.Mode())
			}
			requests, err := os.ReadFile(filepath.Join(dir, "requests"))
			if err != nil {
				t.Fatal(err)
			}
			wantPath := "/releases/latest/download/" + asset
			if tt.version != "" {
				wantPath = "/releases/download/" + tt.version + "/" + asset
			}
			if !strings.Contains(string(requests), wantPath) {
				t.Fatalf("wrong asset URL: %s", requests)
			}
			leftovers, err := filepath.Glob(filepath.Join(dir, "agentctl-install.*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary downloads not cleaned up: %v, %v", leftovers, err)
			}
		})
	}
}
