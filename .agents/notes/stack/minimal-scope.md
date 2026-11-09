---
name: minimal-scope
description: What the user cut from scope on 2026-11-09 and why the dashboard stays small
---

- Dashboard is the bare minimum to configure and use agentctl: agent routes, proxy health, switch, add/test/remove
  proxies, refresh Codex. No metrics.
- No CLIProxyAPI management key support (login health, quotas) for now; removed from config and commands.
- No ssh tunnels or background services for proxies: a proxy is a URL plus a client key; users expose proxies with
  Tailscale serve or similar. `--key-cmd` stays as an optional CLI flag for keys that should not be copied. T3 Code's
  background service is separate: it is T3's own (see `t3/`).
