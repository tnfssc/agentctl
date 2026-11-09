---
name: scope
description: What the user asked for in T3 Code management (2026-11-09) and what they cut
---

- agentctl runs on the same machine as T3 Code, Claude Code and Codex; only CLIProxyAPI may live elsewhere. No
  managing T3 on other hosts over SSH.
- Plain, vanilla T3 Code first: install, run as a background service, update, restart, pair devices, manage sessions.
  Make that experience solid before touching providers or settings.
- Nightly channel always.
- No Tailscale handling for now ("don't worry about tailscale"): no `T3CODE_TAILSCALE_SERVE`, no LAN toggles; links
  use a user-set address or the listen address.
- Steer as default follow-up: already the default (`followUpBehavior` defaults to `steer` on main and 0.1.8 nightly).
  It is a client-side setting (desktop `userdata/client-settings.json`, browser localStorage), so agentctl writes
  nothing.
