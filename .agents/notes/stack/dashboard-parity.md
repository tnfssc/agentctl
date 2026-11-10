---
name: dashboard-parity
description: The dashboard must do everything the CLI does (user decision 2026-11-10), and what was still missing
---

User, 2026-11-10, after finding they couldn't allow Palmyra from the dashboard: "everything needs to be on the tui".
Chosen over "only what's needed to use it". Exceptions: `token` and `completion`, which agents and shells call, and
`agentctl uninstall`, which the user wanted reachable only from the CLI.

- Added then: Allow models in the proxy dialog (replacing its Remove button; `d` and the proxy page still remove),
  and an agentctl › Updates row (version, latest release, Update through the ask dialog, Check again).
- Updates checks GitHub's releases/latest redirect once per dashboard start, only for release builds; source builds
  say why they don't update.
- Still CLI-only when this was written: `proxy add --key-cmd` and `--force` (save a failing proxy, replace one),
  `t3 uninstall --all`, and `t3 url --clear` / `opencode url --clear` outside the pair forms.
