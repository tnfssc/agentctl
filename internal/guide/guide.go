// Package guide holds instructions for coding agents that agentctl prints
// on request (`agentctl guide <topic>`). They cover setups agentctl
// deliberately doesn't do itself, such as signing keys: the user pastes
// the command into their agent, and the agent walks them through it.
package guide

import (
	"embed"
	"fmt"
	"path"
	"slices"
	"strings"
)

//go:embed guides/*.md
var files embed.FS

// Topics lists the guides by name.
func Topics() []string {
	entries, _ := files.ReadDir("guides")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	slices.Sort(names)
	return names
}

// Get returns a guide's text.
func Get(topic string) (string, error) {
	b, err := files.ReadFile(path.Join("guides", topic+".md"))
	if err != nil {
		return "", fmt.Errorf("no guide %q; guides: %s", topic, strings.Join(Topics(), ", "))
	}
	return string(b), nil
}

// Command is what a user pastes into their agent to hand it a guide.
func Command(topic string) string { return "agentctl guide " + topic }
