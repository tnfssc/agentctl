---
name: tune-design
description: How agentctl tune works and the decisions behind it
---

Agreed with the user 2026-11-09.

- One curated list per agent (`internal/tune/items.go`), each item with why and cost. Recommended items start
  ticked; risky ones ("never ask for approval") are opt-in and labeled.
- Ticking applies an item; unticking resets it to the agent's default (removes the keys/deny entries it manages).
  "Reset to default" (`--reset`, `x` in the dialog) unticks everything. No per-item undo records: backups plus the diff cover the rest.
- The privacy item replaces DISABLE_TELEMETRY / CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC with
  DISABLE_ERROR_REPORTING + DISABLE_AUTOUPDATER; reset does not bring the old vars back.
- CLAUDE_CODE_EVEN_MORE_DANGEROUSLY_SKIP_PERMISSIONS is ignored in settings.json env (launch env only), so agentctl
  writes it to `~/.config/agentctl/env.sh` and, when fish is configured, `~/.config/fish/conf.d/agentctl.fish`. Other
  shells must source env.sh; agentctl never edits shell rc files.
- Surfaces: `agentctl tune <agent>` (multiselect, `--yes`, `--reset`, `--dry-run`) and a Tune dialog in the
  dashboard; both review the diff before writing.

## Agent instructions (behavior)

- Separate target `instructions` (dashboard sidebar "Behavior", `agentctl tune instructions`). Every item is a marked
  section (`<!-- agentctl:<id> -->` … `<!-- /agentctl:<id> -->`) in `~/.agents/AGENTS.md`; text outside markers is the
  user's. Reset removes the section; a changed section text in a newer release counts as not applied and is replaced.
- Claude Code reads it via `~/.claude/CLAUDE.md`: a missing file becomes a link; an existing file keeps its text and
  gets a marked `@~/.agents/AGENTS.md` import (user-level imports load without an approval prompt, per the memory
  docs). Removing every section also removes the import. No Codex link (user: not needed).
- Items chosen with the user (generic, from their Mac and minipc files): judgment, shared tools, link every PR/ticket,
  working style, project notes, terse replies. Dropped: Git/PR habits beyond linking, Playwright UI checks, anything
  Writer-, gcloud-, kubectl- or machine-specific.

## Recommended is everything (user, 2026-11-11)

"The recommended way should be exactly how I use it: turn on everything." Every item is `Recommended`; the two
no-approvals items are also `Risky` and need their own yes ("Everything, ask on risky"): the CLI asks per item after
the picker (or `--risky`), `--yes` alone leaves them off, and the dashboard's Review and Settings › Recommended ask
with Turn on / Leave off / Cancel. `agentctl tune all` (`tune.PlanAll`, one merged diff for claude, codex,
instructions, git) is the recommended set at once; in the dashboard it ends with a 2 s confetti burst (user asked for
delight; local sessions only, the only animation besides spinners). Instructions gained `web` (Tavily/Firecrawl CLI
instead of built-in search, which fails behind a proxy) and `mcp` (`mcpc` instead of per-agent MCP config), and
`links` now says how to find a URL instead of printing a bare number.
`--only <ids>` (2026-11-11) turns on just those items and leaves the rest as they are, so a guide's agent can switch
on e.g. the Web and MCP instructions without a person at the picker (`agentctl tune instructions --only web,mcp
--yes`). It only turns things on; turning one off stays interactive (picker or dashboard).
Instruction text (user, 2026-11-11, "absolutely garbage"): sections must read like the rest, terse fragments, and
Links applies to replies and reports only: an agent obeying "every PR gets a link" put markdown links into PR titles
and commit subjects, which don't render. Titles, commit subjects and branch names stay plain.

