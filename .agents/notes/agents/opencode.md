---
name: opencode
description: How agentctl routes OpenCode v2 to a proxy, and what was verified against the real binary on 2026-11-09
---

User decisions (2026-11-09): OpenCode v2 only ("we don't care about opencode v1"); one route covers both the proxy's
models and claude-agent on the same proxy; the key comes from a 0600 file; default model picked like Codex.

- Config: `$OPENCODE_CONFIG_DIR` or `$XDG_CONFIG_HOME/opencode` (`~/.config/opencode` on macOS too);
  `opencode.jsonc` wins over `opencode.json`. JSONC with comments and trailing commas; edited with `internal/jsonc`.
  v2 also reads v1 keys (`provider`, `permission`) and merges them with native `providers`.
- Proxy route writes `providers.agentctl` (`package: @ai-sdk/openai-compatible`, `settings.baseURL` = proxy `/v1`,
  `settings.apiKey: "{file:<agentctl>/keys/<proxy>}"`, models keyed by id; OpenAI-owned models get
  `package: @ai-sdk/openai` = Responses API), `providers.claude-agent.settings.{baseURL,authToken}` (proxy root, same
  key file) and `model: agentctl/<id>`. Local removes our provider and only claude-agent settings whose authToken is
  one of our key files, then restores the saved model.
- No command substitution in OpenCode config: only `{env:}` and `{file:}`. `{env:}` fails under T3 Code/launchd.
- Custom providers must list every model; OpenCode rejects unlisted ids (`Model unavailable`), so leaving Claude out
  of our provider is close to a hard lock.
- claude-agent (v2.2.3+, PR anomalyco/opencode#58782) runs the real `claude` via the Claude Agent SDK with
  `settingSources: []`, so Claude Code's settings.json (our apiKeyHelper) is ignored; `settings.baseURL/authToken` map
  to `ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN`. It only activates after `opencode auth login claude-agent --method
  claude-login` (records an "external" credential; the login itself is Claude Code's). Its model list comes from a
  probe of `claude`; with an empty Claude config only the aliases (opus/sonnet/haiku/fable) appear; a cloaking proxy
  adds disguised `claude-fable-5-dd-*` entries (set disable-cloaking-model-list on the proxy).
- Verified 2026-11-09 with OpenCode 2.2.6 against minipc: GPT over Responses, Palmyra over chat completions, and
  claude-agent/haiku through Claude Code, all from agentctl-written config.
- v2 runs a background server (`opencode service`); scripts should pass `--standalone` or stop it afterwards.
