package neo

import (
	"image/color"
	"math/rand/v2"
	"strings"

	"charm.land/lipgloss/v2"
)

// glyphs spells AGENTCTL in the "ANSI Shadow" figlet font. Each glyph's rows
// share one width so they can be laid side by side.
var glyphs = [][6]string{
	{" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
	{" ██████╗ ", "██╔════╝ ", "██║  ███╗", "██║   ██║", "╚██████╔╝", " ╚═════╝ "},
	{"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"},
	{"███╗   ██╗", "████╗  ██║", "██╔██╗ ██║", "██║╚██╗██║", "██║ ╚████║", "╚═╝  ╚═══╝"},
	{"████████╗", "╚══██╔══╝", "   ██║   ", "   ██║   ", "   ██║   ", "   ╚═╝   "},
	{" ██████╗", "██╔════╝", "██║     ", "██║     ", "╚██████╗", " ╚═════╝"},
	{"████████╗", "╚══██╔══╝", "   ██║   ", "   ██║   ", "   ██║   ", "   ╚═╝   "},
	{"██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"},
}

var titleRows = func() [6][]rune {
	var rows [6][]rune
	for _, g := range glyphs {
		for i, part := range g {
			rows[i] = append(rows[i], []rune(part)...)
		}
	}
	return rows
}()

// TitleWidth and TitleHeight are the size of the big title.
var (
	TitleWidth  = len(titleRows[0])
	TitleHeight = len(titleRows)
)

// Title renders AGENTCTL on fill. Solid blocks take a gradient that moves
// with frame, the letter outlines stay dark, so it shimmers but stays legible.
func (p Palette) Title(fill color.Color, frame int) string {
	outline := p.On(fill)
	shimmer := p.Shimmer
	n := len(shimmer)
	lines := make([]string, len(titleRows))
	for y, row := range titleRows {
		var b strings.Builder
		for x, r := range row {
			switch r {
			case '█':
				c := shimmer[((x+y*2-frame)%n+n)%n]
				b.WriteString(lipgloss.NewStyle().Foreground(c).Background(fill).Render("█"))
			default:
				b.WriteString(outline.Render(string(r)))
			}
		}
		lines[y] = b.String()
	}
	return strings.Join(lines, "\n")
}

// SmallTitle is the one-line title for small terminals.
func (p Palette) SmallTitle(fill color.Color, frame int) string {
	var b strings.Builder
	shimmer := p.Shimmer
	for i, r := range "AGENTCTL" {
		c := shimmer[((i*4-frame)%len(shimmer)+len(shimmer))%len(shimmer)]
		b.WriteString(lipgloss.NewStyle().Foreground(c).Background(fill).Bold(true).Render(string(r)))
	}
	return b.String()
}

// Sparkles returns twinkling stars for a width×height area at frame: each
// lives a few frames at a pseudo-random spot, brightening and fading, then
// moves. Positions are stable within a life so the field twinkles rather
// than flickers.
func (p Palette) Sparkles(width, height, frame, count int) []Star {
	colors := []color.Color{p.Yellow, p.Pink, p.Cyan, p.Lime, p.Violet}
	stars := make([]Star, 0, count)
	life := len(Sparkle)
	for i := range count {
		// Stagger lives so stars do not all move on the same frame.
		age := frame + i*3
		// Seeding by star and life keeps a star in place for its whole life.
		r := rand.New(rand.NewPCG(uint64(i), uint64(age/life))) //nolint:gosec // decoration, not security
		stars = append(stars, Star{
			X:     r.IntN(max(1, width)),
			Y:     r.IntN(max(1, height)),
			Glyph: lipgloss.NewStyle().Foreground(colors[r.IntN(len(colors))]).Render(Sparkle[age%life]),
		})
	}
	return stars
}

// Star is one sparkle at a position.
type Star struct {
	X, Y  int
	Glyph string
}
