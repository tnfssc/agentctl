---
name: claude-code-tools
description: Curated Claude Code tool and feature trims for presets, with what each costs
---

Researched 2026-11-09 (Claude Code 2.1.310, code.claude.com tools-reference, permissions, settings-reference).

- A bare tool name in `permissions.deny` removes the tool from context (saves tokens); scoped rules only block calls.
  Globs like `Cron*` work. Deferred tools cost about their name, so denying them saves little.
- Safe for nearly everyone: deny Artifact (publishes to claude.ai), PushNotification, RemoteTrigger, DesignSync,
  NotebookEdit (unless Jupyter), EnterPlanMode (Shift+Tab and /plan still work); `enableWorkflows: false`,
  `workflowKeywordTriggerEnabled: false`, `feedbackDrafts: "off"`, `spinnerTipsEnabled: false`,
  `promptSuggestionEnabled: false` (background request per reply), `awaySummaryEnabled: false` (idle tokens).
- Minimalist/headless tier: Cron* + ScheduleWakeup (/loop), Enter/ExitWorktree, ReportFindings, SendMessage+ListAgents
  (also breaks subagent resume; pair with `crossSessionInbound: "refuse"`), `disableBundledSkills` (loses /code-review,
  /verify, /loop…), `autoMemoryEnabled: false`.
- Never: Read, Edit, Write, Bash, Agent, Skill (kills user skills), TaskStop, Monitor, ToolSearch (MCP tools would all
  load upfront), ExitPlanMode (plan approval breaks). Keep `fileCheckpointingEnabled` (/rewind) except throwaway CI.
- `disableArtifact` is deprecated for `enableArtifact: false`.
