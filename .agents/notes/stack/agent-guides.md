---
name: agent-guides
description: One-time machine setups are guides an agent follows, each with an undo guide and a record; why memory limits and Cloudflare Tunnel work that way (2026-11-10)
---

User, 2026-11-10, after hand-tuning memory on minipc: offer that, and Cloudflare Tunnels for T3 Code and OpenCode, as
guides like `signing` ("one-time setup"), each with an uninstall guide.

- Guides: `memory`, `memory-undo`, `tunnel`, `tunnel-undo` (plus `signing`, which changes nothing outside git and
  has no undo). A guide with a `<topic>-undo` tells the agent to write what it did to
  `<config dir>/guides/<topic>.md` (`<records>` in the text, filled in by `guide.Get`); the undo guide reads it and
  deletes it. `guide.Applied` reports topics with a record: the dashboard's guide dialog then shows the undo command,
  and `agentctl uninstall` lists it.
- Guides were dry-run by two agents (read-only) on 2026-11-10; fixes from that: caps summing to 100% of RAM, Ubuntu's
  systemd-oomd killing whole cgroups under `MemoryHigh` pressure, `set-property --runtime` instead of restarting T3.
- Memory as a guide, not agentctl writing drop-ins: it keeps "agentctl never writes T3's units", and sizes, rootless
  vs rootful Docker and parent-cgroup protection differ per machine. Linux only in effect: macOS has no way to cap a
  process group (launchd limits don't enforce resident memory), so there the guide caps only Docker's VM. minipc (60 GB,
  CachyOS, rootless Docker, zram swap, no oomd) first had agents 36/44/8 GB and Docker 12/16 GB, which add up to all RAM.
- `MemoryMin` only protects up to what every ancestor cgroup protects (kernel docs, memory.min). Default user slices
  have 0, so a drop-in on `t3code.service` alone may guarantee nothing; the guide has the agent check and offer sudo
  drop-ins on the parents. Confirmed on minipc 2026-11-10 (ssh as `tnfssc`): every parent was 0; fixed with
  `MemoryMin=2G` on `app.slice` (user) and `user.slice`, `user-.slice`, `user@.service` (system), live without restarts.
  Caps then set to agents 30/35 GB and Docker 12/16 GB (Docker already peaked at 16, agents at 45).
- Tunnel with `cloudflared` alone: `tunnel login` (browser, picks one zone, writes `cert.pem`), `tunnel create`,
  `tunnel route dns`, a local `config.yml` ingress, then a hand-written macOS launch agent or Linux systemd user unit
  running `cloudflared tunnel run <UUID>`. Not `cloudflared service install` without a token: its launchd plist runs
  bare `cloudflared`, which exits with such a config and restarts every 5 s (reproduced 2026-11-10, cloudflared
  2026.10.3). The dashboard is needed only for Access applications and deleting DNS records
  (cloudflared can't). Access is offered, default off (user's call): it may block T3/OpenCode app clients.
- Memory covers every agent (user, 2026-11-10: "it is generic"). One shared slice: T3 Code's agents' slice when T3
  runs (slices can't be reparented, and T3 names its own), else `app-agents.slice`. Terminal agents and OpenCode's
  server get there through wrappers in `~/.local/share/agentctl/agents-bin/` first on PATH (shell configs and
  `environment.d`, so agentctl's own `opencode` calls go through them too): `systemd-run --user --scope --slice`, skipped
  when already in the slice or without a user bus. Non-interactive SSH on minipc has no `XDG_RUNTIME_DIR`, so the bus
  check falls back to `/run/user/<uid>/bus`; wrapper verified there with `sleep` (lands in the slice, stdin and exit
  code pass through). Applied on minipc 2026-11-11: mise's activation in config.fish runs after conf.d, so the PATH
  line must come after it too; the user's old cliproxy shims (disabled, first on PATH, exec'ing absolute paths) would
  have bypassed the wrappers and were retired with the user's OK. OpenCode's server now runs in T3's agents' slice.
- Dashboard entry points: Behavior › Memory (a machine-wide item, state from the record), T3 Code's Service menu and
  OpenCode's Server menu (Internet access), Git › Signing.
