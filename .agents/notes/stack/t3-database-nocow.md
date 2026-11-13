---
name: t3-database-nocow
description: T3 Code's SQLite commits stall its server on fragmented btrfs; the memory guide offers no-CoW for its userdata (minipc, 2026-11-13)
---

minipc, 2026-11-13: after `MemoryMin` covered the database, T3 Code still froze and dropped clients under load.
`event-loop-stalls.ndjson` had 41 stalls of 15 s or more since 11-10 (max 243 s) with few major page faults and high
system CPU; traces showed `COMMIT` up to 9.8 s. T3's server uses `node:sqlite` (found in the runtime binary), which
is synchronous, so every slow commit blocks the event loop.

- `/home` is btrfs (`compress=zstd:1`). `statev2.sqlite` (3.6 GB) was in 531,355 extents: copy-on-write plus a
  database rewritten all day. Fix (applied by an agent with the user, recorded in minipc's `guides/memory.md`):
  stop T3, reflink backup of `userdata`, `chattr +C` the directory, rewrite the `*.sqlite*` files into it with
  `cp --reflink=never`, `quick_check`, start. Result: 39 extents.
- Cost: btrfs no longer checksums or compresses those files. Snapshots re-CoW no-CoW files, so it only lasts on a
  subvolume that isn't snapshotted regularly (minipc's snapper covers `/` only, no timeline).
- Lives in the `memory` guide's "If it fails" (and `memory-undo`), not its own guide: same symptom (T3 lags under
  load), same record, already reachable from the dashboard. agentctl itself still never touches T3's database.
- Not yet measured whether it ends the disconnects. If 15 s+ stalls continue, next is IO priority (bfq or io.cost,
  see t3-memorymin-covers-database). VACUUM skipped: ~45 MB free inside the file.
