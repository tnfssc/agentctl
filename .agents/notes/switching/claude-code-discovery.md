---
name: claude-code-discovery
description: How Claude Code learns proxy models, and the CLIProxyAPI cloaking check
---

- With `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1`, Claude Code calls `GET /v1/models?limit=1000` on the gateway at
  startup (3s timeout, redirects fail) and adds the results to `/model`. New proxy models need only a restart; Codex's
  catalog is a snapshot and needs `agentctl refresh`.
- CLIProxyAPI answers Claude clients (anthropic-version header / claude-cli UA) in Anthropic format and, unless
  `disable-cloaking-model-list` is set, renames non-Claude ids (`claude-fable-5-dd-…`). `proxy test` and the dashboard
  list models both ways and warn about names that only appear in the Claude view. Not yet confirmed against a real
  proxy; the fake in tests mimics the researched behavior.
