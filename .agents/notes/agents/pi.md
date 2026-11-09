---
name: pi
description: How agentctl routes Pi to a proxy, and Pi facts that shaped it (verified 2026-11-09)
---

- Package is `@earendil-works/pi-coding-agent` (the `@mariozechner/...` name is deprecated). Config dir
  `$PI_CODING_AGENT_DIR` or `~/.pi/agent`: `models.json` (JSONC, never written by Pi), `settings.json` (Pi rewrites it
  as 2-space JSON, merging only fields it changed), `auth.json` (its own logins; agentctl never touches it).
- Proxy route adds `providers.agentctl` to models.json: `baseUrl` = proxy `/v1`, `api: openai-completions`, per-model
  `api: openai-responses` for OpenAI-owned models, `apiKey: "!'<agentctl>' token '<proxy>'"`. Pi runs `!command` keys
  in models.json through /bin/sh on every request (10 s timeout, not cached), so the key never lands in Pi's files.
  settings.json gets `defaultProvider: agentctl` and `defaultModel`; local restores the saved previous pair.
- Every model must be listed (no /v1/models discovery). `enabledModels` does not hide models (T3 Code's RPC ignores
  it), so the Claude lock is leaving Claude ids out of our list.
- Model metadata (name, contextWindow, maxTokens, reasoning, cost, thinkingLevelMap) is copied from Pi's own catalog
  cache `models-store.json` when the id matches; unknown ids get Pi's defaults (128k context, 16k output).
- Pi's Anthropic /login reuses Claude Code's OAuth client, the pattern Anthropic's terms forbid; agentctl doesn't
  offer Claude to Pi at all.
- `pi -p` reads stdin when it isn't a terminal; scripts must pass `</dev/null` or it waits forever.
- Verified with Pi 1.4.0 against minipc: GPT over Responses and Palmyra over chat completions with a command key.
