---
name: instructions-files
description: Where each agent reads global instructions, and the user's call not to link them (2026-11-09)
---

Checked in source: Pi reads `~/.pi/agent/AGENTS.md` (or CLAUDE.md) plus ancestor dirs of the project; OpenCode v2 reads
`~/.config/opencode/AGENTS.md` plus ancestors up to home (its `~/.agents` support is skills only); OpenCode v1 falls
back to `~/.claude/CLAUDE.md`. None read `~/.agents/AGENTS.md` directly.

The user decided not to have agentctl link them ("No links"): `tune instructions` keeps writing only
`~/.agents/AGENTS.md` and linking Claude Code's CLAUDE.md. Users who want Pi or OpenCode to see it link those files
themselves.
