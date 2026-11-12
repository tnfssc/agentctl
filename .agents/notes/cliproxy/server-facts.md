---
name: server-facts
description: How CLIProxyAPI's releases, config, reloads and logins behave, as agentctl relies on them (v8.0.67)
---

- Releases: `CLIProxyAPI_<v>_<darwin|linux>_<aarch64|amd64>[_no-plugin].tar.gz` plus `checksums.txt`; the archive
  holds `cli-proxy-api` and `config.example.yaml`. Linux defaults to the plugin build (glibc); musl gets `no-plugin`.
  Newest tag from GitHub's `releases/latest` redirect, like agentctl's own self-update.
- No version flag: every run prints `CLIProxyAPI Version: ...` then starts a server, so agentctl records the version it
  installed in `<data>/agentctl/cliproxyapi/version`.
- v8 config layout: `server.host/port`, `access.api-keys`, `management.secret-key`, `oauth.auth-dir`; legacy top-level
  spellings still load, v8 wins. agentctl edits it as a yaml.v3 node tree (comments kept), seeded from the release's
  example: host 127.0.0.1, one `sk-` key, a management key. `host: ""` binds every interface.
- The server watches config.yaml and reloads client keys live, also after agentctl's write-temp-and-rename saves
  (checked twice in a row). Host and port changes need a restart; agentctl restarts.
- Logins: `cli-proxy-api -config <cfg> -<provider>-login -no-browser` prints SSH-tunnel hints and a link, waits for
  the browser callback on a fixed local port, and after ~15s also reads a pasted callback URL from stdin, which is
  what makes it work over SSH. Auth files land in `~/.cli-proxy-api` (JSON with `type`, `email`); the server picks
  them up. `-codex-device-login` needs no callback at all.
- In the dashboard a login gets the terminal (`tea.ExecProcess`) through `sh -c 'trap : INT; ...; read'`: ctrl+c
  stops the login but not the shell, so its last lines stay until enter. The dashboard's context ignores the SIGINT
  that ctrl+c also sends agentctl while a login has the terminal (`dash.HandedOver`); before that fix it killed the
  dashboard.
- systemd user services stop at logout without lingering; the status warns (`loginctl enable-linger`) instead of
  enabling it. launchd: `enable` + `bootstrap` to start, `bootout` + `disable` to stop so it stays stopped at the next
  login, `kickstart -k` to restart.
