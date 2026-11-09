---
name: branding
description: The logo and the README demo GIF, and why the GIF is not in git
---

- Logo (user's pick, 2026-11-09): "agentctl" in figlet's Calvin S font, three rows, 24 cells wide (`skin.LogoLines`),
  everywhere: dashboard header, `agentctl --help`, README, install.sh. In the terminal it runs a gradient from the
  accent violet through blue to cyan (`Palette.Logo`); piped output drops the color.
- Demo GIF: recorded with VHS from `docs/demo/demo.tape` (`mise run demo`) against a fake CLIProxyAPI
  (`docs/demo/fakeproxy.py`) and a scratch home at `/tmp/agentctl-demo`, a short path because the recording shows it.
  VHS comes from mise; ttyd from Homebrew (mise's aqua package has no macOS build).
- The GIF is uploaded to the release (`releases/download/<tag>/demo.gif`) and linked from the README, not committed:
  media in git history stays forever. Re-record and upload to the next release when the UI changes, then bump the
  README link.
- Unexplained in VHS only (2026-11-09): at 117×35, a sidebar row whose detail ended in "…" on the same row as the
  filled "Switch route" button drew one cell short, so the bar and button shifted left. Same bytes render correctly
  in tmux, in a `printf` replay inside VHS and at other widths. The demo's Pi default is short to avoid it. xterm.js
  also powers Termius and VS Code's terminal: if it ever shows there, start from the renderer's cursor moves after a
  "…" (lipgloss truncation tail).
