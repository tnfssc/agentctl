---
name: disable-env-costs
description: Which Claude Code disable env vars silently cost features, for curating presets
---

- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` and `DISABLE_TELEMETRY` also stop feature-flag fetching: loses Remote
  Control, `/fast`, claude.ai skill/plugin sync, advisor tool, MCP tasks, `/restart` and more. Any non-empty value,
  even `0`, counts as set.
- Cheap and safe: `DISABLE_ERROR_REPORTING`, `DISABLE_AUTOUPDATER` (manual `claude update` still works),
  `DISABLE_COST_WARNINGS`, `spinnerTipsEnabled: false`.
- Never preset: `DISABLE_PROMPT_CACHING*` (cost and latency), `DISABLE_COMPACT`.
- `includeCoAuthoredBy` and `ANTHROPIC_SMALL_FAST_MODEL` are deprecated; use `attribution.*` and
  `ANTHROPIC_DEFAULT_HAIKU_MODEL`.
