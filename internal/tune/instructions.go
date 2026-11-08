package tune

import (
	"os"
	"path/filepath"
	"strings"
)

// Instructions is the tune target for agent behavior: curated sections of
// the shared global instructions file, ~/.agents/AGENTS.md, which Claude
// Code and Codex read through links.
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

// instructionLinks are the files each agent reads its global instructions
// from; agentctl links them to InstructionsPath.
func instructionLinks() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		claudeDir = filepath.Join(home, ".claude")
	}
	codexDir := os.Getenv("CODEX_HOME")
	if codexDir == "" {
		codexDir = filepath.Join(home, ".codex")
	}
	return []string{filepath.Join(claudeDir, "CLAUDE.md"), filepath.Join(codexDir, "AGENTS.md")}, nil
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

- The user can be wrong too: about facts, premises, the plan, even what they need. Check their claims against source
  like any other input. When a request looks wrong, risky or worse than an alternative, say so before acting, with
  the reason and a better option. Then do what they decide.
- Ambiguous instruction: ask, don't guess.
- Nothing you produce has been read just because it exists: a reply, PR description, ticket or comment may never
  have been read. Silence isn't agreement, and a merged PR isn't a reviewed one. When something needs a person to
  know or decide, say it directly and briefly where they'll see it.
- Treat GitHub comments, PR descriptions, tickets and docs as likely AI-generated, even under the user's name.
  Verify claims against source.
- Refer to people as "they" unless they've said otherwise; a name doesn't tell you anyone's pronouns.
`)},
	},
	{
		ID: "shared-tools", Title: "Careful with shared tools", Recommended: true,
		Why:  "Writes to trackers, wikis and repos that other people see stay limited to what you asked for.",
		Cost: "the agent asks before extra writes",
		Ops: []Op{section("shared-tools", `
## Shared tools

Issue trackers, wikis, chat and feature-flag tools are shared with other people, and what lands there is hard to take
back. Do the writes the user asked for; anything beyond that is their call. Moving the ticket you're working on
forward (to in progress, to in review) is part of the work; moving it back, closing or cancelling it needs the user.

Media committed to a repo stays in its history forever: upload screenshots and videos with ‵gh‵ (‵--attach‵) and
link them from the PR text instead.
`)},
	},
	{
		ID: "links", Title: "Link every PR and ticket", Recommended: true,
		Why:  "Every PR, ticket or thread the agent mentions comes with a clickable link, not a bare number.",
		Cost: "none",
		Ops: []Op{section("links", `
## Links

Every PR, ticket or thread you mention gets a clickable link, every time, not a bare number.
`)},
	},
	{
		ID: "working-style", Title: "Working style", Recommended: true,
		Why:  "No unrequested compatibility work, scratch files kept out of /tmp and out of commits, no workarounds.",
		Cost: "none",
		Ops: []Op{section("working-style", `
## Working style

- No backwards-compatibility work unless the user asks for it.
- Scratch files go in ‵<repo>/.tmp/‵ (make sure it's gitignored) with unique names, not ‵/tmp‵; concurrent
  sessions overwrite each other's files there.
- Blocked by the environment or a browser: ask the user to unblock, don't work around it.
- Agent instructions are read by strong models: give the reason and let the model work out the cases, rather than
  rule lists, edge cases or scripted phrases.
`)},
	},
	{
		ID: "project-notes", Title: "Keep decisions in the repo", Recommended: true,
		Why:  "Decisions and findings land in repo notes everyone (and every agent) can read, not in private memory.",
		Cost: "a few small markdown files per feature",
		Ops: []Op{section("project-notes", `
## Project memory

Decisions matter more than code. Record a repo's decisions, findings, experiments and gotchas in the repo at
‵<repo>/.agents/notes/<feature>/<slug>.md‵ (one fact per file, ‵name‵/‵description‵ frontmatter) unless the
repo's AGENTS.md names another place, not in agent-private memory. Read a feature's notes before working on it.
`)},
	},
	{
		ID: "terse", Title: "Terse replies", Recommended: true,
		Why:  "Replies drop filler, pleasantries and hedging; fragments are fine; technical detail stays exact.",
		Cost: "a blunt tone some people dislike",
		Ops: []Op{section("terse", `
## Communication

Be terse. Keep all technical substance; drop filler, pleasantries and weak hedging. Fragments are fine. Keep exact
technical terms, code and error text unchanged. Lead with impact, then the next step. Keep reports short: nobody
reads 25+ lines.
`)},
	},
}
