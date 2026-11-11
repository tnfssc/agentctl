# Web search and MCP from the shell

You're setting up the command line tools this machine's coding agents use instead of their built-in ones: a web
search CLI (Tavily's `tvly` or Firecrawl's `firecrawl`) with its skill and an API key, and `mcpc` for MCP servers with
its skill. Built-in web search and fetch are unreliable, and through a proxy they don't work at all; MCP servers
added to each agent's config fill every request with tool definitions. agentctl doesn't install these itself; you do,
with the user.

Keep it lean: don't run the vendors' `init` or `setup` commands or their install scripts. They add plugins, MCP
servers, extra skills and editor configs the user doesn't want. Install each CLI from its official package or release
and add one skill each, nothing else. Ask before installing anything.

## Look first

- `command -v tvly firecrawl mcpc uv pipx npm` and their `--version`.
- Skills already there: `ls ~/.agents/skills ~/.claude/skills`. A skill of the same name may be the user's own;
  compare before replacing it, and ask.
- `<records>/tools.md`: if it exists, this was set up before; read it and change that setup.

## Ask the user

Which web CLI: Tavily (default: search and page extraction, plain-text output for agents, search works keyless with a
rate cap) or Firecrawl (stronger at scraping, crawling and JavaScript-heavy sites), or both. And whether they want
`mcpc` (yes unless they never use MCP servers).

## Install

- Tavily: the `tavily-cli` package from PyPI, `uv tool install tavily-cli` (or `pipx install tavily-cli`). Without
  uv or pipx, offer to install uv first.
- Firecrawl: the binary from its latest GitHub release, https://github.com/firecrawl/cli/releases/latest:
  `firecrawl-<darwin|linux>-<arm64|x64>.tar.gz`. Check it against that release's `checksums.txt`, then put
  `firecrawl` in `~/.local/bin` (check it's on PATH).
- mcpc: `brew install mcpc` on macOS with Homebrew, else `npm install -g @apify/mcpc`. Under a Node version manager
  (mise, nvm) a global npm package belongs to one Node version and disappears when it changes; say so.

The Firecrawl binary updates itself in the background and keeps the old one as `firecrawl.previous`; mention it.

## One skill each

Take each skill's folder (its `SKILL.md` and any `references/` or `rules/` next to it) from the repository skills.sh
lists it under, at the default branch:

- Tavily: `skills/core/tavily-search` in https://github.com/tavily-ai/skills (skills.sh/tavily-ai/skills/tavily-search).
  Its setup reference suggests adding Tavily's MCP server to each agent; skip that, the CLI is the point.
- Firecrawl: `skills/firecrawl` in https://github.com/firecrawl/cli (skills.sh/firecrawl/cli), plus the sibling skill
  folders its files link to (e.g. `firecrawl-search`, `firecrawl-scrape`), so those links don't break; nothing more.
- mcpc: `skills/mcpc` in https://github.com/apify/mcpc, or `mcpc help --skill`, which prints a `SKILL.md` that always
  matches the installed version.

Fetch with `gh api repos/<owner>/<repo>/contents/<path>` or the raw files, not `npx skills add`, which installs into
every agent it finds. Write each into `~/.agents/skills/<name>/`, the shared folder, and link it into
`~/.claude/skills/<name>` for Claude Code unless that already points there. For each other agent installed here, check
where it reads skills and link them there too if it doesn't read the shared folder: Codex `~/.codex/skills`, Pi
`~/.pi/agent/skills`, OpenCode `~/.config/opencode/skills`. Then ask a new session of each to list its skills. Leave
every other skill alone.

## API keys

Have the user sign in in their own terminal, so the key never lands in the chat; never print or log a key. Prefer the
browser sign-in: `--api-key <key>` leaves the key in shell history.

- Tavily: `tvly login` (browser; over SSH `tvly login --no-browser` prints the link, and the callback must reach this
  machine), or a key from https://app.tavily.com with `tvly login --api-key <key>`. Without a key, `tvly search` and
  `tvly extract` still work with a rate cap.
- Firecrawl: `firecrawl login` (browser), or a key from https://www.firecrawl.dev/app/api-keys with
  `firecrawl login --api-key <key>`.
- mcpc needs none; each MCP server has its own sign-in when connecting.

## Tell the agents

Without the shared Web and MCP instructions, agents may still try their built-in tools first. Offer to turn on just
those two, leaving the user's other instructions as they are: `agentctl tune instructions --only web,mcp --yes`
(`--dry-run` first shows the diff).

## Check

- `tvly search "agentctl" --json | head` or `firecrawl search "agentctl" --limit 1` returns results.
- `mcpc --version`, and `mcpc` lists sessions without errors.
- A new agent session lists the skills (Claude Code: `/skills`).

## Record what you did

Write `<records>/tools.md` (create the directory) so it can be undone, also by `agentctl guide tools-undo`: the date,
each CLI and how it was installed (package manager or path), each skill folder and link you created and where it came
from, and which sign-ins you set up (never the keys). Then tell the user what's ready and how to undo it.
