# cliproxy-cli

Go CLI with a Bubble Tea TUI. Module `github.com/tnfssc/cliproxy-cli`, binary `cliproxy`.

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

## TUI conventions

- Styles come from `internal/tui/theme`, built from `tea.BackgroundColorMsg` (assume dark until it arrives).
- Layout from `tea.WindowSizeMsg`; views must not exceed the terminal width (tests check this).
- Timed or async events carry a generation number when a screen can restart, so stale timers are dropped.
- Test models by feeding messages to `Update` and inspecting the model and `View().Content`.

## Verifying a TUI by hand

`tmux new-session -d -x 100 -y 34 'bin/cliproxy demo'` then `tmux capture-pane -ep`. Agent shells may export
`NO_COLOR=1`, which the app honors; unset it (`env -u NO_COLOR`) when checking colors.
