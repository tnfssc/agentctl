// Package guide holds instructions for coding agents that agentctl prints
// on request (`agentctl guide <topic>`). They cover setups agentctl
// deliberately doesn't do itself, such as signing keys: the user pastes
// the command into their agent, and the agent walks them through it.
//
// Guides that change the machine have an undo guide (`<topic>-undo`), and
// tell the agent to write what it did to a record in agentctl's config
// directory, so the undo guide, the dashboard and `agentctl uninstall` know
// it is set up.
package guide

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tnfssc/agentctl/internal/config"
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

// Get returns a guide's text, with the records directory filled in.
func Get(topic string) (string, error) {
	b, err := files.ReadFile(path.Join("guides", topic+".md"))
	if err != nil {
		return "", fmt.Errorf("no guide %q; guides: %s", topic, strings.Join(Topics(), ", "))
	}
	dir, err := RecordsDir()
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(b), "<records>", dir), nil
}

// Undo names the guide that reverses topic, "" when it has none.
func Undo(topic string) string {
	if slices.Contains(Topics(), topic+"-undo") {
		return topic + "-undo"
	}
	return ""
}

// RecordsDir is where agents write what a guide changed.
func RecordsDir() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "guides"), nil
}

// Applied lists the topics whose setup is on record, i.e. not undone yet.
func Applied() []string {
	dir, err := RecordsDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, t := range Topics() {
		if Undo(t) == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, t+".md")); err == nil {
			out = append(out, t)
		}
	}
	return out
}

// Command is what a user pastes into their agent to hand it a guide.
func Command(topic string) string { return "agentctl guide " + topic }
