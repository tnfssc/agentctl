---
name: updates
description: How agentctl updates itself, the agents' CLIs and the servers, and where the buttons are (2026-11-11)
---

User asked for update buttons for each CLI (Pi with its extensions, OpenCode, Claude Code, Codex) and one Update all.

- Each tool updates through its own updater, which knows how it was installed: `claude update`, `codex update`,
  `pi update --all --no-approve` (pi and its extensions; never trusts project-local files from agentctl's cwd),
  `opencode upgrade` + server restart (`ocweb.Update`), `t3 update` (service restarts), agentctl's own release swap.
  Newest versions: npm dist-tags (`internal/upgrade`), as OpenCode and T3 already did; agentctl from GitHub.
- Pi's update is always "worth" running (extensions can't be checked), so it always has a button; the count of
  updates counts only real new releases.
- `app.Update`/`app.CheckUpdates` is the shared list (CLI and dashboard); it inspects only the tools asked for, so
  `agentctl update` alone doesn't hit npm.
- CLI: `agentctl update [tool…|all]`, no argument = agentctl (as before), `--check` lists. Dashboard (user's pick):
  an Update button in each agent's third slot (Tune moved to Settings › Claude Code / Codex) and Settings › Updates
  with Update all, which runs tools one after another with "Updating X (2 of 4)" in the footer.
