package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// State is what agentctl remembers between runs that is not user
// configuration.
type State struct {
	// ClaudeProxyAcknowledged records that the user accepted the warning
	// about routing Claude Code through a proxy.
	ClaudeProxyAcknowledged bool `json:"claude_proxy_acknowledged,omitempty"`
	// CodexLocalModel is the model Codex used before it was routed to a
	// proxy, restored on `use codex local`.
	CodexLocalModel string `json:"codex_local_model,omitempty"`
	// PiLocalProvider and PiLocalModel are Pi's default before it was
	// routed to a proxy.
	PiLocalProvider string `json:"pi_local_provider,omitempty"`
	PiLocalModel    string `json:"pi_local_model,omitempty"`
	// OpenCodeLocalModel is OpenCode's "provider/model" before it was
	// routed to a proxy.
	OpenCodeLocalModel string `json:"opencode_local_model,omitempty"`
}

func statePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// LoadState reads the state; a missing file is the zero state.
func LoadState() (*State, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}
	st := &State{}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return st, json.Unmarshal(b, st)
}

// Save writes the state.
func (s *State) Save() error {
	path, err := statePath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(b, '\n'), 0o600)
}
