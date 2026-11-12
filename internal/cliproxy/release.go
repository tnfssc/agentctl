package cliproxy

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Repo is where releases come from; tests point it at a fake.
var Repo = "https://github.com/router-for-me/CLIProxyAPI"

// Latest is the newest release tag, from GitHub's releases/latest redirect,
// which needs no API token and has no API rate limit.
func Latest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, Repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking for updates: %w", err)
	}
	_ = resp.Body.Close()
	loc := resp.Header.Get("Location")
	tag := path.Base(loc)
	if !strings.HasPrefix(tag, "v") || !strings.Contains(loc, "/releases/tag/") {
		return "", fmt.Errorf("checking for updates: no %s release found (%s)", Title, resp.Status)
	}
	return tag, nil
}

// musl reports whether this Linux uses musl, which can't load the plugin
// build's glibc.
var musl = func() bool {
	m, _ := filepath.Glob("/lib/ld-musl-*")
	return len(m) > 0
}

// Asset is this platform's release archive for version (without a v).
func Asset(version string) (string, error) {
	arch := map[string]string{"amd64": "amd64", "arm64": "aarch64"}[runtime.GOARCH]
	if arch == "" {
		return "", fmt.Errorf("%s releases are built for amd64 and arm64, not %s", Title, runtime.GOARCH)
	}
	suffix := ""
	switch runtime.GOOS {
	case "darwin":
	case "linux":
		if musl() {
			suffix = "_no-plugin"
		}
	default:
		return "", fmt.Errorf("agentctl runs %s on macOS and Linux, not %s", Title, runtime.GOOS)
	}
	return fmt.Sprintf("CLIProxyAPI_%s_%s_%s%s.tar.gz", version, runtime.GOOS, arch, suffix), nil
}

// release is a downloaded, verified release: the server binary and its
// example config.
type release struct {
	version string
	bin     []byte
	example []byte
}

func download(ctx context.Context, tag string) (release, error) {
	version := strings.TrimPrefix(tag, "v")
	asset, err := Asset(version)
	if err != nil {
		return release{}, err
	}
	base := Repo + "/releases/download/" + tag
	sums, err := fetch(ctx, base+"/checksums.txt")
	if err != nil {
		return release{}, err
	}
	want, err := checksum(sums, asset)
	if err != nil {
		return release{}, err
	}
	archive, err := fetch(ctx, base+"/"+asset)
	if err != nil {
		return release{}, err
	}
	if got := sha256.Sum256(archive); hex.EncodeToString(got[:]) != want {
		return release{}, fmt.Errorf("%s: checksum mismatch; nothing changed", asset)
	}
	r := release{version: version}
	if r.bin, r.example, err = unpack(archive); err != nil {
		return release{}, fmt.Errorf("%s: %w", asset, err)
	}
	return r, nil
}

func unpack(archive []byte) (bin, example []byte, err error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		var dst *[]byte
		switch path.Base(h.Name) {
		case "cli-proxy-api":
			dst = &bin
		case "config.example.yaml":
			dst = &example
		default:
			continue
		}
		if *dst, err = io.ReadAll(io.LimitReader(tr, 512<<20)); err != nil {
			return nil, nil, err
		}
	}
	if bin == nil {
		return nil, nil, errors.New("no cli-proxy-api in the archive")
	}
	return bin, example, nil
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

// checksum finds asset's SHA-256 in a checksums.txt.
func checksum(sums []byte, asset string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("the release lists no checksum for %s", asset)
}
