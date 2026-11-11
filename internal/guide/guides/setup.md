# Set up this machine with agentctl

You're walking the user through agentctl's whole setup, so they don't have to click through every menu: proxies and
routing, the recommended settings, web search and MCP, then servers and the machine. Go in this order, one step at a
time. At each step say in a sentence what it does and what it costs, show the change (`--dry-run` where a command
has it, otherwise describe what it will do), and ask before applying. Skip what the user doesn't want, and anything
already done.

agentctl's commands are the way to do each step; run them with `--yes` once the user agreed, since you aren't at a
terminal prompt. `agentctl status` and `agentctl <command> --help` tell you where things stand. Steps that are a guide
of their own (`agentctl guide <topic>`): read that guide and follow it in full.

## 1. Proxies and routing

- `agentctl proxy ls`. To add one, the user needs its URL and a client key; have them run
  `agentctl proxy add <name> --url <url>` themselves so the key goes into the prompt, not this chat. `--key-cmd` reads
  it from a command instead, e.g. over ssh from the machine that runs the proxy.
- `agentctl proxy test` must pass. `agentctl proxy models <name>` lists what each agent may use; models from vendors
  agentctl can't prove aren't Anthropic's need `agentctl proxy allow <name> <model>`, only for models the user knows
  aren't Claude.
- Route each agent the user wants on the proxy: `agentctl use <claude|codex|opencode|pi> <proxy> --yes` (Codex,
  OpenCode and Pi take `--model`). Claude Code and OpenCode through a proxy come with Anthropic's terms warning:
  `agentctl use claude <proxy> --dry-run` prints it. Let the user read it and decide; `--yes` accepts it for good.

## 2. The recommended settings

Everything agentctl curates is recommended: it's how agentctl's author runs agents. `agentctl tune all --dry-run`
shows the whole diff: Claude Code and Codex (fewer unused tools, no background spend, privacy without losing
features), the shared instructions every agent reads (`~/.agents/AGENTS.md`), and git settings for agents.

Two items are risky and need the user's own yes, asked on their own, not folded into "apply everything": Claude Code's
"Never ask for approval" and Codex's "Never ask, full disk access". With them, agents run every command, deletes
included, with no prompt and no sandbox. Explain that (`agentctl tune all --dry-run --risky` shows their part of the
diff); then `agentctl tune all --yes --risky` with them, or `agentctl tune all --yes` without. Individual targets:
`agentctl tune <claude|codex|instructions|git>`.

Then commit signing, if commits aren't signed yet (`git config --global commit.gpgsign`): `agentctl guide signing`.

## 3. Web search and MCP

Built-in web search doesn't work through a proxy. `agentctl guide tools` sets up a web search CLI (Tavily or
Firecrawl) with one skill and the user's key, and `mcpc` for MCP servers.

## 4. Servers and the machine

- T3 Code (a web, desktop and phone app for coding agents): `agentctl t3 status`; `agentctl t3 install --yes` runs it
  as a background service. Pair devices with `agentctl t3 pair`.
- OpenCode's web app: `agentctl opencode status`, `agentctl opencode start`, `agentctl opencode pair`.
- Reaching them from anywhere on the user's own domain: `agentctl guide tunnel`.
- Memory limits so agents can't freeze the machine (Linux; macOS caps only Docker): `agentctl guide memory`.
- Updates: `agentctl update --check all`, then `agentctl update all --yes` if the user wants them.

## Finish

Summarize what's set up and what was skipped, with the undo for each (`agentctl tune <target> --reset`,
`agentctl use <agent> local`, the `-undo` guides). Running agent sessions pick up new settings when restarted. If the
user applied the recommended setup, tell them to open `agentctl` (the dashboard) to see it all in one place.
