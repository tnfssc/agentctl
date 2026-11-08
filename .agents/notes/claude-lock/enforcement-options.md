---
name: enforcement-options
description: What can and cannot hard-block Claude models from non-Claude-Code agents
---

Requirement: Claude models only from Claude Code. Findings 2026-11-08:

- CLIProxyAPI (v8.0.61) has no per-client-key model scoping; every key reaches every model (feature request #4949
  became a discussion). `excluded-models` settings are global and would hide Claude from Claude Code too.
- CLIProxyAPI "cloak" mode disguises non-Claude-Code clients as Claude Code by default (`disable-claude-cloak-mode`),
  which actively makes Codex-to-Claude work.
- Third-party CPA plugins (key-model-access, key-provider-access) are alpha and do not cover `/v1/models` or all routes.
- Codex: filtered `model_catalog_json` only blocks the `/model` picker; `-m claude-*` still goes through.
- `owned_by` in `/v1/models` is unreliable (Antigravity serves `claude-*` as `owned_by: antigravity`; aliases rename).
  Classify by model id pattern plus management `auth-files/models` per credential.
- Only fully enforceable without changing remote proxies: a local filtering gateway in front of the proxy for non-Claude
  agents, rejecting Claude model ids and filtering model lists.
