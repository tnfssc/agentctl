---
name: t3-database-nocow
description: Agents' SQLite databases fragment on btrfs and T3 Code's slow commits stall its server; the memory guide offers no-CoW for T3, OpenCode and Codex, not Claude Code or Pi (minipc, 2026-11-13)
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
- Extended to OpenCode and Codex the same day (user: "lets work em anyway", after measuring little gain). Fragmentation
  is judged by average extent size, not count: btrfs stores compressed data in extents of at most 128 KiB, so a
  compressed file's floor is size/128 KiB (opencode.db: 219 extents at a 175 floor, not fragmented). Codex's
  `logs_2.sqlite` was 30x its floor (17,668 extents, 72 MB), `thread_history_1.sqlite` 3.5x (17,867, 635 MB).
  After the rewrite 363 and 5,190: the copies landed in scattered free space inside allocated chunks (742 GiB
  allocated, 622 used), unlike T3's, which got 39.
- Claude Code and Pi are left out: no SQLite, append-only JSONL at the compression floor (a 161 MB transcript: 1,410
  extents, floor 1,290); no-CoW would only lose zstd on 3+ GB of text.
- Lives in the `memory` guide's "If it fails" (and `memory-undo`), not its own guide: same symptom (T3 lags under
  load), same record, already reachable from the dashboard. agentctl itself still never touches T3's database.
- Not yet measured whether it ends the disconnects. If 15 s+ stalls continue, next is IO priority (bfq or io.cost,
  see t3-memorymin-covers-database). VACUUM skipped: ~45 MB free inside the file.
