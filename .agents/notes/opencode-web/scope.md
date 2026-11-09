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
- OpenTunnel (2026-11-10, user asked after first trying it): remote access on/off (`opencode service set remote true`
  / `unset remote`), warned first. With it on and no address set, links use `opencode pair --remote --yes`; a set
  address still wins. agentctl never reads the tunnel route (secret; `service get` hides it).
- Copying the pairing link must be obvious (user, 2026-11-10): the pairing dialog turns mouse capture off so the
  terminal can select, click or right-click the link, which is an OSC 8 hyperlink wrapped over up to 3 rows instead of
  cut. Cost: its buttons take keys (c, enter, esc), not clicks. OSC 52 (`c`) alone failed for the user.
- Extras wanted: sign out all devices, Update (`opencode upgrade` + restart), Stop.
