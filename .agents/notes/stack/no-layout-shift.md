---
name: no-layout-shift
description: Hard rule that nothing in the TUI moves on interaction, and how the layout enforces it
---

User rule (2026-11-09): no layout shift, ever. A Codex audit (gpt-6.1-sol, 6,904 frame comparisons) found 14 shift
classes, all from content-sized layout.

- Everything sits on a grid computed from terminal size only (`internal/tui/dash/view.go` constants): fixed dialog
  shell (size/position per terminal size), button slots (extras from left, Cancel and primary on the right), fixed
  list windows with a reserved "↓ N more" row, fixed description/warning/error rows, fixed fact rows, fixed-width
  buttons/pill/columns (`skin.Button`, `skin.Pill`, `skin.Row` labelW, `skin.Cell`, `skin.Clip`).
- Overflow truncates or scrolls inside its own area; it never pushes neighbors. Empty slots stay empty.
- `layout_test.go` compares rects of identified layers across many states and sizes (80x24..200x60); mutation-checked
  to catch reintroduced shifts. Give new elements layer IDs so the tests cover them.
- Data changes (adding a proxy reorders the sorted sidebar) are not interaction shifts and are allowed.
