---
name: neo-brutalist-dashboard
description: Why the dashboard is a layered, mouse-driven neo-brutalist scene and the rendering gotchas found
---

Decided 2026-11-09: the user asked for a "fancy glittery", neo-brutalist, mouse-driven UI that feels like a web page,
replacing the plain list dashboard.

- Built as Lip Gloss v2 layers + compositor; `Compositor.Hit` gives the clicked/hovered element, wired through
  `tea.View.OnMouse` so hit testing always matches the frame on screen. Dialogs are panels at higher Z; clicking any
  non-dialog layer closes the dialog (web modal behavior); the page behind is drawn with a dimmed palette.
- Ink (borders, shadows) follows the terminal background (cream on dark, near-black on light); fills stay loud.
- Text inside fills must carry the fill background per span: Lip Gloss resets after each styled span.
- Add-proxy form is three bubbles textinputs (clickable boxes), not huh: huh fields are not mouse-addressable.
- freeze mis-renders `ESC[1m` after a color (drops fg); tmux capture loses attributes. Use the snapshot test.
