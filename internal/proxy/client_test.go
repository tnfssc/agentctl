package proxy

import (
	"context"
	"testing"

	"github.com/tnfssc/agentctl/internal/testutil"
)

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://h:8317":         "http://h:8317",
		"http://h:8317/":        "http://h:8317",
		"https://h:8445/v1/":    "https://h:8445",
		" https://h/base/v1 ":   "https://h/base", //nolint:gocritic // surrounding spaces are what is tested
		"http://h:8317/v1?x=1#": "http://h:8317",
	} {
		if got, err := NormalizeURL(in); err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "h:8317", "ftp://h", "http://"} {
		if _, err := NormalizeURL(bad); err == nil {
			t.Errorf("NormalizeURL(%q) should fail", bad)
		}
	}
}

func TestClient(t *testing.T) {
	srv := testutil.Proxy(t, map[string]string{"b-model": "openai", "a-model": "google"})
	c, err := New(srv.URL + "/v1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Health(ctx); err != nil {
		t.Fatal(err)
	}
	ms, err := c.Models(ctx, testutil.Key)
	if err != nil || len(ms) != 2 || ms[0].ID != "a-model" {
		t.Fatalf("Models = %v, %v", ms, err)
	}
	if _, err := c.Models(ctx, "wrong"); !IsUnauthorized(err) {
		t.Fatalf("want unauthorized, got %v", err)
	}
}
