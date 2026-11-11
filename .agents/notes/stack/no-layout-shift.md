---
name: no-layout-shift
description: Hard rule that nothing in the TUI moves on interaction, and how the layout enforces it
---

User rule (2026-11-09): no layout shift, ever. A Codex audit (gpt-6.1-sol, 6,904 frame comparisons) found 14 shift
classes, all from content-sized layout.

- Everything sits on a grid computed from terminal size only (`internal/tui/dash/view.go` constants): fixed dialog
  shell (size/position per terminal size), button slots (extras from left, Cancel and primary on the right), fixed
  list windows with a reserved "↓ N more" row, fixed description/warning/error rows, fixed fact rows, fixed-width
  buttons/columns (`skin.Button`, `skin.Row` labelW, `skin.Cell`, `skin.Clip`).
- Overflow truncates or scrolls inside its own area; it never pushes neighbors. Empty slots stay empty.
- `layout_test.go` compares rects of identified layers across many states and sizes (80x24..200x60); mutation-checked
  to catch reintroduced shifts. Give new elements layer IDs so the tests cover them.
- A second Codex audit found 5 more: report scroll reset on probe refresh, "Add a proxy" following the list length,
  multi-line text (errors, model ids) growing one-row cells, and text moving inside the pill. Now: refreshes keep the
  scroll offset, Add sat on a fixed row below a fixed-height window (now an item ending Proxies, which moves only
  when proxies are added or removed), `skin.Cell` flattens line breaks, pill fields have fixed widths. Model maps (probes, tune selection) are copied on write so value copies never share state.
- Async feedback (user, 2026-11-11: "the button needs to change when it's loading", without moving or resizing): the
  element that started an action (`m.pressed`, set in `act`/`actDialog`/form submits/list picks) shows a spinner in
  its fixed slot while `busy`; every other button dims and drops hover; hints say only ctrl+c works. Background checks
  a button started (proxy Test, update Check again) spin that button without blocking (`working`). List options
  always reserve the mark column so a spinner never shortens their label. `busy_test.go` checks size and place.
