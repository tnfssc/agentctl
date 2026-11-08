# agentctl

Go CLI that routes Claude Code and Codex to their own login or to CLIProxyAPI servers. Module
`github.com/tnfssc/agentctl`, binary `agentctl`. Decisions and research live in `.agents/notes/`; read the notes for a
feature before changing it.

## Rules that are product decisions

- Claude models are only ever offered to Claude Code. Model classification fails safe: unknown counts as Anthropic.
- Agent configs are edited surgically and must round-trip: proxy then local restores the file byte-for-byte. Keys never
  go into agent configs; agents call `agentctl token <proxy>`.
- Never write to a user's real `~/.claude` or `~/.codex` while developing; use `CLAUDE_CONFIG_DIR`, `CODEX_HOME` and
  `XDG_CONFIG_HOME` pointing into `.tmp/` (tests do this via `internal/testutil`).

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

## Verifying interactive flows by hand

Run under tmux with a script (tmux's default shell is fish, so wrap commands in `sh`), then `tmux capture-pane -p`.
Agent shells may export `NO_COLOR=1`, which the app honors; unset it when checking colors.
