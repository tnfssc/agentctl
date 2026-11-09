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

`agentctl tune claude` (or the Tune button) applies curated settings: fewer unused tools, no background spend,
privacy without losing features, and an opt-in "never ask for approval". `agentctl tune instructions` (Behavior in
the dashboard) adds curated sections to your shared `~/.agents/AGENTS.md`, which Claude Code reads too; your own text
there is never touched. `agentctl tune git` (Git in the dashboard) sets git up for agents in your global git config:
merge on pull, upstream set on first push, conflicts with the common base, remembered resolutions, clearer diffs, pruned
remote branches. Unticking an item, or "Reset to default", puts the defaults back. You review a diff before anything is
written.

agentctl doesn't touch signing keys. `agentctl guide signing` prints instructions for your coding agent instead: paste
the command into it and it walks you through the key, git and GitHub.

Models agentctl cannot prove are non-Anthropic, and image or audio models, are hidden from Codex, OpenCode and Pi.
Clear one you trust with `agentctl proxy allow spark <model>`; anything that looks like Claude stays blocked. Claude
Code picks up new proxy models on its own at startup; the others need `agentctl refresh`.

Routing Claude Code or OpenCode through a proxy asks for a one-time acknowledgement: Anthropic's terms forbid proxies
that serve Claude subscription logins, and accounts on them can be banned. OpenCode's claude-agent needs turning on once
with `opencode auth login claude-agent`.

### T3 Code

agentctl also runs [T3 Code](https://github.com/pingdotgg/t3code) on the same machine, on the nightly channel, as a
background service (T3 Code under Agents in the dashboard):

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

## Develop

Tools are pinned in `mise.toml`.

```sh
mise install
mise run check     # vet, golangci-lint, race tests, build
mise run install   # go install with version info
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
internal/models/       which models count as Anthropic
internal/proxy/        CLIProxyAPI client
internal/tomledit/     comment-preserving TOML edits
internal/config/       agentctl's own config and state
docs/demo/             the demo recording: VHS tape, fake proxy, scratch setup
```
