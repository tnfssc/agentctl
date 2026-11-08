package models

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		id, owner string
		allow     []string
		want      Class
	}{
		{"claude-opus-5-5", "anthropic", nil, Anthropic},
		{"claude-opus-5-5-high", "antigravity", nil, Anthropic},
		{"team/claude-haiku-5-5", "", nil, Anthropic},
		{"my-opus-alias", "codex", nil, Anthropic},
		{"fable-5-1", "", nil, Anthropic},
		{"gpt-6.1-sol", "openai", nil, Other},
		{"gemini-3-pro", "google", nil, Other},
		{"daybreak-blue", "openai", nil, Other},
		{"palmyra-x6", "WRITER", nil, Unknown},
		{"palmyra-x6", "WRITER", []string{"palmyra-x6"}, Other},
		{"team/palmyra-x6(high)", "WRITER", []string{"palmyra-x6"}, Other},
		{"gemini-3-pro", "antigravity", nil, Unknown},
		// An allow entry can never clear a Claude signal.
		{"claude-sonnet-5", "antigravity", []string{"claude-sonnet-5"}, Anthropic},
		// "opus" must be a whole word, not part of another name.
		{"magnum-opusx", "openai", nil, Other},
	}
	for _, tt := range tests {
		if got := Classify(tt.id, tt.owner, tt.allow); got != tt.want {
			t.Errorf("Classify(%q, %q, %v) = %v, want %v", tt.id, tt.owner, tt.allow, got, tt.want)
		}
	}
}
