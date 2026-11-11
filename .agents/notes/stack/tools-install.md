---
name: tools-install
description: agentctl installs Tavily's CLI, mcpc and their skills itself (`agentctl tools`, part of the recommended setup); why, and how it stays lean (2026-11-11)
---

User, 2026-11-11: the recommended setup turned on the Web and MCP instructions while the tools they name came only from
`agentctl guide tools`, and the dashboard said "not set up" on a machine with `tvly` installed. "This makes no sense."
Chose: Apply installs the tools (over the guide owning it, or gating the instructions on detection).

- `internal/tools`: Tavily's CLI (`uv tool install tavily-cli`, else pipx), mcpc (`brew install mcpc` on macOS, else
  `npm install -g @apify/mcpc`), only what's missing; no installer means a warning, never a fallback script.
- Skills: the folder alone from the repo's default-branch tarball (codeload), into `~/.agents/skills/<name>` (Codex,
  Pi, OpenCode read it; Pi docs and the Codex binary confirm), linked into Claude Code's `skills/`. A skill already in
  the shared folder is the user's: linked, never replaced. "Missing" means absent from Claude Code's folder.
- Installs are `agent.Plan.Runs`: shown in the review as `run ...` lines, taken by `app.Apply` after the files.
  `tune.PlanAll` adds them, so `agentctl tune all` and the dashboard's Recommended › Apply both install;
  `agentctl tools` and Settings › Web & MCP › Install do it alone.
- State comes from what's installed (`tools.Detect`), not from the guide record. Each install still appends to
  `<records>/tools.md`, so `agentctl guide tools-undo` knows what to remove.
- Keys stay the user's: Tavily searches keyless, rate-capped; `tvly login` lifts it. The guide is now for Firecrawl,
  keys and other skill folders.
- Tests: `testutil.Home` makes `tools.Inspect` report everything installed, so no test can plan or run the real
  brew/npm/uv; `testutil.Tools` fakes the package managers and GitHub.
