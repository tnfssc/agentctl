package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// fakeReleases serves a v9.9.9 release whose binary is body, with sums as
// its SHA256SUMS (the right ones when sums is "").
func fakeReleases(t *testing.T, body, sums string) {
	t.Helper()
	asset, err := Asset()
	if err != nil {
		t.Skip(err)
	}
	if sums == "" {
		h := sha256.Sum256([]byte(body))
		sums = hex.EncodeToString(h[:]) + "  " + asset + "\nffff  other\n"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("HEAD /releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://github.com/tnfssc/agentctl/releases/tag/v9.9.9")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("GET /releases/download/v9.9.9/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(sums)) })
	mux.HandleFunc("GET /releases/download/v9.9.9/"+asset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	prev := Repo
	Repo = srv.URL
	t.Cleanup(func() { Repo = prev })
}

// fakeBinary is the "running" agentctl, reached through a symlink like a
// version manager's.
func fakeBinary(t *testing.T) (link, target string) {
	t.Helper()
	dir := t.TempDir()
	target = filepath.Join(dir, "real", "agentctl")
	link = filepath.Join(dir, "bin", "agentctl")
	for _, d := range []string{filepath.Dir(target), filepath.Dir(link)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	prev := Executable
	Executable = func() (string, error) { return link, nil }
	t.Cleanup(func() { Executable = prev })
	return link, target
}

func TestLatestAndInstall(t *testing.T) {
	fakeReleases(t, "new binary", "")
	link, target := fakeBinary(t)
	tag, err := Latest(context.Background())
	if err != nil || tag != "v9.9.9" {
		t.Fatalf("latest = %q, %v", tag, err)
	}
	got, err := Install(context.Background(), tag)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(target); got != want {
		t.Fatalf("replaced %s, want the symlink's target %s", got, target)
	}
	if b, _ := os.ReadFile(link); string(b) != "new binary" {
		t.Fatalf("binary = %q", b)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink must stay a symlink")
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".agentctl-update-*")); len(left) != 0 {
		t.Fatalf("staged files left behind: %v", left)
	}
}

func TestBadChecksumChangesNothing(t *testing.T) {
	asset, _ := Asset()
	fakeReleases(t, "tampered", "0000000000000000000000000000000000000000000000000000000000000000  "+asset+"\n")
	_, target := fakeBinary(t)
	if _, err := Install(context.Background(), "v9.9.9"); err == nil {
		t.Fatal("a checksum mismatch must fail")
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Fatalf("binary changed to %q", b)
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		current, latest string
		want            bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.1.0", "v1.0.9", false},
		{"v1.0.0-rc.1", "v1.0.0", true},
		{"dev", "v9.9.9", false},
		{"v1.0.0-3-gabc", "v1.0.0", true},
	} {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.current, c.latest, got)
		}
	}
}
