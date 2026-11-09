---
name: claude-code-discovery
description: How Claude Code learns proxy models, and the CLIProxyAPI cloaking check
---

- With `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1`, Claude Code calls `GET /v1/models?limit=1000` on the gateway at
  startup (3s timeout, redirects fail) and adds the results to `/model`. New proxy models need only a restart; Codex's
  catalog is a snapshot and needs `agentctl refresh`.
- Claude Code keeps only discovered ids starting with `claude` (verified 2026-11-09, Claude Code 2.1.310, fake
  gateway listing claude-opus-5-5, gpt-6.1-sol, palmyra-x6: only the Claude id reached
  `~/.claude/cache/gateway-models.json`).
- That is why CLIProxyAPI cloaks its Anthropic-format model list by default: non-Claude ids become
  `claude-fable-5-dd-<id reversed>` and requests are mapped back (`internal/client/claude/models/models.go`). It is
  the only way GPT and other models reach Claude Code's /model picker, so leave `disable-cloaking-model-list` off.
  The user relies on it (T3 Code favorites run gpt-5.6-sol and palmyra-x6 through Claude Code).
- Side effect: anything that lists models as Claude Code (OpenCode's claude-agent) shows the disguised names too.
- `proxy test` and the dashboard report which of the two Claude Code will see; neither is a problem.
