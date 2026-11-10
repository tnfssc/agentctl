# Cap memory for coding agents and Docker

You're helping the user keep this machine responsive when coding agents fill its memory. Agents (Claude Code, Codex,
OpenCode, Pi, and the sessions T3 Code runs) start test runners, browsers and dev servers; together with Docker
containers they can use all the RAM, and then the servers and the whole machine freeze. The goal: every agent,
however it was started, shares one group that slows down and then hits a cap well before the machine runs out;
Docker gets its own cap; T3 Code's server keeps the memory it needs. This works on Linux; on macOS only Docker can
be capped (see the macOS section). agentctl doesn't do this itself; you set it up once with the user. Show the plan
with the actual numbers and ask before writing anything.

## Look first

- `uname -s` and the RAM and swap: `free -g` on Linux, `sysctl hw.memsize` and `sysctl vm.swapusage` on macOS.
- `agentctl t3 status` and `agentctl opencode status`: which servers run here.
- Which agents are installed and where they come from: `command -v claude codex opencode pi` (and every match with
  `which -a`), so wrappers can find the real ones.
- Docker: `docker info` (rootless shows `rootless` under Security Options; Docker Desktop, OrbStack, Colima and
  Podman run containers in a VM), `docker ps -q | wc -l`, and how much the containers use now
  (`docker stats --no-stream`), so a cap doesn't starve them. If the VM is stopped, `docker info` fails; read its
  configured size instead (e.g. `colima list`) and don't start it without asking.
- Linux: whether systemd-oomd runs (`systemctl is-active systemd-oomd`, `oomctl`). Ubuntu and Fedora enable it, with
  `ManagedOOMMemoryPressure=kill` on user sessions: it kills a whole cgroup (an agent's session, or T3 Code's
  server) when memory pressure stays high, and throttling at `MemoryHigh` raises that pressure.
- `<records>/memory.md`: if it exists, this was set up before; read it and change that setup instead of adding a
  second one.

## Linux (systemd, cgroup v2)

Check `stat -fc %T /sys/fs/cgroup` prints `cgroup2fs`; without cgroup v2 these limits don't apply, so say so and
stop.

Work with drop-ins only: never edit unit files T3 Code or Docker write, because their updates rewrite them. Name
drop-ins `99-<what>.conf` so they sort after T3 Code's own and win.

1. Find the units. `systemctl --user list-units --type=slice,service | grep -i t3` shows T3 Code's server (e.g.
   `t3code.service`) and its agents' slice (e.g. `app-t3code-agents.slice`). `systemctl --user cat <unit>` shows
   the limits T3 Code sets itself, and `systemctl --user show -p ControlGroup <unit>` where it lives under
   `/sys/fs/cgroup`. For Docker, see where a container's processes land:
   `cat /proc/$(docker inspect -f '{{.State.Pid}}' <container>)/cgroup`.

   The agents' slice is the one group every agent will share: T3 Code's agents' slice when T3 Code runs here (its
   name is T3 Code's, and a slice can't be moved under another), else a new `app-agents.slice`. Slices need no
   unit file; starting something in one creates it, and a drop-in sets its limits.
2. Pick sizes with the user, as shares of RAM, and keep the two caps together under about 85% so T3 Code's server,
   the desktop and the system always have room:
   - agents' slice, shared by every agent: `MemoryHigh` 55% (they slow down there), `MemoryMax` 65% (hard cap),
     `MemorySwapMax` 8G or less if swap is smaller (zram swap lives in RAM too);
   - Docker containers: `MemoryHigh` 15%, `MemoryMax` 20%, or more if they already use more (step "Look first");
   - T3 Code's server, if it runs here: `MemoryMin` 2G, so reclaim never starves it.
   Round to whole gigabytes; the numbers below are an example, not for copying. Tell the user what happens: near
   `MemoryHigh` work slows down and can look hung; at a cap Linux kills the largest process in that group, usually
   a test runner or browser rather than the agent; their own containers share the Docker cap. With systemd-oomd on,
   explain that it may kill a whole session first, and offer `ManagedOOMMemoryPressure=auto` in the agents' slice
   drop-in if they'd rather rely on the caps.
3. Write the drop-ins, e.g. `~/.config/systemd/user/app-t3code-agents.slice.d/99-memory-budget.conf`:
   ```ini
   [Slice]
   MemoryHigh=36G
   MemoryMax=44G
   MemorySwapMax=8G
   ```
   (or `app-agents.slice.d/` without T3 Code) and `~/.config/systemd/user/t3code.service.d/99-memory-guarantee.conf`
   with `[Service]` `MemoryMin=2G`.
