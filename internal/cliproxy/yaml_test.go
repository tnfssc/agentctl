package cliproxy

import (
	"os"
	"strings"
	"testing"
)

// The real v8 example from a release: seeding must find every setting it
// changes and keep the rest, comments included.
func TestSeedRealExample(t *testing.T) {
	example, err := os.ReadFile("testdata/config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c, mgmt, err := seed(example, "config.yaml", 9999)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.bytes()
	if err != nil {
		t.Fatal(err)
	}
	again, err := parseConfig("config.yaml", b)
	if err != nil {
		t.Fatal(err)
	}
	if again.host() != "127.0.0.1" || again.port() != 9999 || again.managementKey() != mgmt {
		t.Fatalf("host %q port %d mgmt %q", again.host(), again.port(), again.managementKey())
	}
	if keys := again.apiKeys(); len(keys) != 1 || strings.HasPrefix(keys[0], "your-api-key") {
		t.Fatalf("keys = %v", keys)
	}
	out := string(b)
	for _, want := range []string{"config-version: 8", "# Management key. If a plaintext value", "disable-control-panel: false", "allow-remote: false"} {
		if !strings.Contains(out, want) {
			t.Fatalf("seeded config lost %q", want)
		}
	}
	if strings.Count(out, "secret-key:") != strings.Count(string(example), "secret-key:") {
		t.Fatal("seeding must not add a second secret-key")
	}
}

// Configs from before v8 keep their settings at the top level; v8 wins
// when both exist, and writes move the value to v8.
func TestLegacyLayout(t *testing.T) {
	c, err := parseConfig("c.yaml", []byte("port: 8400\nhost: \"0.0.0.0\"\napi-keys:\n  - old\nremote-management:\n  secret-key: hashed\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.port() != 8400 || c.host() != "0.0.0.0" || c.apiKeys()[0] != "old" || c.managementKey() != "hashed" {
		t.Fatal("legacy values not read")
	}
	c.setAPIKeys([]string{"new"})
	c.setHost("127.0.0.1")
	c.setManagementKey("plain")
	b, _ := c.bytes()
	again, _ := parseConfig("c.yaml", b)
	if again.apiKeys()[0] != "new" || again.host() != "127.0.0.1" || again.managementKey() != "plain" || again.port() != 8400 {
		t.Fatalf("after writes:\n%s", b)
	}
	if strings.Contains(string(b), "- old") || strings.Count(string(b), "secret-key") != 1 {
		t.Fatalf("legacy spellings should be gone:\n%s", b)
	}
}
