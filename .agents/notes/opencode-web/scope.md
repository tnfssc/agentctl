---
name: scope
description: What the user asked for in OpenCode web server management (2026-11-10) and what they chose against
---

Asked: support OpenCode's web service next to T3 Code (screenshot of OpenCode's own Pair dialog as reference).

- Wrap the CLI only: `opencode service start/restart/stop`, no launchd/systemd unit written by agentctl, so the server
  stays down after a reboot until something starts it. Rejected: a login unit running `opencode service start`, and a
  supervised unit running `opencode serve --service` (it would fight opencode's own replace-on-version-mismatch and
  `service restart`, which SIGTERM the server and spawn their own).
- Address like T3: the user sets the address links use (`agentctl opencode url`, `[opencode] url`), exposing the
  port is their job (Tailscale Serve etc.). No `opencode service set hostname 0.0.0.0` toggle.
- OpenTunnel: added, then dropped the same day (user, 2026-11-10) for Cloudflare Tunnel on the user's own domain,
  set up by their agent from `agentctl guide tunnel` (see `stack/agent-guides.md`). agentctl no longer reads or sets
  `remote`; a server where it was turned on keeps it until `opencode service unset remote`.
- Copying the pairing link must be obvious (user, 2026-11-10): the pairing dialog turns mouse capture off so the
  terminal can select, click or right-click the link, which is an OSC 8 hyperlink wrapped over up to 3 rows instead of
  cut. Cost: its buttons take keys (c, enter, esc), not clicks. OSC 52 (`c`) alone failed for the user.
- Extras wanted: sign out all devices, Update (`opencode upgrade` + restart), Stop.
