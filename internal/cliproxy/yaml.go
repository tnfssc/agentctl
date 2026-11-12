package cliproxy

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tnfssc/agentctl/internal/config"
)

// serverConfig is CLIProxyAPI's config.yaml as a node tree, so edits keep
// its comments and the settings agentctl doesn't know. Reads accept both
// the v8 layout (server.port, access.api-keys, management.secret-key) and
// the legacy one (port, api-keys, remote-management.secret-key); v8 wins
// when both are present, as in CLIProxyAPI. Writes use the v8 spelling.
type serverConfig struct {
	path string
	doc  yaml.Node
}

func loadConfig(path string) (*serverConfig, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the config agentctl manages
	if err != nil {
		return nil, err
	}
	return parseConfig(path, b)
}

func parseConfig(path string, b []byte) (*serverConfig, error) {
	c := &serverConfig{path: path}
	if err := yaml.Unmarshal(b, &c.doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(c.doc.Content) == 0 {
		c.doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if c.root().Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: not a YAML mapping", path)
	}
	return c, nil
}

func (c *serverConfig) root() *yaml.Node { return c.doc.Content[0] }

func (c *serverConfig) bytes() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&c.doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// save writes the config owner-only: it holds client keys.
func (c *serverConfig) save() error {
	b, err := c.bytes()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	return config.WriteFileAtomic(c.path, b, 0o600)
}

// lookup follows keys through nested mappings, returning nil when any is
// missing.
func lookup(n *yaml.Node, keys ...string) *yaml.Node {
	for _, k := range keys {
		if n == nil || n.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == k {
				next = n.Content[i+1]
			}
		}
		n = next
	}
	return n
}

// ensure returns the value at keys, creating mappings on the way and the
// last value with kind when missing.
func ensure(n *yaml.Node, kind yaml.Kind, keys ...string) *yaml.Node {
	for i, k := range keys {
		want := yaml.MappingNode
		if i == len(keys)-1 {
			want = kind
		}
		next := lookup(n, k)
		if next == nil || next.Kind != want {
			if next == nil {
				next = &yaml.Node{}
				n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, next)
			}
			*next = yaml.Node{Kind: want, HeadComment: next.HeadComment, LineComment: next.LineComment}
			switch want {
			case yaml.MappingNode:
				next.Tag = "!!map"
			case yaml.SequenceNode:
				next.Tag = "!!seq"
			}
		}
		n = next
	}
	return n
}

// remove deletes key from mapping n.
func remove(n *yaml.Node, key string) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return
		}
	}
}

// pick is the v8 value, else the legacy one.
func (c *serverConfig) pick(v8, legacy []string) *yaml.Node {
	if n := lookup(c.root(), v8...); n != nil {
		return n
	}
	return lookup(c.root(), legacy...)
}

func scalar(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

func (c *serverConfig) host() string {
	return scalar(c.pick([]string{"server", "host"}, []string{"host"}))
}

func (c *serverConfig) port() int {
	if p, err := strconv.Atoi(scalar(c.pick([]string{"server", "port"}, []string{"port"}))); err == nil && p > 0 {
		return p
	}
	return DefaultPort
}

func (c *serverConfig) tls() bool {
	return scalar(c.pick([]string{"server", "tls", "enable"}, []string{"tls", "enable"})) == "true"
}

func (c *serverConfig) apiKeys() []string {
	n := c.pick([]string{"access", "api-keys"}, []string{"api-keys"})
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	var out []string
	for _, k := range n.Content {
		if k.Kind == yaml.ScalarNode && k.Value != "" {
			out = append(out, k.Value)
		}
	}
	return out
}

func (c *serverConfig) managementKey() string {
	return scalar(c.pick([]string{"management", "secret-key"}, []string{"remote-management", "secret-key"}))
}

// authDir is where CLIProxyAPI keeps logins, with ~ expanded.
func (c *serverConfig) authDir() string {
	d := scalar(c.pick([]string{"oauth", "auth-dir"}, []string{"auth-dir"}))
	if d == "" {
		d = "~/.cli-proxy-api"
	}
	if rest, ok := strings.CutPrefix(d, "~"); ok {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, rest)
	}
	return d
}

func (c *serverConfig) setScalar(value, tag string, keys ...string) {
	n := ensure(c.root(), yaml.ScalarNode, keys...)
	n.Value, n.Tag, n.Style = value, tag, 0
	if tag == "!!str" {
		n.Style = yaml.DoubleQuotedStyle
	}
}

func (c *serverConfig) setHost(host string) {
	c.setScalar(host, "!!str", "server", "host")
	remove(c.root(), "host")
}

func (c *serverConfig) setPort(port int) {
	c.setScalar(strconv.Itoa(port), "!!int", "server", "port")
	remove(c.root(), "port")
}

func (c *serverConfig) setAPIKeys(keys []string) {
	seq := ensure(c.root(), yaml.SequenceNode, "access", "api-keys")
	seq.Content = nil
	for _, k := range keys {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k, Style: yaml.DoubleQuotedStyle})
	}
	remove(c.root(), "api-keys")
}

// setManagementKey writes a plaintext key, which CLIProxyAPI hashes on its
// next start. Management stays local-only unless already opened up.
func (c *serverConfig) setManagementKey(key string) {
	c.setScalar(key, "!!str", "management", "secret-key")
	if legacy := lookup(c.root(), "remote-management"); legacy != nil {
		remove(legacy, "secret-key")
	}
}

// seed turns the release's example config into agentctl's: this machine
// only, one fresh client key, a fresh management key.
func seed(example []byte, path string, port int) (*serverConfig, string, error) {
	c, err := parseConfig(path, example)
	if err != nil {
		return nil, "", err
	}
	if lookup(c.root(), "access", "api-keys") == nil && lookup(c.root(), "api-keys") == nil {
		return nil, "", errors.New("the release's config.example.yaml has no api-keys; agentctl doesn't know this layout")
	}
	c.setHost("127.0.0.1")
	c.setPort(port)
	c.setAPIKeys([]string{newKey()})
	mgmt := newManagementKey()
	c.setManagementKey(mgmt)
	return c, mgmt, nil
}
