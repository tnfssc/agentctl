# Remove the Cloudflare Tunnel for T3 Code and OpenCode

You're helping the user take T3 Code and OpenCode off the internet again, undoing what `agentctl guide tunnel` set
up and nothing else. Show what you'll remove and ask before deleting anything, especially in their Cloudflare
account; other tunnels, DNS records and Access applications there are theirs.

## Find what was set up

Read `<records>/tunnel.md`: the tunnel's name and UUID, hostnames, files, how it runs, Access applications, and
the pairing addresses it replaced. If it's missing, look at `~/.cloudflared/config.yml`, `cloudflared tunnel list`,
and cloudflared services (`systemctl --user list-units | grep -i cloudflared`, `systemctl status cloudflared`,
`launchctl list | grep -i cloudflare`), and confirm with the user which tunnel is the one for T3 Code and OpenCode.

## Undo

1. Stop it running:
   - macOS launch agent: `launchctl bootout gui/$(id -u)/dev.agentctl.cloudflared`, then delete its plist and
     `~/Library/Logs/dev.agentctl.cloudflared.log`. If the record says `cloudflared service install` made it,
     `cloudflared service uninstall` instead.
   - Linux user unit: `systemctl --user disable --now cloudflared-agentctl`, delete
     `~/.config/systemd/user/cloudflared-agentctl.service`, `systemctl --user daemon-reload`. Leave lingering as it
     was; T3 Code's service may need it.
   - System service: `sudo cloudflared service uninstall`, and the copies in `/etc/cloudflared/` the setup made.
2. Delete the tunnel: `cloudflared tunnel cleanup <UUID>`, then `cloudflared tunnel delete <UUID>`. That also
   removes `~/.cloudflared/<UUID>.json`; delete it if it's still there.
3. Delete the DNS records: cloudflared has no command for it. In Cloudflare's dashboard, the domain → DNS →
   Records, delete each hostname's CNAME to `<UUID>.cfargotunnel.com`, and put back records the setup overwrote,
   from the record.
4. Delete the Access applications the setup created, if any: Zero Trust → Access → Applications.
5. Remove the tunnel's lines from `~/.cloudflared/config.yml`, or the file if the setup created it and nothing else
   is in it. Ask whether to delete `~/.cloudflared/cert.pem` too: it's needed for any other tunnel management
   from this machine.
6. Point pairing links back at the previous addresses from the record: `agentctl t3 url <previous>` or
   `agentctl t3 url --clear`, and the same with `agentctl opencode url`.
7. If the setup installed cloudflared and nothing else uses it, offer to uninstall it with the package manager.

## Check

`cloudflared tunnel list` no longer shows it, `dig +short t3.<domain> @1.1.1.1` returns nothing or the restored
record (ask Cloudflare's resolver; others cache), and `agentctl t3 status` / `agentctl opencode status` show the
restored pairing addresses. Then delete `<records>/tunnel.md` and tell the user what was removed.
