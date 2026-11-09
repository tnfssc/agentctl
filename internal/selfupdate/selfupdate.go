// Package selfupdate replaces the running agentctl with the latest release,
// the way install.sh installs it: the binary for this OS and architecture,
// checked against the release's SHA256SUMS, staged beside the old one and
// renamed over it. The path stays the same, so agents that call
// `agentctl token` keep working.
package selfupdate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tnfssc/agentctl/internal/semver"
)

// Repo is where releases come from; tests point it at a fake.
var Repo = "https://github.com/tnfssc/agentctl"

// Executable locates the running binary; tests replace it.
var Executable = os.Executable

// Latest is the newest release tag, read from GitHub's releases/latest
// redirect, which needs no API token and has no API rate limit.
func Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, Repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	c := &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking for a new release: %w", err)
	}
	_ = resp.Body.Close()
	tag := path.Base(resp.Header.Get("Location"))
	if !strings.HasPrefix(tag, "v") || !strings.Contains(resp.Header.Get("Location"), "/releases/tag/") {
		return "", fmt.Errorf("checking for a new release: no release found (%s)", resp.Status)
	}
	return tag, nil
}

// Newer reports whether latest is a newer release than current. A build
// without a release version (from source) is never updated.
func Newer(current, latest string) bool {
	return strings.HasPrefix(current, "v") && semver.Compare(strings.TrimPrefix(latest, "v"), strings.TrimPrefix(current, "v")) > 0
}

// Asset is this platform's release binary.
func Asset() (string, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return "", fmt.Errorf("releases are built for macOS and Linux, not %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return "", fmt.Errorf("releases are built for amd64 and arm64, not %s", runtime.GOARCH)
	}
	return "agentctl-" + runtime.GOOS + "-" + runtime.GOARCH, nil
}

// Install downloads tag, checks it and puts it in place of the running
// binary. It returns the path it replaced.
func Install(ctx context.Context, tag string) (string, error) {
	asset, err := Asset()
	if err != nil {
		return "", err
	}
	exe, err := Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	base := Repo + "/releases/download/" + tag
	sums, err := fetch(ctx, base+"/SHA256SUMS")
	if err != nil {
		return "", err
	}
	want, err := checksum(sums, asset)
	if err != nil {
		return "", err
	}
	bin, err := fetch(ctx, base+"/"+asset)
	if err != nil {
		return "", err
	}
	if got := sha256.Sum256(bin); hex.EncodeToString(got[:]) != want {
		return "", errors.New("checksum mismatch; nothing changed")
	}
	// Stage beside the binary so the rename is atomic.
	staged, err := os.CreateTemp(filepath.Dir(exe), ".agentctl-update-*")
	if err != nil {
		return "", fmt.Errorf("can't write next to %s: %w", exe, err)
	}
	defer func() { _ = os.Remove(staged.Name()) }()
	if _, err := staged.Write(bin); err != nil {
		_ = staged.Close()
		return "", err
	}
	if err := staged.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(staged.Name(), 0o755); err != nil { //nolint:gosec // an executable
		return "", err
	}
	if err := os.Rename(staged.Name(), exe); err != nil {
		return "", err
	}
	return exe, nil
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", path.Base(url), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", path.Base(url), resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// checksum finds asset's hash in a SHA256SUMS file.
func checksum(sums []byte, asset string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset && len(f[0]) == 64 {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS has no checksum for %s", asset)
}
