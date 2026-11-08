---
name: anthropic-subscription-policy
description: Anthropic terms on subscription OAuth through proxies, as of 2026-11
---

- code.claude.com/docs/en/legal-and-compliance: OAuth is for "ordinary use of Claude Code and other native Anthropic
  applications"; developers may not "route requests through Free, Pro, Max, or Ultra plan credentials" or "collect,
  store, or intermediate Claude.ai credentials or session tokens".
- Consumer Terms (eff. 2025-10-08) §2 forbid sharing account credentials; §3(7) forbid automated access except via API
  key or express permission.
- Server-side blocks on third-party OAuth use since 2026-01-09.
- Implication: a CLIProxyAPI holding a Pro/Max login is the prohibited pattern even when only Claude Code calls it.
  The Claude-only-in-Claude-Code lock reduces risk, it does not make it compliant. Claude Code on its own login, or a
  proxy whose Claude upstream is an API key, is fine.
