# agentctl

```text
┌─┐┌─┐┌─┐┌┐┌┌┬┐┌─┐┌┬┐┬
├─┤│ ┬├┤ │││ │ │   │ │
┴ ┴└─┘└─┘┘└┘ ┴ └─┘ ┴ ┴─┘
```

Switch coding agents between their own login and [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) servers.

![agentctl: testing a proxy, routing Codex and OpenCode to it from the dashboard, then checking the status](https://github.com/tnfssc/agentctl/releases/download/v1.0.0/demo.gif)

Works with Claude Code, Codex, [OpenCode](https://opencode.ai) (v2) and [Pi](https://pi.dev).

- `local` uses whatever the agent is already logged in with. Nothing is copied.
- A proxy uses the accounts logged in on that proxy.
- Claude models only ever run through Claude Code. Codex, OpenCode and Pi get model lists without them; OpenCode
  reaches the proxy's Claude models through its claude-agent provider, which runs your Claude Code.

agentctl edits the agents' own config files (`~/.claude/settings.json`, `~/.codex/config.toml`,
`~/.config/opencode/opencode.json`, `~/.pi/agent/models.json`), shows a diff first, keeps a backup of every write, and
going back to `local` restores the files exactly. Keys stay out of them: agents ask `agentctl token` for one, except
OpenCode, which can't run a command and reads a private copy in `~/.config/agentctl/keys/`.

## Install

macOS or Linux, Intel or ARM64:

```sh
curl -fsSL https://github.com/tnfssc/agentctl/releases/latest/download/install.sh | sh
```

Downloads the matching binary, verifies its SHA-256 checksum and installs to `~/.local/bin` without sudo.
Add that directory to your PATH if needed, then run `agentctl`. Update with `agentctl update` (from v1.1.0; before
that, run the install command again). `agentctl update all` also updates the agents' CLIs and the servers, each
through its own updater (`claude update`, `codex update`, `opencode upgrade`, `pi update --all` with Pi's extensions,
`t3 update`); `--check` only lists what's out of date. In the dashboard each agent's page has an Update button, and
Settings › Updates has Update all.

For a specific version or install directory:

```sh
curl -fsSL https://github.com/tnfssc/agentctl/releases/latest/download/install.sh \
  | AGENTCTL_VERSION=v0.1.0 AGENTCTL_INSTALL_DIR="$HOME/.local/bin" sh
```

Or [download a release](https://github.com/tnfssc/agentctl/releases) manually. With Go installed:

```sh
go install github.com/tnfssc/agentctl/cmd/agentctl@latest
```

To remove it, run `agentctl uninstall`. It first lists what agentctl set up that stays behind (agents routed through a
proxy, which stop working without it, curated settings, T3 Code's service, setups your agent did from a guide) with the
command that undoes each, and offers to open the dashboard to undo them. Then it deletes the binary, and with
`--purge` (or if you say yes) `~/.config/agentctl` with your proxies' keys.

## Use

Run `agentctl` in a terminal for the dashboard (it also works over SSH from a phone, down to 40×16) showing where each agent is routed and which proxies work.
Click (or use the keyboard) to switch routes, add, test or remove proxies, and refresh model lists.

The same things from the command line:

```sh
agentctl proxy add spark --url https://spark.your-tailnet.ts.net:8445   # prompts for the key
agentctl proxy test                  # health, key, models, and what Claude Code's /model will show
agentctl proxy models spark          # which models each agent may use

agentctl use codex spark --model gpt-6.1-sol
agentctl use opencode spark          # also points OpenCode's claude-agent at spark
agentctl use pi spark
agentctl use claude local
agentctl refresh                     # rebuild model lists after the proxy's models change
agentctl status
```

A proxy is just a URL and a client API key; reach it however you like (Tailscale serve, a LAN address). For a key
that should stay on the proxy's machine, `--key-cmd` runs a command to print it instead of storing it.

Not sure where to start? Paste `agentctl guide setup` into your coding agent: it walks you through proxies and
routing, the recommended settings, web search and MCP, and the servers, asking before each step.

`agentctl tune claude` (Settings › Claude Code in the dashboard) applies curated settings: fewer unused tools, no
background spend, privacy without losing features, and "never ask for approval". `agentctl tune instructions`
(Settings › Instructions) adds curated sections to your shared `~/.agents/AGENTS.md`, which Claude Code reads too, among
them: link every PR and ticket, search the web with a CLI, reach MCP servers through `mcpc`; your own text there is
never touched. `agentctl tune git` (Git in the dashboard) sets git up for agents in your global git config:
merge on pull, upstream set on first push, conflicts with the common base, remembered resolutions, clearer diffs, pruned
remote branches. Unticking an item, or "Reset to default", puts the defaults back. You review a diff before anything is
written. Everything is recommended: `agentctl tune all` (Settings › Recommended) applies the whole set at once. The two
risky items, no approval prompts for Claude Code and no sandbox for Codex, ask separately, or take `--risky`.

agentctl doesn't touch signing keys. `agentctl guide signing` prints instructions for your coding agent instead: paste
the command into it and it walks you through the key, git and GitHub. Other one-time setups work the same way, each
with an undo guide, and the dashboard shows the command to paste:

- `agentctl guide memory`: one shared memory cap for every agent however it starts (T3 Code's sessions, OpenCode's
  server, Claude Code, Codex and Pi from a terminal) and one for Docker, below the machine's RAM, plus a guarantee for
  T3 Code's server, so a full machine slows tests down instead of freezing. Linux (systemd drop-ins and small
  wrappers that start terminal agents in the capped slice); on macOS only Docker's VM can be capped. Undo:
  `agentctl guide memory-undo`.
- `agentctl guide tunnel`: reaches T3 Code and OpenCode from anywhere through a Cloudflare Tunnel on your own domain,
  with `cloudflared`, optionally behind Cloudflare Access. Undo: `agentctl guide tunnel-undo`.
- `agentctl guide tools`: Firecrawl instead of or next to Tavily, and API keys. `agentctl tools` (and the recommended
  setup) already installs Tavily's CLI and `mcpc`, each from its official package with one skill; built-in web search
  doesn't work through a proxy. Undo: `agentctl guide tools-undo`.

Models agentctl cannot prove are non-Anthropic, and image or audio models, are hidden from Codex, OpenCode and Pi.
Clear one you trust with `agentctl proxy allow spark <model>`; anything that looks like Claude stays blocked. Claude
Code picks up new proxy models on its own at startup; the others need `agentctl refresh`.

Routing Claude Code or OpenCode through a proxy asks for a one-time acknowledgement: Anthropic's terms forbid proxies
that serve Claude subscription logins, and accounts on them can be banned. Routing OpenCode also turns on its
claude-agent provider (`opencode auth login claude-agent --method claude-login`), without which OpenCode lists no Claude
models.

### T3 Code

agentctl also runs [T3 Code](https://github.com/pingdotgg/t3code) on the same machine, on the nightly channel, as a
background service (T3 Code under Servers in the dashboard):

```sh
agentctl t3 install                  # T3 Code's installer, then its background service
agentctl t3                          # running?, version, newer nightly
agentctl t3 update                   # newest nightly; restarts the service
agentctl t3 url https://box.your-tailnet.ts.net   # the address other devices use, once
agentctl t3 pair --label phone       # one-time link and QR code for a new device
agentctl t3 devices                  # paired devices and open links, last seen
agentctl t3 revoke --idle            # sign out paired devices last connected over 7 days ago
agentctl t3 uninstall                # remove the service; --all removes t3 too (projects stay)
```

Everything goes through T3 Code's own `t3` CLI, so updates started from a T3 Code desktop app reach the server too.

### OpenCode web

OpenCode v2 runs a background server that also serves its web app. agentctl drives it through `opencode` (OpenCode web
under Servers in the dashboard):

```sh
agentctl opencode                    # running?, address, version, newer release
agentctl opencode start              # start the server (it does not come back after a reboot on its own)
agentctl opencode url https://box.your-tailnet.ts.net   # the address other devices use, once
agentctl opencode pair               # one-time link and QR code; a device stays signed in for 30 days
agentctl opencode signout            # new server password: signs out every paired device
agentctl opencode update             # opencode upgrade, then restart the server
agentctl opencode restart | stop
```

The server listens on 127.0.0.1; put something like Tailscale Serve or a Cloudflare Tunnel (`agentctl guide tunnel`)
in front of it for other devices.

## Develop

Tools are pinned in `mise.toml`.

```sh
mise install
mise run check     # vet, golangci-lint, race tests, build
mise run install   # build into ~/.local/bin (or $AGENTCTL_INSTALL_DIR), like install.sh
mise run demo      # re-record the demo GIF (VHS; needs ttyd and ffmpeg)
```

```
cmd/agentctl/          main
internal/app/          what agentctl does, shared by the CLI and the dashboard
internal/cli/          cobra commands (fang for help and errors)
internal/tui/dash/     the dashboard (layered scene, mouse hit testing)
internal/tui/skin/     dashboard palette, logo and UI styles
internal/agent/        planned file changes: diff, backup, apply
internal/agent/claude  settings.json routing
internal/agent/codex   config.toml routing and the filtered model catalog
internal/agent/opencode opencode.json routing (OpenCode v2, claude-agent)
internal/agent/pi      models.json and settings.json routing
internal/jsonc/        comment-preserving JSONC edits
internal/t3/           T3 Code through its t3 CLI
internal/ocweb/        OpenCode's web server through its opencode CLI
internal/models/       which models count as Anthropic
internal/proxy/        CLIProxyAPI client
internal/tomledit/     comment-preserving TOML edits
internal/config/       agentctl's own config and state
docs/demo/             the demo recording: VHS tape, fake proxy, scratch setup
```
