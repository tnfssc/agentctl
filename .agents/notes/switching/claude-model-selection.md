---
name: claude-model-selection
description: Claude route switching must not pin a model or override Claude Code's /model selection
---

User decision: agentctl must not set `ANTHROPIC_MODEL`. The env override caused Claude Code to warn that new sessions
would ignore the model selected with `/model`.

- Claude routing skips the dashboard model picker; the CLI rejects `--model` for Claude with guidance to use `/model`.
- Route changes remove `env.ANTHROPIC_MODEL`, including values left by earlier agentctl runs, on both local and proxy
  routes. Claude Code's own top-level `model` setting and unrelated environment settings stay untouched.
- Existing installations are cleaned up on the next applied route change, not at startup. Restart Claude Code after
  applying; a running process can retain the old variable. Shell exports must be cleared separately.
- Codex model handling is unchanged.
