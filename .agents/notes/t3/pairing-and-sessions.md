---
name: pairing-and-sessions
description: Facts about T3 Code pairing links and sessions that shape agentctl's devices UI
---

- Pairing link: `<base>/pair#token=<12-char credential>`, from `t3 auth pairing create --json --base-url <base>`
  (`pairUrl`). Default TTL 5m, one device per link. `t3 pair` has no `--json` and needs a running server, so agentctl
  uses `auth pairing create`.
- `<base>` must be an origin: T3 builds the link with `new URL("/pair", base)`, dropping any path, so agentctl
  refuses addresses with one.
- `<base>` defaults to the server's own origin (often `http://127.0.0.1:3773`, this machine only). Users set the
  address devices use once (`agentctl t3 url`, `[t3] url` in config); the dashboard pair form saves it too.
- Sessions last 30 days and are never pruned; minipc had 20, mostly stale. `connected` is always false from the CLI
  (it's in-memory in the server), and live state is only on the server's WebSocket RPC (`subscribeAuthAccess`), too
  much to reimplement.
- `lastConnectedAt` is set only when a session goes from disconnected to connected (SessionStore `markConnected`), so
  it is "last connected", not "last seen": a device connected without a break looks stale. Server restarts (every
  nightly update) reconnect everyone, which keeps this rare.
- Idle (user's choice: 7 days) = a paired device (`subject: one-time-token`) last connected, or if never connected
  issued, over 7 days ago. Other sessions (`desktop-bootstrap`, CLI-issued, MCP) are never idle: revoking the desktop
  app's own session would cut it off. Pending links are never idle.
- `revoke` exits 0 even for unknown ids ("No active session found ..."). A revoked session needs a new link; an open
  WebSocket isn't closed by a CLI revoke (the close event is in-process), so it may work until it reconnects.
- QR codes are drawn black on white with half blocks; a ~55 char tailnet URL is version 3 (17 rows), which fits in a
  dialog from a 30-row terminal. Smaller terminals get the link only, with the height needed. Terminals without color
  get no QR: unstyled, it would be inverted on dark backgrounds.
