# agentctl

Switch coding agents between their own login and [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) servers.

- `local` uses whatever Claude Code or Codex is already logged in with. Nothing is copied.
- A proxy uses the accounts logged in on that proxy.
- Claude models are only ever offered to Claude Code. Codex gets a model list without them.

agentctl edits the agents' own config files (`~/.claude/settings.json`, `~/.codex/config.toml`), shows a diff first,
keeps a backup of every write, and going back to `local` restores the files exactly.

## Use

```sh
agentctl proxy add spark --url https://spark.example.ts.net:8445   # prompts for the key
agentctl proxy add home --url http://127.0.0.1:8317 --key-cmd 'cat ~/cliproxyapi/.env | grep KEY | cut -d= -f2'
agentctl proxy test                  # health, key, models, and upstream logins with --management-key
agentctl proxy models spark          # which models each agent may use

agentctl use codex spark --model gpt-6.1-sol
agentctl use claude local
agentctl status
agentctl                             # interactive: pick agent, route and model
```

Models agentctl cannot prove are non-Anthropic are hidden from Codex. Clear one you trust with
`agentctl proxy allow spark <model>`; anything that looks like Claude stays blocked.

Routing Claude Code through a proxy asks for a one-time acknowledgement: Anthropic's terms forbid proxies that serve
Claude subscription logins, and accounts on them can be banned.

## Develop

Tools are pinned in `mise.toml`.

```sh
mise install
mise run check     # vet, golangci-lint, race tests, build
mise run install   # go install with version info
```

```
cmd/agentctl/          main
internal/cli/          cobra commands (fang for help and errors), interactive mode
internal/agent/        planned file changes: diff, backup, apply
internal/agent/claude  settings.json routing
internal/agent/codex   config.toml routing and the filtered model catalog
internal/models/       which models count as Anthropic
internal/proxy/        CLIProxyAPI client
internal/tomledit/     comment-preserving TOML edits
internal/config/       agentctl's own config and state
```
