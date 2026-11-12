---
name: minimal-scope
description: What the user cut from scope on 2026-11-09 and why the dashboard stays small
---

- No metrics in the dashboard. (Its scope was "the bare minimum"; since 2026-11-10 it matches the CLI, see
  `dashboard-parity.md`.)
- No CLIProxyAPI management API features (login health, quotas) for now. Since 2026-11-12 agentctl does keep and
  show the management key of the CLIProxyAPI it runs itself (see `cliproxy/scope.md`), but still calls no management
  endpoint.
- No ssh tunnels for proxies: a proxy is a URL plus a client key; users expose proxies with Tailscale serve or
  similar. `--key-cmd` stays as an optional CLI flag for keys that should not be copied. Remote proxies get no
  background service; the one CLIProxyAPI agentctl runs on this machine does (see `cliproxy/`), as does T3 Code
  (T3's own, see `t3/`).
