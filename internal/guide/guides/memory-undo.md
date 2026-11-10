# Remove the memory caps for coding agents and Docker

You're helping the user undo the memory limits set up with `agentctl guide memory`, putting back what was there
before. Show what you'll remove and ask before removing or restarting anything.

## Find what was set up

Read `<records>/memory.md`: it lists every file created or changed, the values, and what they replaced. If it's
missing, look for the usual places and confirm each with the user before touching it:

- Linux: `99-*` drop-ins under `~/.config/systemd/user/*.d/` (e.g. `app-t3code-agents.slice.d`, `user.slice.d`,
  `t3code.service.d`, `app.slice.d`) and under `/etc/systemd/system/*.d/` (`user.slice.d`, `user-.slice.d`,
  `user@.service.d`), with memory settings in them; a `containers.slice` in `/etc/systemd/system/` and
  `cgroup-parent` in `/etc/docker/daemon.json`; agent wrappers in `~/.local/share/agentctl/agents-bin/` and the
  lines that put it on `PATH` (shell configs, `~/.config/environment.d/`).
- macOS: the Docker VM's memory size (Docker Desktop, OrbStack, Colima or Podman). Without a record there's no
  previous size; ask the user what they want.

Never remove limits T3 Code or Docker set themselves; only what the setup added.

## Undo

1. Delete the drop-ins the setup created, and empty `*.d` directories it left behind.
2. Take the wrappers' directory off `PATH` in every file the record names, then delete the wrappers. Running
   agents keep their slice until they exit; OpenCode's server until it restarts.
3. For a rootful Docker slice: take `cgroup-parent` out of `/etc/docker/daemon.json` (restore the backup, or delete
   the file if the setup created it), delete `/etc/systemd/system/containers.slice`, and restart Docker after
   asking: containers without a restart policy stop and stay stopped unless `live-restore` is on. Containers created
   meanwhile keep the old parent until they're recreated.
4. `systemctl --user daemon-reload`, and `sudo systemctl daemon-reload` if anything system-wide changed. If a
   running unit keeps an old value, `systemctl --user set-property --runtime <unit> <Property>=` clears it without
   a restart.
5. On macOS, put the Docker VM back to the previous size from the record, if the user wants it back; that restarts
   the VM and its containers.

## Check

`systemctl --user show <unit> -p MemoryHigh -p MemoryMax -p MemorySwapMax -p MemoryMin` (and `systemctl show` for
system units) shows the values T3 Code or systemd set without the drop-ins (`infinity` or T3 Code's own),
`/etc/docker/daemon.json` has no `cgroup-parent`, and on macOS the VM reports the size the user chose. Then delete
`<records>/memory.md`, or if the user kept some of it, rewrite it to list only what's still in place, and tell the
user what's back to how it was.
