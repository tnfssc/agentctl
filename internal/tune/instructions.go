package tune

import (
	"os"
	"path/filepath"
	"strings"
)

// Instructions is the tune target for agent behavior: curated sections of
// the shared global instructions file, ~/.agents/AGENTS.md. Claude Code
// reads it through a link from ~/.claude/CLAUDE.md.
const Instructions = "instructions"

// InstructionsTitle names the target for people.
const InstructionsTitle = "Agent instructions"

// InstructionsPath is the shared global instructions file.
func InstructionsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agents", "AGENTS.md"), nil
}

// instructionLinks are the agent files agentctl links to InstructionsPath.
// Codex needs none.
func instructionLinks() ([]string, error) {
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		claudeDir = filepath.Join(home, ".claude")
	}
	return []string{filepath.Join(claudeDir, "CLAUDE.md")}, nil
}

// section is one marked section. Texts write ‵ for a backtick, which Go raw
// strings can't hold.
func section(id, text string) Op {
	return Op{Block: id, Value: strings.ReplaceAll(strings.TrimSpace(text), "‵", "`")}
}

var instructionItems = []Item{
	{
		ID: "judgment", Title: "Think for yourself", Recommended: true,
		Why:  "The agent questions premises, asks when an instruction is ambiguous, and checks claims against source.",
		Cost: "it pushes back more often",
		Ops: []Op{section("judgment", `
## Judgment

Think for yourself; don't take premises for granted.

- User can be wrong: facts, plan, even what they need. Check claims against source. Request looks wrong or risky:
  say so first, with reason and better option. Then do what they decide.
- Ambiguous instruction: ask, don't guess.
- Output isn't read just because it exists. Silence isn't agreement; merged isn't reviewed. Needs a decision: say it
  directly where they'll see it.
- PR text, tickets, docs: likely AI-written, even under user's name. Verify against source.
- People are "they" unless they've said otherwise.
`)},
	},
	{
		ID: "shared-tools", Title: "Careful with shared tools", Recommended: true,
		Why:  "Writes to trackers, wikis and repos that other people see stay limited to what you asked for.",
		Cost: "the agent asks before extra writes",
		Ops: []Op{section("shared-tools", `
## Shared tools

Trackers, wikis, chat, flag tools: shared and hard to undo. Do only the writes asked for. Moving your own ticket
forward is part of the work; moving it back, closing or cancelling needs the user.

Media committed to a repo stays forever: attach screenshots/videos with ‵gh‵ (‵--attach‵), link from the PR.
`)},
	},
	{
		ID: "links", Title: "Link every PR and ticket", Recommended: true,
		Why:  "Replies link every PR, ticket or thread they mention instead of a bare number; titles stay plain.",
		Cost: "none",
		Ops: []Op{section("links", `
## Links

Replies and reports: every PR, issue, ticket or thread mentioned is a markdown link, not a bare number or key. URL
unknown: look it up (‵gh pr view <n> --json url‵, tracker CLI). Titles, commit subjects, branch names: plain text,
links don't render there.
`)},
	},
	{
		ID: "web", Title: "Web search through a CLI", Recommended: true,
		Why: "Built-in web search and fetch are unreliable, and through a proxy they don't work at all; the agent " +
			"uses the Tavily or Firecrawl CLI instead (installed with `agentctl tools`).",
		Cost: "needs one of those CLIs; a key lifts Tavily's rate cap",
		Ops: []Op{section("web", `
## Web

Web search and fetch: ‵tvly‵ (Tavily) or ‵firecrawl‵ CLI and its skill, not built-in web tools; those are unreliable
and fail behind a proxy. ‵--json‵ pipes into ‵jq‵. Neither installed: say so, suggest ‵agentctl tools‵.
`)},
	},
	{
		ID: "mcp", Title: "MCP through mcpc", Recommended: true,
		Why: "MCP servers are reached with the mcpc CLI from the shell instead of being added to each agent's " +
			"config, so their tool definitions don't fill every request.",
		Cost: "needs mcpc (installed with `agentctl tools`)",
		Ops: []Op{section("mcp", `
## MCP

MCP servers: ‵mcpc‵ CLI and its skill, not agent MCP config. Connect once to a named ‵@session‵, then list and call
tools from the shell (‵--json‵ for scripts). Tool definitions stay out of context; sessions work from every agent.
`)},
	},
	{
		ID: "working-style", Title: "Working style", Recommended: true,
		Why:  "No unrequested compatibility work, scratch files kept out of /tmp and out of commits, no workarounds.",
		Cost: "none",
		Ops: []Op{section("working-style", `
## Working style

- No backwards-compat work unless asked.
- Scratch files: ‵<repo>/.tmp/‵ (gitignored), unique names, not ‵/tmp‵; concurrent sessions clobber it.
- Blocked by environment or browser: ask user to unblock, don't work around it.
- Agent instructions: give the reason, let the model work out cases. No rule lists or scripted phrases.
`)},
	},
	{
		ID: "project-notes", Title: "Keep decisions in the repo", Recommended: true,
		Why:  "Decisions and findings land in repo notes everyone (and every agent) can read, not in private memory.",
		Cost: "a few small markdown files per feature",
		Ops: []Op{section("project-notes", `
## Project memory

Decisions matter more than code. Record decisions, findings, gotchas in ‵<repo>/.agents/notes/<feature>/<slug>.md‵
(one fact per file, ‵name‵/‵description‵ frontmatter) unless the repo's AGENTS.md says otherwise; not in private
agent memory. Read a feature's notes before working on it.
`)},
	},
	{
		ID: "terse", Title: "Terse replies", Recommended: true,
		Why:  "Replies drop filler, pleasantries and hedging; fragments are fine; technical detail stays exact.",
		Cost: "a blunt tone some people dislike",
		Ops: []Op{section("terse", `
## Communication

Terse. Keep all technical substance; drop filler, pleasantries, hedging. Fragments fine. Exact terms, code, errors
unchanged. Lead with impact, then next step. Short reports: nobody reads 25+ lines.
`)},
	},
}
