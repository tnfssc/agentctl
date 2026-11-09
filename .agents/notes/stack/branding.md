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
