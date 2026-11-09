---
name: self-update
description: How `agentctl update` replaces itself, and why it works like install.sh
---

- Latest tag comes from the `github.com/<repo>/releases/latest` redirect (HEAD, no redirects followed): no API token,
  no API rate limit.
- Downloads `agentctl-<os>-<arch>` and `SHA256SUMS` from that tag, checks the hash, stages the file beside the
  running binary (symlinks resolved, so a version manager's link stays a link) and renames over it. Same steps as
  install.sh. The path never changes, so agents' `apiKeyHelper`/auth commands that call `agentctl token` keep
  working.
- Source builds (version not starting with `v`, e.g. `dev` from `go run`/`mise run install` on a dirty tree) refuse
  to update; they'd be replaced by a release silently.
- Verified 2026-11-09: a scratch v0.9.0 build updated itself to v1.0.0 from the real release.
