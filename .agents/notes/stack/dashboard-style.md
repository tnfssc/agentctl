---
name: dashboard-style
description: Dashboard look and performance decisions after two rejected designs
---

2026-11-09. Rejected: plain list (too bare), neo-brutalist cards with shimmer and sparkles ("ugly", "1990s").
The user pointed at voidzero.dev; then asked to stop overdoing it and keep things simple.

- Kept: voidzero-like hairline layout, labels, muted body text, outlined/solid buttons, status pill, sidebar + detail,
  mouse support. Dropped: gradient art panel, top strip, fake terminal card, all ambient animation.
- Must stay fast over SSH (user requirement): no animation when idle, spinners only while loading, CellMotion mouse
  mode over SSH (no hover), full repaint only on start/resize (~7 KB). Minimum terminal 80x24; layout rules in no-layout-shift.md.
- freeze hangs or mis-renders on these frames; don't spend time on screenshots.
