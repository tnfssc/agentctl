package dash

import (
	"strconv"
	"strings"

	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// Compact layout, for terminals narrower than minWidth or shorter than
// minHeight, such as SSH on a
// phone: one page at a time (the home list, or one item's page with a Back
// button), full-screen dialogs and full-width buttons. Like the wide layout,
// every position comes from the terminal size alone.

const (
	compactMinWidth  = 40
	compactMinHeight = 16
)

// compact reports whether the terminal is too small for the wide layout.
func (m Model) compact() bool { return m.width < minWidth || m.height < minHeight }

// tooSmall reports whether even the compact layout doesn't fit.
func (m Model) tooSmall() bool { return m.width < compactMinWidth || m.height < compactMinHeight }

// shortHeader is the one-line header terminals too short for the logo get.
func (m Model) shortHeader() bool { return m.height < logoRows }

// contentTop is the first row under the header's hairline.
func (m Model) contentTop() int {
	if m.shortHeader() {
		return 2
	}
	return pageTop
}

// bodyTop is the wide layout's first row of sidebar and detail: one blank
// row under the header's hairline, none under a short header.
func (m Model) bodyTop() int {
	if m.shortHeader() {
		return m.contentTop()
	}
	return m.contentTop() + 1
}

// content is the compact page area between the header and the footer.
func (m Model) content() rect {
	top := m.contentTop()
	return rect{0, top, m.width, m.height - 2 - top}
}

func (m Model) drawCompactHeader(sc *scene, p skin.Palette) {
	if m.shortHeader() {
		sc.add(p.Wordmark(), 1, 0, 1, "")
		sc.add(p.HLine(m.width), 0, 1, 1, "")
		return
	}
	sc.add(p.Logo(), 1, navTop, 1, "")
	sc.add(p.HLine(m.width), 0, pageTop-1, 1, "")
}

// drawCompactDetail is one item's page: Back, what it is, its facts, and its
// buttons in fixed slots at the bottom.
func (m Model) drawCompactDetail(sc *scene, p skin.Palette) {
	c := m.content()
	d := m.detail(p)
	backW := 10
	sc.add(p.Button("‹ Back", backW, false, m.hover == "back"), 1, c.Y, 2, "back")
	textX := 1 + backW + 2
	textW := max(1, m.width-textX-1)
	sc.add(skin.Cell(p.Label(d.label), textW), textX, c.Y+1, 2, "")
	sc.add(skin.Cell(p.Heading(d.heading), textW), textX, c.Y+2, 2, "")

	// Buttons: the first slot full width, the other two side by side.
	w := m.width - 2
	half := (w - 1) / 2
	btnTop := c.Y + c.H - 6
	slots := [3]rect{{1, btnTop, w, 3}, {1, btnTop + 3, half, 3}, {1 + half + 1, btnTop + 3, w - half - 1, 3}}
	for i, b := range d.buttons {
		if b.label != "" {
			sc.add(p.Button(fit(b.label, slots[i].W), slots[i].W, b.primary, m.hover == b.id), slots[i].X, slots[i].Y, 2, b.id)
		}
	}

	// Description and facts share the rows between the heading and the
	// buttons; how many each gets depends on the terminal height only.
	// Facts win: they hold what changes (a proxy's error, the model).
	top := c.Y + 3
	info := btnTop - top
	gap := 0
	if info >= factRows+3 {
		gap = 1
	}
	bodyRows := min(bodyLines, max(1, info-factRows-gap))
	for i, line := range skin.Clip(d.body, w, bodyRows) {
		sc.add(skin.Fg(p.Muted).Render(line), 1, top+i, 2, "")
	}
	factTop := top + bodyRows + gap
	for i := range min(factRows, btnTop-factTop) {
		sc.add(skin.Cell(d.facts[i], w), 1, factTop+i, 2, "fact:"+strconv.Itoa(i))
	}
}

// shortLabels are button labels for slots too narrow for the full one.
var shortLabels = map[string]string{
	"Reset to default": "Reset",
	"Check and add":    "Add",
	"Test again":       "Test",
	"Pair device":      "Pair",
	"Copy command":     "Copy",
	"Switch route":     "Switch",
	"Set up service":   "Set up",
	"Allow models":     "Allow",
	"Check again":      "Check",
}

// fit picks the label to show in a button slot width cells wide.
func fit(label string, width int) string {
	if len([]rune(label))+4 <= width {
		return label
	}
	if s, ok := shortLabels[label]; ok {
		return s
	}
	if strings.HasPrefix(label, "Revoke idle") {
		return "Idle" + strings.TrimPrefix(label, "Revoke idle")
	}
	return label
}
