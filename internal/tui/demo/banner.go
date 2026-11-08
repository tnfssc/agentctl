package demo

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/tnfssc/cliproxy-cli/internal/tui/theme"
)

// glyphs spells CLIPROXY in the "ANSI Shadow" figlet font. Every row of a
// glyph has the same cell width so rows can be concatenated.
var glyphs = [][6]string{
	{" ██████╗", "██╔════╝", "██║     ", "██║     ", "╚██████╗", " ╚═════╝"},
	{"██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"},
	{"██╗", "██║", "██║", "██║", "██║", "╚═╝"},
	{"██████╗ ", "██╔══██╗", "██████╔╝", "██╔═══╝ ", "██║     ", "╚═╝     "},
	{"██████╗ ", "██╔══██╗", "██████╔╝", "██╔══██╗", "██║  ██║", "╚═╝  ╚═╝"},
	{" ██████╗ ", "██╔═══██╗", "██║   ██║", "██║   ██║", "╚██████╔╝", " ╚═════╝ "},
	{"██╗  ██╗", "╚██╗██╔╝", " ╚███╔╝ ", " ██╔██╗ ", "██╔╝ ██╗", "╚═╝  ╚═╝"},
	{"██╗   ██╗", "╚██╗ ██╔╝", " ╚████╔╝ ", "  ╚██╔╝  ", "   ██║   ", "   ╚═╝   "},
}

// bannerRows joins the glyphs into the six rows of the big title.
var bannerRows = func() [6][]rune {
	var rows [6][]rune
	for _, g := range glyphs {
		for i, part := range g {
			rows[i] = append(rows[i], []rune(part)...)
		}
	}
	return rows
}()

// bannerWidth is the cell width of the big title.
var bannerWidth = len(bannerRows[0])

// palette is long enough that one full cycle spans the banner and a bit.
var palette = theme.Gradient(bannerWidth + 20)

// renderBanner paints the title with the brand gradient shifted by frame, so
// successive frames scroll the colors across it. Narrow terminals get the
// word in plain gradient text instead.
func renderBanner(frame, maxWidth int) string {
	if maxWidth < bannerWidth {
		return paint([]rune("cliproxy"), frame, true)
	}
	lines := make([]string, len(bannerRows))
	for y, row := range bannerRows {
		// Offset each row a little so the sweep runs diagonally.
		lines[y] = paint(row, frame+y*2, false)
	}
	return strings.Join(lines, "\n")
}

func paint(runes []rune, frame int, bold bool) string {
	var b strings.Builder
	n := len(palette)
	for x, r := range runes {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		c := palette[((x-frame)%n+n)%n]
		b.WriteString(lipgloss.NewStyle().Foreground(c).Bold(bold).Render(string(r)))
	}
	return b.String()
}
