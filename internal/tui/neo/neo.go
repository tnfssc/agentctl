// Package neo is agentctl's neo-brutalist look for the terminal: loud flat
// fills, thick ink borders, hard offset shadows, chunky press-in buttons and
// a shimmering block-letter title.
//
// Lip Gloss resets styles after every span, so text inside a filled shape
// must carry the fill itself; build such text with Palette.On.
package neo

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Palette holds the colors for one terminal background.
type Palette struct {
	IsDark bool
	Ink    color.Color // borders, shadows and outline text: high contrast with the terminal
	Black  color.Color // text on fills, always dark
	Muted  color.Color // secondary text on the terminal background

	Yellow, Pink, Cyan, Lime, Red, Violet, Paper color.Color

	// Shimmer is the cyclic gradient the title's blocks move through.
	Shimmer []color.Color
}

// NewPalette returns colors that read on a dark or light terminal. Fills
// stay loud either way; only the ink flips.
func NewPalette(isDark bool) Palette {
	p := Palette{
		IsDark: isDark,
		Black:  lipgloss.Color("#111111"),
		Yellow: lipgloss.Color("#FFD23F"),
		Pink:   lipgloss.Color("#FF7AC6"),
		Cyan:   lipgloss.Color("#5CE1E6"),
		Lime:   lipgloss.Color("#B8F35C"),
		Red:    lipgloss.Color("#FF5D5D"),
		Violet: lipgloss.Color("#B79CFF"),
		Paper:  lipgloss.Color("#FFF8E7"),
		Shimmer: lipgloss.Blend1D(48,
			lipgloss.Color("#FF2E93"), lipgloss.Color("#7B2FFF"), lipgloss.Color("#0077FF"), lipgloss.Color("#FF2E93")),
	}
	if isDark {
		p.Ink = lipgloss.Color("#F5F0E1")
		p.Muted = lipgloss.Color("#9A968C")
	} else {
		p.Ink = lipgloss.Color("#111111")
		p.Muted = lipgloss.Color("#5E5A52")
	}
	return p
}

// On returns a style for dark text on fill.
func (p Palette) On(fill color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(p.Black).Background(fill)
}

// Text is ink text on the terminal background.
func (p Palette) Text() lipgloss.Style { return lipgloss.NewStyle().Foreground(p.Ink) }

// Dim is secondary text on the terminal background.
func (p Palette) Dim() lipgloss.Style { return lipgloss.NewStyle().Foreground(p.Muted) }

// Line pads styled spans to width with fill so a filled shape has no holes.
// Spans must already carry the fill (see On); the line is cut if too long.
func (p Palette) Line(fill color.Color, width int, spans ...string) string {
	s := ansi.Truncate(strings.Join(spans, ""), width, "…")
	if gap := width - lipgloss.Width(s); gap > 0 {
		s += p.On(fill).Render(strings.Repeat(" ", gap))
	}
	return s
}

// Card renders lines (built with Line at the inner width) inside a thick
// border. Width is the outer width.
func (p Palette) Card(fill, border color.Color, width int, lines ...string) string {
	inner := width - 4
	body := make([]string, len(lines))
	for i, l := range lines {
		body[i] = p.On(fill).Render(" ") + p.Line(fill, inner, l) + p.On(fill).Render(" ")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(border).
		Render(strings.Join(body, "\n"))
}

// Shadow is a solid block the size of a shape, drawn offset beneath it.
func (p Palette) Shadow(width, height int) string {
	row := lipgloss.NewStyle().Foreground(p.Ink).Render(strings.Repeat("█", width))
	return strings.TrimSuffix(strings.Repeat(row+"\n", height), "\n")
}

// Button is a two-row chunky button: a filled label with a half-block ink
// shadow under it. Pressed buttons sink into their shadow.
func (p Palette) Button(label string, fill color.Color, pressed bool) string {
	text := p.On(fill).Bold(true).Render(" " + label + " ")
	w := lipgloss.Width(text)
	ink := lipgloss.NewStyle().Foreground(p.Ink)
	if pressed {
		return " " + text + "\n" + strings.Repeat(" ", w+1)
	}
	return text + ink.Render("▌") + "\n" + " " + ink.Render(strings.Repeat("▀", w))
}

// ButtonOn is Button drawn inside a filled card, so its gaps carry the fill.
func (p Palette) ButtonOn(card color.Color, label string, fill color.Color, pressed bool) string {
	text := p.On(fill).Bold(true).Render(" " + label + " ")
	w := lipgloss.Width(text)
	ink := lipgloss.NewStyle().Foreground(p.Black).Background(card)
	gap := p.On(card)
	if pressed {
		return gap.Render(" ") + text + "\n" + gap.Render(strings.Repeat(" ", w+1))
	}
	return text + ink.Render("▌") + "\n" + gap.Render(" ") + ink.Render(strings.Repeat("▀", w))
}

// Chip is a small inverted label, used for section headings and badges.
func (p Palette) Chip(label string, fill color.Color) string {
	return p.On(fill).Bold(true).Render(" " + strings.ToUpper(label) + " ")
}

// Sparkle frames, from dim to bright; a sparkle cycles through them.
var Sparkle = []string{"·", "✧", "✦", "✶", "✦", "✧"}

// Spin is a small busy indicator frame.
func Spin(frame int) string { return []string{"◐", "◓", "◑", "◒"}[frame%4] }

// Panel renders a dialog: a thick ink border, a filled title bar and plain
// body lines (on the terminal background) padded to the inner width.
func (p Palette) Panel(width int, titleFill color.Color, title string, body []string) string {
	inner := width - 2
	lines := []string{p.Line(titleFill, inner, p.On(titleFill).Bold(true).Render(" "+strings.ToUpper(title)))}
	for _, l := range body {
		l = ansi.Truncate(l, inner-2, "…")
		lines = append(lines, " "+l+strings.Repeat(" ", max(0, inner-2-lipgloss.Width(l)))+" ")
	}
	return lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(p.Ink).Render(strings.Join(lines, "\n"))
}

// Dimmed returns the palette with every fill grayed out, for the screen
// behind a dialog.
func (p Palette) Dimmed() Palette {
	gray := lipgloss.Color("#8C8A84")
	light := lipgloss.Color("#B5B2AA")
	p.Yellow, p.Pink, p.Cyan, p.Lime, p.Violet, p.Red = light, gray, light, light, gray, gray
	p.Paper = light
	p.Ink = p.Muted
	p.Shimmer = []color.Color{lipgloss.Color("#6E6C66")}
	return p
}
