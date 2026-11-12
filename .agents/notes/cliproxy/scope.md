---
name: scope
description: What the user asked for in running CLIProxyAPI locally (2026-11-12), what they chose, and what was left out
---

User, 2026-11-12: "make this able to have a CLI proxy service as well and it should be able to give you the API key
and also the management key". This reversed `stack/minimal-scope.md` (no background services for proxies, no
management key); the user confirmed when asked.

Chosen (over alternatives offered):

- agentctl installs from GitHub releases and writes its own launchd agent / systemd user unit, over Homebrew
  everywhere (needs brew on Linux, bottle trails releases) or brew + upstream Linux installer (stale for v8).
- agentctl keeps the management key's plaintext in its own `keys/` (see `management-key.md`), over
  `MANAGEMENT_PASSWORD`, which upstream treats as "allow remote management".
- In the first cut too: provider logins, adding/removing API keys and rotating the management key, and a
  listen-on-every-interface toggle. Default stays 127.0.0.1; the management API stays local-only either way.

Claude logins: left out at first (`claude-lock/anthropic-subscription-policy.md`: a proxy holding a Claude
subscription is the prohibited pattern), then added the same day at the user's call ("show a warning and allow them to
sign in anyway"). Claude is last in the list, marked "against Anthropic's terms", and signing in needs an explicit yes
after the warning (`cliproxy.ClaudeRisk`) every time, in the CLI and the dashboard; the warning must fit the ask dialog
at 40×16 (tested). The status keeps warning while a `claude` auth file is there. Agents other than Claude Code still
never get Claude models.

Left out: management API calls (usage, quotas) and TLS settings.
