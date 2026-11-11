# Remove the web search and MCP command line tools

You're helping the user undo what `agentctl guide tools` set up: the web search CLI, `mcpc`, their skills and
sign-ins, and nothing else. Show what you'll remove and ask first.

## Find what was set up

Read `<records>/tools.md`: each CLI and how it was installed, each skill folder and link, and the sign-ins. If it's
missing, look at `command -v tvly firecrawl mcpc`, `uv tool list`, `npm ls -g --depth=0`, and the `tavily-search`,
`firecrawl` and `mcpc` folders in `~/.agents/skills` and `~/.claude/skills`, and confirm each with the user: they may
have installed some of these themselves.

## Undo

1. Sign out, so no key stays on disk: `tvly logout`, `firecrawl logout`, and for mcpc `mcpc logout <server>` for each
   sign-in it holds (in the system keychain) and `mcpc clean all` for its sessions. A `TAVILY_API_KEY` or
   `FIRECRAWL_API_KEY` set in the shell outlives logout; point it out. Offer to revoke the keys in Tavily's and
   Firecrawl's dashboards too.
2. Remove the CLIs the way they were installed: `uv tool uninstall tavily-cli` (or `pipx uninstall tavily-cli`);
   `~/.local/bin/firecrawl`, `firecrawl.previous` next to it and `~/.firecrawl/`; `brew uninstall mcpc` or
   `npm uninstall -g @apify/mcpc`, then `~/.mcpc` if it's still there.
3. Delete the skill folders and the links the setup made; leave other skills alone.
4. Offer to turn off the Web and MCP shared instructions, which point agents at these tools: the user unticks them
   in their own terminal (`agentctl tune instructions`) or the dashboard (Settings › Instructions). Don't use
   `--reset`, which removes every instruction.

## Check

`command -v tvly firecrawl mcpc` finds none of the removed ones, and the skills are gone from both skill folders. Then
delete `<records>/tools.md`, or rewrite it to list what the user kept, and tell them what was removed.
