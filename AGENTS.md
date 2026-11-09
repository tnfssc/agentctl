# agentctl

Go CLI that routes Claude Code, Codex, OpenCode (v2) and Pi to their own login or to CLIProxyAPI servers, and runs
T3 Code on the same machine. Module `github.com/tnfssc/agentctl`, binary `agentctl`. Decisions and research live in
`.agents/notes/`; read the notes for a feature before changing it.

## Rules that are product decisions

- Claude models only ever run through Claude Code: other agents' model lists leave them out, and OpenCode reaches them
  only through claude-agent, which runs the real `claude`. Model classification fails safe: unknown counts as Anthropic.
- Agent configs are edited surgically and must round-trip: proxy then local restores the file byte-for-byte. Keys never
  go into agent configs; agents call `agentctl token <proxy>`. OpenCode can't run a command, so its config points at a
  0600 copy in agentctl's own `keys/` directory.
- Never write to a user's real `~/.claude`, `~/.codex`, `~/.config/opencode` or `~/.pi` while developing; use
  `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `XDG_CONFIG_HOME`/`XDG_DATA_HOME` and `PI_CODING_AGENT_DIR` pointing into `.tmp/`
  (tests do this via `internal/testutil`). OpenCode and Pi configs are JSONC: edit them with `internal/jsonc`, never
  gjson/sjson, which match keys inside comments.
- T3 Code is managed only through the upstream `t3` CLI and the state it writes (`internal/t3`); agentctl never writes
  T3's units, plists or `service.env`. Never run `t3` or agentctl's T3 commands against the real `~/.t3` while
  developing: they install launchd/systemd services and migrate its database. Set `T3CODE_HOME` into `.tmp/` (tests
  use `testutil.T3`, a fake `t3`).

## Tooling

Go and golangci-lint come from `mise.toml`; run everything through mise (`mise run check`, `mise exec -- go ...`)
so the pinned Go is used. `GOTOOLCHAIN=local` is set there so Go never silently downloads another toolchain.

## Stack

Charm v2 under `charm.land/*/v2` import paths (Bubble Tea, Bubbles, Lip Gloss, fang). Never mix in the
pre-v2 `github.com/charmbracelet/<lib>` paths for libraries that share types; a v1 `lipgloss.Style` does not satisfy
a v2 API. The `charmed-*` skills in `.agents/skills` describe the pinned v2 APIs; prefer `go doc` over upstream
READMEs, which still show v1 signatures.

Commands are cobra, executed through `fang.Execute`, which already sets `SilenceUsage`/`SilenceErrors` and prints
errors styled to stderr. Commands write through `cmd.OutOrStdout()` so tests can capture them.

## UI conventions

- Styled output goes through lipgloss print functions (downsampled, dropped when piped); prompts are huh forms on
  stderr, only when stdin and stdout are terminals, otherwise commands need `--yes`.
- Styles come from `internal/tui/theme`.

## No layout shift (hard rule)

Nothing on screen may move or resize because of hover, selection, scrolling, focus, spinners, loading, status
messages, probe results or dialogs opening. Reserve space for anything that can appear (fixed-width columns, fixed
rows for messages, padded labels, scroll windows of constant height with constant-size indicators). Every UI change
must keep this; tests in `internal/tui/dash` check it.

## Dashboard

Bare `agentctl` opens a small dashboard (`internal/tui/dash`, look in `internal/tui/skin`, after voidzero.dev):
hairlines, a sidebar, a detail pane, outlined buttons, mouse click/hover/wheel via `View().OnMouse` and compositor
hit testing. Keep it simple and cheap over SSH: nothing animates except spinners while loading, an idle dashboard
schedules no frames, and SSH sessions get no hover tracking. Lip Gloss resets styles after every span, so text inside
a filled shape must carry the fill (`skin.On`).

## Verifying interactive flows by hand

Run under tmux with a script (tmux's default shell is fish, so wrap commands in `sh`), then `tmux capture-pane -p`.
Agent shells may export `NO_COLOR=1`, which the app honors; unset it when checking colors.
