// Package tune applies agentctl's curated settings to Claude Code and Codex.
// Each item is a small set of settings with a reason and a cost; ticking it
// sets them, unticking it resets them to the agent's own defaults.
package tune

import (
	"github.com/tnfssc/agentctl/internal/agent/claude"
	"github.com/tnfssc/agentctl/internal/agent/codex"
)

// Op is one setting an item manages. Exactly one of Value, Drop, Entry or
// Env is used.
type Op struct {
	Path  string // dotted settings path
	Value any    // set Path to this
	Drop  bool   // remove Path (reset leaves it removed)
	Entry string // add this to the array at Path
	Env   string // export Env=1 in the shell that launches the agent
}

// Item is one curated change.
type Item struct {
	ID    string
	Title string
	Why   string // what it buys you
	Cost  string // what you give up
	// Recommended items start ticked; the rest are opt-in.
	Recommended bool
	Ops         []Op
}

func set(path string, v any) Op { return Op{Path: path, Value: v} }

func deny(tools ...string) []Op {
	ops := make([]Op, len(tools))
	for i, t := range tools {
		ops[i] = Op{Path: "permissions.deny", Entry: t}
	}
	return ops
}

var claudeItems = []Item{
	{
		ID: "remote-tools", Title: "Remove publishing and remote tools", Recommended: true,
		Why:  "Artifact, PushNotification, RemoteTrigger and DesignSync leave Claude's context and can't send work off the machine.",
		Cost: "no claude.ai artifacts, push notifications or cloud triggers",
		Ops:  deny("Artifact", "PushNotification", "RemoteTrigger", "DesignSync"),
	},
	{
		ID: "notebook", Title: "Remove the notebook editor", Recommended: true,
		Why:  "NotebookEdit is only useful for Jupyter notebooks.",
		Cost: "Claude can't edit .ipynb cells",
		Ops:  deny("NotebookEdit"),
	},
	{
		ID: "plan-tool", Title: "Stop Claude entering plan mode by itself", Recommended: true,
		Why:  "You decide when to plan.",
		Cost: "none; Shift+Tab and /plan still work",
		Ops:  deny("EnterPlanMode"),
	},
	{
		ID: "workflows", Title: "Turn off workflows", Recommended: true,
		Why:  "Workflows fan out many subagents and burn usage, sometimes from a keyword in your prompt.",
		Cost: "no multi-agent workflows",
		Ops:  []Op{set("enableWorkflows", false), set("workflowKeywordTriggerEnabled", false)},
	},
	{
		ID: "quiet", Title: "Quiet background extras", Recommended: true,
		Why:  "No tips, no suggested next prompts, no idle recaps, no feedback drafts; the last three spend usage in the background.",
		Cost: "no prompt suggestions or away summaries",
		Ops: []Op{
			set("spinnerTipsEnabled", false), set("promptSuggestionEnabled", false),
			set("awaySummaryEnabled", false), set("feedbackDrafts", "off"),
		},
	},
	{
		ID: "privacy", Title: "Private without losing features", Recommended: true,
		Why: "Stops error reports and background updates. Replaces DISABLE_TELEMETRY and " +
			"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC, which also turn off feature flags and with them " +
			"Remote Control, /fast and plugin sync.",
		Cost: "update with `claude update`",
		Ops: []Op{
			{Path: "env.DISABLE_TELEMETRY", Drop: true},
			{Path: "env.CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Drop: true},
			set("env.DISABLE_ERROR_REPORTING", "1"), set("env.DISABLE_AUTOUPDATER", "1"),
		},
	},
	{
		ID: "no-approvals", Title: "Never ask for approval (risky)",
		Why: "Claude runs every tool, including deletes of important paths, without asking. " +
			"Only for machines and sandboxes you are happy to lose.",
		Cost: "no safety net against mistakes or prompt injection",
		Ops: []Op{
			set("permissions.defaultMode", "bypassPermissions"),
			set("skipDangerousModePermissionPrompt", true),
			{Env: "CLAUDE_CODE_EVEN_MORE_DANGEROUSLY_SKIP_PERMISSIONS"},
		},
	},
}

var codexItems = []Item{
	{
		ID: "image-gen", Title: "Turn off image generation", Recommended: true,
		Why:  "One less tool in every request.",
		Cost: "Codex can't create images",
		Ops:  []Op{set("features.image_generation", false)},
	},
	{
		ID: "plugin-suggest", Title: "Turn off plugin suggestions", Recommended: true,
		Why:  "Codex stops offering to install plugins mid-task.",
		Cost: "none",
		Ops:  []Op{set("features.tool_suggest", false)},
	},
	{
		ID: "analytics", Title: "Turn off analytics", Recommended: true,
		Why:  "Usage analytics stay on your machine.",
		Cost: "none",
		Ops:  []Op{set("analytics.enabled", false)},
	},
	{
		ID: "no-approvals", Title: "Never ask, full disk access (risky)",
		Why: "Codex runs commands without a sandbox or approvals, and web search goes live. " +
			"Only for machines and sandboxes you are happy to lose.",
		Cost: "no safety net against mistakes or prompt injection",
		Ops:  []Op{set("approval_policy", "never"), set("sandbox_mode", "danger-full-access")},
	},
}

// Items returns the curated items for an agent.
func Items(agentName string) []Item {
	switch agentName {
	case claude.Name:
		return claudeItems
	case codex.Name:
		return codexItems
	}
	return nil
}
