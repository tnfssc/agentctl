---
name: charm-v2-fang-cobra
description: Why the CLI uses cobra + fang + Charm v2 and Go 1.26
---

Picked on 2026-11-08 when scaffolding the demo.

- Charm v2 (`charm.land/*/v2`) over v1: v2 is current upstream, has declarative `tea.View` fields and the
  `charmed-*` skills target it. bubbletea v2.1.x requires Go 1.26, which is why `mise.toml` pins Go 1.26.
- fang wraps cobra for styled help, errors, `--version`, completions and a hidden `man` command, so we do not
  hand-roll those.
- Viper was left out: no config surface yet. Add it when the first real command needs config files or env vars.
