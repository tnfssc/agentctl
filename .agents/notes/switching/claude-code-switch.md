---
name: claude-code-switch
description: How to point Claude Code at a proxy and back to its own login by editing settings, not shims
---

Researched 2026-11-08 against code.claude.com docs (Claude Code 2.1.3xx).

- `env` block of `~/.claude/settings.json` overrides shell exports and is re-applied to running sessions, but
  removing a key only takes effect at next launch.
- Proxy: `ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN` (Bearer). Auth precedence puts `ANTHROPIC_AUTH_TOKEN` above the
  `/login` subscription, so it wins silently. `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1` lists proxy models in
  `/model`. Alias targets: `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU}_MODEL` (`ANTHROPIC_SMALL_FAST_MODEL` is deprecated).
- Local: remove `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY`, `apiKeyHelper`. Base URL alone keeps
  the subscription but still routes through the proxy. A file cannot unset a shell export; set it to `""`.
- Stored login lives in macOS Keychain or `~/.claude/.credentials.json` (Linux); tied to `CLAUDE_CONFIG_DIR` if set.
- VS Code extension's pre-launch auth check reads `claudeCode.environmentVariables` in VS Code user settings, not
  settings.json. Desktop app ignores all of this.
- Do not write `~/.claude.json`; it is Claude Code's own state.
