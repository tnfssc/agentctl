---
name: server-facts
description: How OpenCode v2's background server, pairing and sessions work (read from anomalyco/opencode at e36e6ce and verified with 2.2.6)
---

- Not a system service: `opencode service start` (and every opencode command, through `Service.ensure`) spawns a
  detached `opencode serve --service` and returns once it answers. Stopped by `service stop` or a reboot.
- Registration: `$XDG_STATE_HOME/opencode/service.json` (`~/.local/state` default, macOS too) with `url`, `version`,
  `pid` and the password, 0600. Channels other than latest/dev/beta/next use `service-<channel>.json`; agentctl reads
  only `url`/`version` from `service.json` and gets running/URL from `opencode service status` (prints URL or
  `stopped`, exit 0 either way).
- Settings: `$XDG_CONFIG_HOME/opencode/service.json` (`hostname`, `port`, `password`, `cors`, `env`, `disabled`,
  `remote`), changed only via `opencode service set/unset`; every set/unset stops the server. Default
  `127.0.0.1:49374` (0xC0DE) on the latest channel.
- Password: the server generates one on first start and saves it to settings, so paired devices survive restarts.
  Paired sessions are stateless HMAC tokens signed with a key derived from the password, 30-day cookie
  (`opencode_session_<port>`). There is no device list and no per-device revoke; `service unset password` + `service
  start` gives a new password and signs everyone out. Local TUI/desktop clients reread it from the registration.
- `opencode pair --url <base>` prints for people only (no `--json`): "Links work once and expire in 5 minutes.", one
  `<base>/auth/connect/<code>` per URL, then a QR. Its QR encodes JSON `{code, urls}` for OpenCode's app; agentctl's QR
  encodes the link so a phone camera opens the web app. `new URL("/auth/connect/..", base)` drops any path, so bases
  with a path are refused. Browser GET of the link → 302 to `/` with the cookie; a reused link → 401.
- Errors print `Error: <message>` then a Bun stack, exit 1.
- Newest version: npm dist-tag `latest` of `@opencode/cli` (v2; `opencode-ai` is v1), as T3 uses npm's dist-tags.
  Not GitHub: v1 and v2 release from the same repo and GitHub marks v1.19.5 "latest" (2026-11-10). `opencode upgrade`
  asks `https://opencode.ai/update/api/latest/cli/<npm|homebrew>` by install method, which it detects itself; a brew
  tap can trail npm briefly, so "update" may show while upgrade says it is already installed.
