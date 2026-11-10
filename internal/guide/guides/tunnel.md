# Reach T3 Code and OpenCode from the internet with Cloudflare Tunnel

You're helping the user reach the T3 Code and OpenCode web servers on this machine from anywhere, at hostnames on
their own domain, through a Cloudflare Tunnel. The tunnel connects out to Cloudflare, so no ports open on this
machine or its router. agentctl doesn't do this itself; you set it up with the user using `cloudflared`, and send
them to Cloudflare's dashboard only for what the CLI can't do. Ask before installing software, creating anything
in their Cloudflare account or changing DNS. Never print or paste `~/.cloudflared/cert.pem` or a tunnel's
credentials file: either one lets someone else run or change their tunnels.

## Look first

- `agentctl t3 status` and `agentctl opencode status`: which servers run here and where they listen (T3 Code
  usually on `http://127.0.0.1:3773`, OpenCode on `http://127.0.0.1:49374`) and the addresses pairing links use
  now. Only servers that run can be exposed; offer to set them up first (`agentctl` dashboard, or `agentctl t3
  install` / `agentctl opencode start`).
- `cloudflared --version`. If it's missing, install it: `brew install cloudflared` on macOS; on Linux, Cloudflare's
  package repository for the distribution (see developers.cloudflare.com, "Downloads" for cloudflared) or the
  distribution's own package.
- `ls ~/.cloudflared`, `cloudflared tunnel list` (after login) and an existing cloudflared service
  (`systemctl status cloudflared`, `systemctl --user list-units | grep -i cloudflared`,
  `launchctl list | grep -i cloudflare`). If the user already runs a tunnel, offer to add these hostnames to it
  instead of a second one, and don't overwrite its config without asking.
- `<records>/tunnel.md`: if it exists, this was set up before; read it and change that setup.

## Ask the user

- Which domain. It must be a zone on their Cloudflare account with Cloudflare's nameservers. Without one, stop:
  they can add a domain to Cloudflare first, or use something like Tailscale Serve instead.
- Which servers to expose, and at which hostnames (suggest `t3.<domain>` and `opencode.<domain>`).
- Whether to put Cloudflare Access in front (default no). Without it, anyone can reach the servers' sign-in pages;
  getting in still needs a one-time pairing link. With Access, Cloudflare asks for a login
  (e.g. an email code) first; browsers handle it, but T3 Code's and OpenCode's desktop and phone apps may not get
  past it.

## Set up

1. Log in: `cloudflared tunnel login`. It opens a browser (over SSH it prints a URL to open on any device); the user
   picks the domain, and cloudflared saves `~/.cloudflared/cert.pem`. That certificate is for that one zone. If a
   `cert.pem` exists already, login refuses; ask whether it's for the same domain and reuse it, or move it aside.
2. Create the tunnel: `cloudflared tunnel create agentctl-<machine>`. Note its UUID; its credentials are saved to
   `~/.cloudflared/<UUID>.json`.
3. Route each hostname: `cloudflared tunnel route dns <UUID> t3.<domain>`. It creates a CNAME to
   `<UUID>.cfargotunnel.com` and fails if a record exists; ask before `--overwrite-dns`, and note the old record for
   the undo. A hostname outside the zone from step 1 gets a wrong record (`t3.other.com.<domain>`); check the DNS
   page in Cloudflare's dashboard if unsure.
4. Write `~/.cloudflared/config.yml` (or add to the existing one):
   ```yaml
   tunnel: <UUID>
   credentials-file: <home>/.cloudflared/<UUID>.json
   ingress:
     - hostname: t3.<domain>
       service: http://127.0.0.1:3773
     - hostname: opencode.<domain>
       service: http://127.0.0.1:49374
     - service: http_status:404
   ```
   Use the real listen addresses from "Look first" and the absolute home path (`/Users/<user>` on macOS,
   `/home/<user>` on Linux). Check it:
   `cloudflared tunnel ingress validate` and `cloudflared tunnel ingress rule https://t3.<domain>`.
