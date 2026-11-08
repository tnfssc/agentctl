---
name: codex-tools
description: Curated Codex config trims for presets, with what each costs
---

Researched 2026-11-09 against openai/codex main (≈0.167).

- gpt-5.6/6 models run code-mode-only: the model sees `exec`/`wait` and calls other tools inside; disabling tools
  shrinks the nested catalog, not the direct list. No config-level tool allowlist.
- Safe for nearly everyone: `features.image_generation = false`, `features.tool_suggest = false`,
  `[analytics] enabled = false`, `check_for_update_on_startup = false` (when a package manager updates), drop the
  redundant `features.hooks = true`.
- Minimalist/headless: `web_search = "disabled"`, `[agents] enabled = false`, `features.goals = false`,
  `features.apps/plugins = false`, `[tools.experimental_request_user_input] enabled = false`,
  `[history] persistence = "none"`.
- `approval_policy = "never"` + `sandbox_mode = "danger-full-access"` silently upgrades web_search to live: prompt
  injection can run anything. Warn; recommend workspace-write + on-request + `approvals_reviewer = "auto_review"`.
- `service_tier = "fast"` is a legacy alias (valid: default, priority, flex). `[profiles.x]` tables were removed in
  0.134; `--profile x` overlays `~/.codex/x.config.toml`.
