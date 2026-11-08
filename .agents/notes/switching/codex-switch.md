---
name: codex-switch
description: How to point Codex at a proxy and back to its ChatGPT login via config.toml
---

Researched 2026-11-08 against openai/codex source (0.167.0).

- Proxy: top-level `model_provider = "<our-id>"` plus `[model_providers.<our-id>]` with `base_url = "<proxy>/v1"`,
  `wire_api = "responses"` (`chat` is now a hard error), `env_key` or `auth` (command printing a token),
  `supports_websockets`. Built-in ids like `openai` cannot be overridden. Profiles cannot hold `model_providers`.
- Never set `requires_openai_auth = true` on the proxy provider: Codex would send the ChatGPT credential to the proxy.
- Local: remove `model_provider` (defaults to `openai`), so `~/.codex/auth.json` (ChatGPT login) is used again.
- Model picker: `model_catalog_json` (local file, read at startup) replaces the bundled catalog and disables fetching.
  `/model <name>` rejects names not in it; `-m`, `-c model=` and `model =` do not (falls back to generic metadata).
- CLIProxyAPI answers `GET /models?client_version=` in Codex catalog format including Claude, so do not point Codex's
  `model_catalog_url` at it.
