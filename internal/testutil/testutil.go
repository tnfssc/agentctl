// Package testutil isolates tests from the real home directory and fakes a
// CLIProxyAPI server.
package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tnfssc/agentctl/internal/agent"
)

// Key is the client key the fake proxy accepts.
const Key = "sk-test"

// Home points every config location at a fresh temp dir and fakes the
// helper binary path. It returns the temp home.
func Home(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	for _, k := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"} {
		t.Setenv(k, "")
	}
	prev := agent.Executable
	agent.Executable = func() (string, error) { return "/opt/bin/agentctl", nil }
	t.Cleanup(func() { agent.Executable = prev })
	return home
}

// Proxy starts a fake CLIProxyAPI serving the given model ids and owners.
func Proxy(t *testing.T, models map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+Key {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}
		type m struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		}
		var data []m
		for id, owner := range models {
			data = append(data, m{id, owner})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// BundledCatalog is a stand-in for `codex debug models --bundled`.
func BundledCatalog() []map[string]any {
	return []map[string]any{
		{"slug": "gpt-6.1-sol", "display_name": "GPT-6.1-Sol", "visibility": "list", "priority": float64(1), "base_instructions": "x"},
		{"slug": "gpt-5.5", "display_name": "GPT-5.5", "visibility": "list", "priority": float64(9), "base_instructions": "x", "upgrade": map[string]any{"model": "gpt-6"}},
	}
}
