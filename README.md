# cliproxy

A terminal front end for cliproxy, built on the [Charm](https://charm.land) v2 stack.

The real commands are still to come. For now there is an animated demo of the UI stack.

## Run

Tools are pinned in `mise.toml`.

```sh
mise install
mise run demo          # animated demo; r replays, q quits
mise run check         # vet, golangci-lint, race tests, build
mise run build         # bin/cliproxy with version from git
```

Or install straight from source:

```sh
go install github.com/tnfssc/cliproxy-cli/cmd/cliproxy@latest
```

## Layout

```
cmd/cliproxy/        main, only calls cli.Execute
internal/cli/        cobra commands, run through fang for styled help and errors
internal/tui/theme/  palette and shared Lip Gloss styles
internal/tui/demo/   the demo Bubble Tea model
```
