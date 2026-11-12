---
name: management-key
description: Why agentctl keeps the CLIProxyAPI management key's plaintext, and how rotation applies
---

CLIProxyAPI (v8.0.67) bcrypt-hashes a plaintext `management.secret-key` in config.yaml when it starts and writes the
hash back, so the key can't be read from the config afterwards. Verified 2026-11-12 with the real binary: the file
held `$2a$10$...` after start, and the plaintext opened `/v0/management/config` (200, wrong key 401).

- agentctl generates the key at install and keeps the plaintext in `~/.config/agentctl/keys/cliproxyapi.management`
  (0600; the dot keeps it apart from OpenCode's per-proxy key copies, whose names can't contain one). That copy is
  the only way `agentctl cliproxy keys` can show it.
- Without a copy (deleted, or a config from elsewhere) the status says so and points at `keys rotate-management`.
- Rotation writes a new plaintext and restarts a running server so it hashes and applies it: old key 401, new 200.
- `MANAGEMENT_PASSWORD` would have kept a plaintext usable, but upstream sets `allowRemoteOverride` whenever it is set,
  opening management to non-local clients. Rejected.
