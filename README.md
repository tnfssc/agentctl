# agentctl

```text
┌─┐┌─┐┌─┐┌┐┌┌┬┐┌─┐┌┬┐┬
├─┤│ ┬├┤ │││ │ │   │ │
┴ ┴└─┘└─┘┘└┘ ┴ └─┘ ┴ ┴─┘
```

Switch Claude Code, Codex, [OpenCode](https://opencode.ai) (v2) and [Pi](https://pi.dev) between their own login and
[CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) servers.

![agentctl: testing a proxy, routing Codex and OpenCode to it from the dashboard, then checking the status](https://github.com/tnfssc/agentctl/releases/download/v1.6.0/demo.gif)

agentctl edits each agent's own config, shows a diff first and keeps a backup; switching back to `local` restores the
file exactly. Keys never go into agent configs. Claude models only ever run through Claude Code.

## Install

macOS or Linux:

```sh
curl -fsSL https://github.com/tnfssc/agentctl/releases/latest/download/install.sh | sh
```

Installs to `~/.local/bin`, no sudo. Or `go install github.com/tnfssc/agentctl/cmd/agentctl@latest`.
`agentctl update all` updates agentctl, the agents and the servers. `agentctl uninstall` removes it.

## Use

Run `agentctl` for the dashboard. It works over SSH, down to a phone screen. Everything it does is also a command:

```sh
agentctl proxy add spark --url https://spark.your-tailnet.ts.net:8445
agentctl proxy test
agentctl use codex spark
agentctl use claude local
agentctl status
```

New here? Paste `agentctl guide setup` into your coding agent and it walks you through the rest.
`agentctl <command> --help` has the details.

Routing Claude Code or OpenCode through a proxy asks for an acknowledgement first: Anthropic's terms forbid proxies
serving Claude subscription logins, and accounts can be banned.

<img src="https://github.com/tnfssc/agentctl/releases/download/v1.6.0/phone.gif" width="360" alt="the compact dashboard on a phone-sized terminal: the home list, a server's page">

## More

**Curated settings.** `agentctl tune all` sets Claude Code, Codex, your shared `AGENTS.md` and git up for agents.
Review a diff before anything is written.

![Settings › Claude Code: tick curated settings, review the diff, apply](https://github.com/tnfssc/agentctl/releases/download/v1.6.0/tune.gif)

**CLIProxyAPI here.** `agentctl cliproxy install` runs a CLIProxyAPI server as a background service and attaches it
as a proxy, with its keys and logins.

![agentctl cliproxy install and keys, then the dashboard: keys, logins with Claude's warning, the server menu](https://github.com/tnfssc/agentctl/releases/download/v1.6.0/cliproxy.gif)

**T3 Code.** `agentctl t3 install` runs [T3 Code](https://github.com/pingdotgg/t3code) as a background service;
`agentctl t3 pair` gives other devices a link and QR code.

![T3 Code under Servers: the paired devices, then a one-time pairing link with its QR code](https://github.com/tnfssc/agentctl/releases/download/v1.6.0/t3.gif)

**OpenCode web.** `agentctl opencode start` runs OpenCode's web server; `agentctl opencode pair` signs in a device.

![OpenCode web: status, a pairing link with its QR code, the server menu](https://github.com/tnfssc/agentctl/releases/download/v1.6.0/opencode.gif)

**Guides** for one-time setups agentctl leaves to your agent, each with an undo: `agentctl guide signing`, `memory`,
`tunnel`, `tools`.

## Develop

```sh
mise install
mise run check     # vet, lint, race tests, build
mise run install   # build into ~/.local/bin
mise run demo      # re-record the README GIFs
```
