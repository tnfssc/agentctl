---
name: t3-memorymin-covers-database
description: T3 Code's server MemoryMin must fit its SQLite database, or it re-reads it from disk under load; IOWeight is inert under kyber (minipc, 2026-11-12)
---

minipc, 2026-11-12: with CPU and memory caps live, T3 Code still lagged and dropped connections under load. An
AI agent's analysis blamed git spawned across worktrees and proposed `MemoryMin=4G` plus IO weights; measured
instead:

- The reads come from the `t3 serve` process itself (`/proc/<pid>/io` read_bytes ~1 TB in 22h, about 600 GB of it
  charged to NVMe in `t3code.service`'s `io.stat`). Its open files: `~/.t3/userdata/statev2.sqlite`, 3.6 GB (+65 MB WAL).
- `workingset_refault_file` 20.5M vs NVMe `rios` 21.8M, `pgsteal_kswapd` ~400 GB: reclaim drops the database's
  cached pages and the server reads them back. Server's cgroup sat at the 2G floor (0.65G anon, 1.4G file), IO
  pressure high while CPU/memory pressure ~0, `sock_throttled` 19260.
- So `MemoryMin` = database size + ~2G, on the server and every parent; 4G would not have fit. Set 6G (user
  drop-ins live 2026-11-12; the three `/etc` parents need sudo from the user). The guide sizes it this way now.
- `IOWeight=1000` on the server does nothing: NVMe scheduler is kyber and `io.cost.qos` is empty; io.weight applies
  only under bfq or io.cost. The io controller is delegated to the user manager, so it would work once io.cost is on.
  User chose to defer io.cost until MemoryMin is measured.
- A 3.6 GB state DB read this hard is T3's own problem (upstream); agentctl never touches T3's database.
