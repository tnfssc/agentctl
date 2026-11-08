# agentctl

Switch coding agents between their own login and [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) servers.

- `local` uses whatever Claude Code or Codex is already logged in with. Nothing is copied.
- A proxy uses the accounts logged in on that proxy.
- Claude models are only ever offered to Claude Code. Codex gets a model list without them.

agentctl edits the agents' own config files (`~/.claude/settings.json`, `~/.codex/config.toml`), shows a diff first,
keeps a backup of every write, and going back to `local` restores the files exactly.

## Install

macOS or Linux, Intel or ARM64:

```sh
curl -fsSL https://github.com/tnfssc/agentctl/releases/latest/download/install.sh | sh
```

Downloads the matching binary, verifies its SHA-256 checksum and installs to `~/.local/bin` without sudo.
Add that directory to your PATH if needed, then run `agentctl`. Run the same command to update.

For a specific version or install directory:

```sh
curl -fsSL https://github.com/tnfssc/agentctl/releases/latest/download/install.sh \
  | AGENTCTL_VERSION=v0.1.0 AGENTCTL_INSTALL_DIR="$HOME/.local/bin" sh
```

Or [download a release](https://github.com/tnfssc/agentctl/releases) manually. With Go installed:

```sh
go install github.com/tnfssc/agentctl/cmd/agentctl@latest
```

## Use

Run `agentctl` in a terminal for the dashboard showing where each agent is routed and which proxies work.
Click (or use the keyboard) to switch routes, add, test or remove proxies, and refresh Codex's model list.

The same things from the command line:

```sh
agentctl proxy add spark --url https://spark.your-tailnet.ts.net:8445   # prompts for the key
agentctl proxy test                  # health, key, models, and what Claude Code's /model will show
agentctl proxy models spark          # which models each agent may use

agentctl use codex spark --model gpt-6.1-sol
agentctl use claude local
agentctl refresh                     # rebuild Codex's model list after the proxy's models change
agentctl status
```

A proxy is just a URL and a client API key; reach it however you like (Tailscale serve, a LAN address). For a key
that should stay on the proxy's machine, `--key-cmd` runs a command to print it instead of storing it.

Models agentctl cannot prove are non-Anthropic are hidden from Codex. Clear one you trust with
`agentctl proxy allow spark <model>`; anything that looks like Claude stays blocked. Claude Code picks up new
proxy models on its own at startup; Codex needs `agentctl refresh`.

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
internal/app/          what agentctl does, shared by the CLI and the dashboard
internal/cli/          cobra commands (fang for help and errors)
internal/tui/dash/     the dashboard (layered scene, mouse hit testing)
internal/tui/skin/     dashboard palette and UI styles
internal/agent/        planned file changes: diff, backup, apply
internal/agent/claude  settings.json routing
internal/agent/codex   config.toml routing and the filtered model catalog
internal/models/       which models count as Anthropic
internal/proxy/        CLIProxyAPI client
internal/tomledit/     comment-preserving TOML edits
internal/config/       agentctl's own config and state
```
