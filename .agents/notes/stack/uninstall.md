---
name: uninstall
description: What `agentctl uninstall` removes and what it leaves, and why (user decisions 2026-11-10)
---

- CLI only (`agentctl uninstall`), not in the dashboard: the user's call.
- It lists what agentctl set up that outlives the binary, each with its undo command: agents routed through a proxy
  (they call `agentctl token`, so they break), applied tune items, T3 Code's service, guide setups on record. Then:
  open the dashboard to undo them (and look again when it closes), uninstall anyway, or cancel. The user wanted both
  a way to clean up first and a way to just remove it.
- Removes the running binary (symlinks resolved, and the link it was started through). `~/.config/agentctl` (proxy
  keys, backups, guide records) goes only with `--purge` or a yes at the prompt. `--yes` skips the prompts and leaves
  everything else as it is.
