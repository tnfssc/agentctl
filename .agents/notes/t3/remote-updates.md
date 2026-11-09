---
name: remote-updates
description: How "Update" in the T3 Code desktop app also updates a remote server, and what agentctl must keep for it to work
---

The desktop app asks a connected server to update (`server.updateServer` RPC, scope `environment:maintain`). The
server downloads the target runtime and sends `request-update` over IPC to its service launcher, which backs up the
DB, starts the new version as a trial, commits on `prepared` or rolls back, and records the outcome in
`~/.t3/runtime/service-state.json` (`update.status`: committed / rolled-back / failed, with `reason`).

It only works when the server runs under t3's own service launcher (`ExecStart=<runtime>/t3 __service-launcher`,
installed by `t3 service install` / `t3 up`). A server started by hand or by a custom unit refuses ("Remote updates
require the T3 Code background service"). The launcher also refuses downgrades and Stable↔Nightly crossings.

Evidence: minipc's 0.1.7→0.1.8 update committed 2026-11-06 17:08, the same minute the Mac desktop app restarted for
its own update (`~/.t3/runtime/desktop-update-restart`). This is the "I fixed it on minipc" behavior: nothing to add
in agentctl beyond installing the service the upstream way and showing the last update in the dashboard.

Desktop and server should share a channel; agentctl always installs and updates nightly (`t3 update --channel
nightly --yes`).
