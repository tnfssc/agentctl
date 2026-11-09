---
name: opencode-claude-agent-connect
description: Why OpenCode listed no Claude models, and why agentctl connects claude-agent on every OpenCode route
---

Seen 2026-11-10 on the Mac: OpenCode routed to minipc with `providers.claude-agent.settings` written, yet no Claude
models in the web app or CLI. `opencode auth list` had no Claude Agent.

claude-agent (anomalyco/opencode `packages/core/src/plugin/provider/claude-agent/provider.ts`) registers with
`activation: "auto"` but its model list is a catalog saved from probing `claude`, and the probe only runs once a
claude-agent connection exists. No connection, empty catalog, no models. The connection is an "external" method
(`opencode auth login claude-agent --method claude-login`, non-interactive with both given): it records only that the
user chose Claude Code's login; no token. After connecting, the models appeared in about 20 s (35 with a cloaking
proxy: aliases, dated ids and `claude-fable-5-dd-*` disguised proxy models).

So agentctl connects it after any OpenCode route (`use opencode ...`, dashboard), applied or already in place: it is
the sanctioned way Claude reaches OpenCode (through the real `claude`). It checks `auth list` first so it never adds a
second connection, and goes through the background service (starting it if needed) so the running server probes at
once. Failure is a warning; the route is already in place. The plan warning telling users to do it by hand is gone.