4. Put agents started outside T3 Code in the same slice. A process stays in the cgroup it was started in, so
   `claude`, `codex`, `opencode` and `pi` run from a terminal land in that terminal's group, uncapped. Wrap each
   installed one in a small script in its own directory, e.g. `~/.local/share/agentctl/agents-bin/claude`, that
   finds the real command further down `PATH` and runs it in the slice, unless it already runs there (T3 Code's
   sessions do) or the user's systemd isn't reachable:
   ```sh
   #!/bin/sh
   # agentctl guide memory: run in the shared agents' slice
   slice=app-t3code-agents.slice
   dir=$(dirname "$0") name=$(basename "$0")
   real=$(PATH=$(printf %s "$PATH" | tr : '\n' | grep -vxF "$dir" | paste -sd: -) command -v "$name") || exit 127
   [ "$(readlink -f "$real")" != "$(readlink -f "$0")" ] || { echo "$name: only this wrapper is on PATH" >&2; exit 127; }
   case $(cat /proc/self/cgroup) in */"$slice"/*) exec "$real" "$@" ;; esac
   [ -S "${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/bus" ] || exec "$real" "$@"
   exec systemd-run --user --scope --quiet --collect --slice="$slice" -- "$real" "$@"
   ```
   Put that directory first on `PATH` for login and interactive shells (the user's shell config: `~/.profile`,
   `~/.bashrc`, `~/.zshrc`, `~/.config/fish/conf.d/`), and in `~/.config/environment.d/` so services the user's
   systemd starts see it too (`systemctl --user set-environment PATH=...` for the running manager). Version managers
   like mise put their paths first when the shell config activates them, often on every prompt: add the line after
   that activation, and in fish also on the `fish_prompt` event. Check with a new shell's `command -v claude`. Other
   shims already first on `PATH` (an older proxy setup, say) must come after the wrappers, or they bypass them; ask
   the user before moving or retiring them. `systemd-run --scope` keeps the terminal attached, so interactive agents work as
   before; check one with `claude --version` and `cat /proc/<pid>/cgroup` while it runs.

   OpenCode's web server isn't a systemd unit: every `opencode` command starts it if it isn't running, and it stays
   in the group of whatever started it. With the wrapper on `PATH` (agentctl calls `opencode` from `PATH` too), the
   next start puts it in the slice; offer `agentctl opencode restart` now, which stops running agent turns.
5. Docker depends on where containers land:
   - Rootless Docker under the user's systemd puts containers in the user manager's `user.slice`; a drop-in at
     `~/.config/systemd/user/user.slice.d/99-docker-memory-budget.conf` (`[Slice]` with the two limits) caps them.
     First check with `systemd-cgls --user-unit user.slice` that nothing else the user cares about runs there, and
     that the memory controller is delegated to the user manager (`memory` in `cgroup.controllers` under its
     `ControlGroup`); without it, user-level limits do nothing.
   - Rootful Docker puts containers in the system's `system.slice`, which holds everything else too; don't cap
     that. Instead offer a dedicated slice: `/etc/systemd/system/containers.slice` with the limits (a name without
     dashes, which systemd reads as nesting), and `"cgroup-parent": "containers.slice"` merged into
     `/etc/docker/daemon.json` after backing it up. That needs sudo and restarting Docker: containers without a
     restart policy stop and stay stopped unless `live-restore` is on, so ask first. Existing containers keep their
     old place until they're recreated, which loses anything not in a volume; leave that to the user.
6. `systemctl --user daemon-reload` (and `sudo systemctl daemon-reload` for system changes), then read the cgroup
   files (see Check). If a running unit still shows its old values, apply them now with
   `systemctl --user set-property --runtime <unit> <Property>=<value>` rather than restarting T3 Code, which stops
   running agent turns.
7. `MemoryMin` only protects as far as every parent cgroup is protected too: the kernel limits a group's protection
   to its ancestors'. Read `memory.min` along the path from `/sys/fs/cgroup` down to T3 Code's server. If a parent is
   0, the guarantee does nothing until each of them gets at least as much: `app.slice` (and any `app-*.slice` in
   between) with user drop-ins, and `user.slice`, `user-<uid>.slice` and `user@<uid>.service` with sudo drop-ins
   (`/etc/systemd/system/user.slice.d`, `user-.slice.d`, `user@.service.d`). Name each `99-memory-guarantee.conf`.
   Explain this and ask; without it, say the server has no real guarantee.

## macOS

macOS has no equivalent: launchd can't cap the memory of a group of processes, and its resource limits aren't
enforced for resident memory, so there is no way to cap agents' sessions together. Don't touch T3 Code's launchd
plist. macOS compresses and swaps memory rather than freezing outright, so this matters less there. What can be
capped is Docker, because containers run in a VM with a fixed size:

- Docker Desktop: Settings → Resources → Memory.
- OrbStack: `orb config get memory_mib`, then `orb config set memory_mib <MiB>`.
- Colima: `colima list` shows it; `colima stop && colima start --memory <GiB>`.
- Podman: `podman machine stop && podman machine set --memory <MiB> && podman machine start`.

Suggest 15-20% of RAM like on Linux, or what the containers need if that's more, and note the previous value
first. Resizing restarts the VM and every container in it; ask, and don't start a VM that was stopped.

## Check

- `systemctl --user show <unit> -p MemoryHigh -p MemoryMax -p MemorySwapMax -p MemoryMin` shows the new values, and
  the `memory.high`, `memory.max` and `memory.min` files under each unit's `ControlGroup` match.
- A container's cgroup path (step 1) is inside the capped slice, and so is an agent started from a new terminal and
  OpenCode's server (`cat /proc/<pid>/cgroup`).
- With systemd-oomd on, `oomctl` shows the agents' slice with the setting you chose.
- On macOS, the VM reports the new size: `docker info --format '{{.MemTotal}}'`.

## Record what you did

Write `<records>/memory.md` (create the directory) so the change can be undone later, also by
`agentctl guide memory-undo`: the date, the machine, every file you created or changed with its full path, the
values, previous values you replaced (including Docker VM sizes and the `daemon.json` backup), the wrappers and
every shell or environment file you added the `PATH` line to, and anything you restarted. Then tell the user the limits are live and how to undo them.

## If it fails

- A value doesn't show up: a later drop-in or the unit itself overrides it; `systemctl --user cat <unit>` lists
  every file in order.
- `MemorySwapMax` has no effect: swap accounting is off in the kernel (`swapaccount=0`), or there's no swap.
- The cap is reached all the time: the share is too small for this user's work; raise it with them rather than
  removing it.