5. Keep it running, the way T3 Code's server runs (a launch agent on macOS, a systemd user service on Linux). Every
   option runs `cloudflared tunnel run <UUID>`; plain `cloudflared` without `tunnel run` exits at once with a config
   like this one.
   - macOS: write a launch agent, `~/Library/LaunchAgents/dev.agentctl.cloudflared.plist`, with `Label`
     `dev.agentctl.cloudflared`, `ProgramArguments` `<path to cloudflared>`, `--no-autoupdate`, `tunnel`, `run`,
     `<UUID>`, `RunAtLoad` and `KeepAlive` true, and `StandardOutPath`/`StandardErrorPath` in
     `~/Library/Logs/dev.agentctl.cloudflared.log`; then `launchctl bootstrap gui/$(id -u) <plist>`. Don't use
     `cloudflared service install` without a token: its launch agent runs plain `cloudflared`, which then restarts
     every few seconds. A launch agent runs while the user is logged in, like T3 Code's own on macOS.
   - Linux: prefer a user unit, which needs no sudo: `~/.config/systemd/user/cloudflared-agentctl.service` with
     `Wants=network-online.target`, `After=network-online.target`, `ExecStart=<path to cloudflared> --no-autoupdate
     tunnel run <UUID>`, `Restart=on-failure`, `[Install]` `WantedBy=default.target`; then
     `systemctl --user daemon-reload && systemctl --user enable --now cloudflared-agentctl`. It runs without anyone
     logged in only with lingering on (`loginctl show-user $USER -p Linger`; `sudo loginctl enable-linger $USER`).
     Updates then come from the package manager.
   - If the user would rather have a system service on Linux: `sudo cloudflared service install`, after copying the
     config and credentials where it reads them (`/etc/cloudflared/`), with `credentials-file` pointing there; check
     its unit runs `tunnel run`.
6. Make pairing links use the new addresses: `agentctl t3 url https://t3.<domain>` and
   `agentctl opencode url https://opencode.<domain>`. Note the previous addresses for the record first.
7. Only if the user wants Access: in Cloudflare's dashboard, Zero Trust → Access → Applications → Add an application
   → Self-hosted, one per hostname, with a policy that allows their email address. Walk them through it; cloudflared
   can't create these.

## Check

- `cloudflared tunnel info <UUID>` lists active connections.
- `curl -sI https://t3.<domain>` answers from T3 Code (not a Cloudflare 502 or 1033 page); same for OpenCode.
- Make a link with `agentctl t3 pair` or `agentctl opencode pair`: it must start with the new hostname. Have the
  user open it on a phone off this network (mobile data) and confirm they get in and that live updates work
  (both apps use WebSockets, which tunnels pass through).

## Record what you did

Write `<records>/tunnel.md` (create the directory) so it can be undone later, also by
`agentctl guide tunnel-undo`: the date, the tunnel's name and UUID, each hostname and its DNS record, every file
you created or changed with its full path, how the tunnel runs (launch agent, user unit or system service and its
name), DNS records you overwrote and what they were, Access applications created, whether you installed
cloudflared, and the pairing addresses you replaced. Then tell the user their addresses and how to undo this.

## If it fails

- 1033 or "tunnel not connected": the service isn't running or can't read its credentials; check its logs
  (`journalctl --user -u cloudflared-agentctl`, `sudo journalctl -u cloudflared`,
  `~/Library/Logs/dev.agentctl.cloudflared.log`).
- 502: the tunnel runs but the server doesn't answer at the ingress address; compare with "Look first".
- The server rejects the request because of its host name: add `originRequest:` with
  `httpHostHeader: 127.0.0.1:<port>` to that ingress rule. That doesn't change the browser's `Origin`; if WebSockets
  still fail on an origin check, look at the server's own allowed-origins setting (OpenCode has
  `opencode service set cors`).
- The hostname doesn't resolve, or resolves somewhere odd: the CNAME is missing or in the wrong zone (step 3).
