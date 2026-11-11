---
name: screen-sizes
description: How the dashboard fits phones over SSH, and the user's calls on it (2026-11-09)
---

User's calls: phones over SSH matter (portrait ~40-50 cols, landscape ~90-110×18-22); a stacked compact layout below
the wide one; 40×16 minimum; bigger touch targets everywhere, not only on phones.

- Layout is a function of terminal size: wide from 80×20 (one-line header under 24 rows, three-row logo from 24),
  compact below that, a "needs at least 40×16" message below the minimum.
- Compact: a home list, then one item's page (three-row Back, heading, description and facts, buttons at the
  bottom where a thumb reaches: slot 1 full width, slots 2-3 halves). Number keys 1-3 press the buttons in both
  layouts. Dialogs fill the page with three equal button slots; long labels swap for short ones (`fit`). Forms use
  one-row inputs, label and input on one row when even that doesn't fit; the choice is the same for every form.
  Facts win over description rows on small pages because they hold what changes (a proxy's error).
- The home list (sidebar in wide, whole page in compact) has two-row items, a scrolling window, and a reserved
  "more" row that pages through the list when tapped (not every phone terminal sends swipes as wheel events). "Add a
  proxy" was pinned below until 2026-11-11; it now ends the Proxies group (see dashboard-style.md).
- No-layout-shift with scrolling: the frame (window, more row, pinned item, footer, button slots) never moves;
  items move only when a new selection scrolls the list, and tests compare item positions per selection.
