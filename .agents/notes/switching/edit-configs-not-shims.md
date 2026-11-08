---
name: edit-configs-not-shims
description: Why agentctl switches routes by editing each agent's config instead of PATH shims, and the details chosen
---

Decided 2026-11-09 with the user.

- Routes are written into the agents' own files (`~/.claude/settings.json`, `~/.codex/config.toml`), not injected by a
  PATH shim like the earlier `cliproxy` shell tool. Reason: IDE extensions, desktop apps and services such as T3 Code
  read those files without sharing the shell's PATH. Cost: running sessions keep the old route until restarted.
- `local` means the agent's own login; nothing is copied or handed off. Proxy mode uses accounts logged in on the proxy.
- Keys never land in agent configs: Claude Code gets `apiKeyHelper = "<agentctl> token <proxy>"`, Codex gets
  `[model_providers.agentctl.auth] command/args`. The helper path is the running binary without resolving symlinks
  so version-manager links survive upgrades; `go run` binaries are refused.
- Edits are surgical and reversible: proxy→local restores the original file byte-for-byte (tested on real configs),
  every write is backed up under `~/.config/agentctl/backups/<agent>/` (last 20), symlinked configs stay symlinks.
- Codex: provider id `agentctl`, `wire_api = "responses"`, websockets left off (CPA websocket writes failed in logs).
  The previous local `model` is saved in state and restored on `use codex local`.
- Claude through a proxy is allowed after a one-time acknowledged warning (Anthropic terms; see claude-lock notes).
  The Claude lock is soft: Codex's catalog omits Claude/unverified models; typing one by hand is the user's problem.
- v1 scope: Claude Code and Codex only; it configures the machine it runs on.
