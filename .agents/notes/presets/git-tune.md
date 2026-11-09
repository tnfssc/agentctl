---
name: git-tune
description: The git tune target, and why commit signing is a printed guide instead of a setting (2026-11-09)
---

- `tune git` edits the global git config (`$GIT_CONFIG_GLOBAL`, else ~/.gitconfig unless only the XDG file exists)
  by running `git config --file` on a temp copy, so git keeps its own format and the change still shows as a diff
  with a backup. Reset unsets keys and removes sections it emptied; a file left empty is deleted.
- Items (all recommended): merge on pull (`pull.rebase=false`, `pull.ff=true`), `push.autoSetupRemote`,
  `merge.conflictStyle=zdiff3`, `rerere.enabled`, `diff.algorithm=histogram`, `fetch.prune`. The user already had
  the first two by hand; they back the global instruction "sync branches by merging; never rebase or force-push
  pushed work".
- Signing (user's call): agentctl never creates, stores or uploads keys. `agentctl guide signing` prints an embedded
  doc (`internal/guide/guides/signing.md`) written for a coding agent; the dashboard's Git › Signing shows that
  command to paste into an agent, with a clipboard copy only in local sessions (no clipboard over SSH). Guides work
  like a skill that is never installed: printed on demand from the binary. New setups agentctl won't own go there.
