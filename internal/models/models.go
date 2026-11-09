// Package models decides which proxy models are Anthropic's, so agentctl can
// keep them out of every agent except Claude Code.
//
// The rule fails safe: a model counts as non-Anthropic only with positive
// evidence. Names churn (codenames, aliases, resellers serving Claude under
// their own owner), and hiding a non-Claude model costs the user one allow
// entry while showing a Claude model breaks the policy.
package models

import (
	"regexp"
	"slices"
	"strings"
)

// Class is the verdict for one model.
type Class int

const (
	// Unknown models are treated as Anthropic unless the user allows them.
	Unknown Class = iota
	// Anthropic models may only be used from Claude Code.
	Anthropic
	// Other models are positively known not to be Anthropic's.
	Other
)

func (c Class) String() string {
	switch c {
	case Anthropic:
		return "anthropic"
	case Other:
		return "other"
	default:
		return "unknown"
	}
}

var anthropicName = regexp.MustCompile(`(?i)(^|[^a-z0-9])(claude|anthropic|opus|sonnet|haiku|fable|mythos)([^a-z0-9]|$)`)

// singleVendorOwners are owned_by values that only ever carry one vendor's
// models. Resellers (antigravity, openai-compatible names, vertex) are not
// here because they serve Claude under their own name.
var singleVendorOwners = []string{"openai", "google", "xai", "x-ai", "moonshot", "moonshotai", "meta", "deepseek", "mistral", "cognition", "qwen", "zhipu", "minimax"}

// Classify returns a model's class from its id and owned_by. allow lists ids
// the user cleared for non-Claude agents; it only resolves Unknown, never
// Anthropic.
func Classify(id, ownedBy string, allow []string) Class {
	base := Normalize(id)
	owner := strings.ToLower(strings.TrimSpace(ownedBy))
	if owner == "anthropic" || anthropicName.MatchString(id) {
		return Anthropic
	}
	if slices.Contains(singleVendorOwners, owner) {
		return Other
	}
	if slices.ContainsFunc(allow, func(a string) bool { return Normalize(a) == base }) {
		return Other
	}
	return Unknown
}

// Usable reports whether a non-Claude-Code agent may list the model.
func Usable(c Class) bool { return c == Other }

// Normalize strips decorations CLIProxyAPI adds to ids: a credential prefix
// ("team/gpt-6"), a parenthesized variant ("gpt-6(high)") and a context
// marker ("[1m]").
func Normalize(id string) string {
	s := strings.ToLower(strings.TrimSpace(id))
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "(["); i > 0 {
		s = s[:i]
	}
	return s
}

// nonChat matches models that can't drive a coding agent: image, audio,
// embedding and moderation models served next to the chat ones.
var nonChat = regexp.MustCompile(`(?i)(^|[^a-z0-9])(image|dall-e|tts|whisper|transcribe|embedding|embed|moderation|realtime|audio)([^a-z0-9]|$)`)

// Chat reports whether a model looks like one a coding agent can talk to.
func Chat(id string) bool { return !nonChat.MatchString(Normalize(id)) }
