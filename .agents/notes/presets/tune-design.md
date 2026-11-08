---
name: tune-design
description: How agentctl tune works and the decisions behind it
---

Agreed with the user 2026-11-09.

- One curated list per agent (`internal/tune/items.go`), each item with why and cost. Recommended items start
  ticked; risky ones ("never ask for approval") are opt-in and labeled.
- Ticking applies an item; unticking resets it to the agent's default (removes the keys/deny entries it manages).
  "Reset to defaults" unticks everything. No per-item undo records: backups plus the diff cover the rest.
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
- Applying links `~/.claude/CLAUDE.md` and `$CODEX_HOME/AGENTS.md` to that file when they don't exist; an existing
  separate file is left alone with a note.
- Items chosen with the user (generic, from their Mac and minipc files): judgment, shared tools, link every PR/ticket,
  working style, project notes, terse replies. Dropped: Git/PR habits beyond linking, Playwright UI checks, anything
  Writer-, gcloud-, kubectl- or machine-specific.
