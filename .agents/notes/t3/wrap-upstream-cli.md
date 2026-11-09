---
name: wrap-upstream-cli
description: agentctl drives T3 Code only through the upstream t3 CLI and the files it writes; why, and which t3 binary each call uses
---

T3 Code (pingdotgg/t3code) already ships everything for running a server: `t3 service install/restart`, `t3 update`,
`t3 auth pairing|session ...`, a service launcher with versioned runtimes in `~/.t3/runtime/versions/<v>/t3`, DB
backup and rollback. agentctl never writes units, plists or `service.env` itself; it calls `t3` and reads state.
Reimplementing any of it would break desktop-triggered updates (see remote-updates) and drift from upstream.

- A machine can have several t3 CLIs on different versions: the service's runtime
  (`runtime/versions/<activeVersion>/t3`), `t3` on PATH, `~/.local/bin/t3` (installer), `~/.t3/bin/t3` (desktop app
  shim). The desktop app updates itself and its shim but not a local service, and desktop-triggered remote updates
  move only the runtime. Seen 2026-11-09 on the Mac: desktop 0.1.8, launchd service still 0.1.3 (no `t3 status`).
  minipc has no `t3` on PATH, only the runtime.
- So status, update and `service ...` use the newest CLI (an older one lacks commands and refuses `service install`
  as a downgrade), and `t3 auth ...` uses the one matching the service's version: auth opens
  `userdata/statev2.sqlite` directly and migrates it to its own version, which an older running server can't read.
  Versions come from the runtime path, else `t3 --version` (`t3 v<version>`).
- t3 before 0.1.8 has no `status`; agentctl then reads the service from `runtime/service-state.json`.
- After install/update/restart, systemd (`--no-block`) and launchd return before the server answers; the dashboard
  shows "starting" and polls for up to 20s.
- `t3 status --json` omits running state, running version, URLs and updates. Running = `userdata/server-runtime.json`
  origin answering `GET /.well-known/t3/environment` (gives `serverVersion`, `label`). Newest nightly = npm
  `https://registry.npmjs.org/-/package/t3/dist-tags` `.nightly` (what the server's own check uses).
- `t3 status --json` reports `service.installed: true` with `installedVersion: null` for a T3 home whose machine runs a
  service for another home (seen on macOS with the desktop app). agentctl treats no version as not installed.
- Install = `https://t3.codes/install.sh` with `T3CODE_CHANNEL=nightly` (installs to `~/.local/bin/t3`, does not run
  `t3 up`), then `t3 service install`. agentctl leaves access settings (`T3CODE_LAN`, Tailscale) to t3's defaults.
- Errors: user errors print `ERROR` alone on a line with the message under it (stderr); other tagged errors are logged
  to stdout as `[time] ERROR (#n): SomeError: msg` plus a stack. `errorMessage` finds either, else the last line.
- Several t3 commands exit 0 when nothing happened (`service restart` with no service, revoke of an unknown id), so
  agentctl checks state first rather than trusting the exit code.
