// Package theme holds the palette and shared Lip Gloss styles so every
// screen in agentctl looks like the same product.
package theme

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Brand gradient stops, cycled by animated headers.
var (
	Pink   = lipgloss.Color("#FF5FAF")
	Violet = lipgloss.Color("#8B5CF6")
	Cyan   = lipgloss.Color("#22D3EE")
)

// Gradient returns a cyclic blend of the brand colors with n steps; the last
// step flows back into the first so it can scroll without a seam.
func Gradient(n int) []color.Color {
	return lipgloss.Blend1D(n, Pink, Violet, Cyan, Pink)
}

// Styles is built once from the terminal background and passed to views.
type Styles struct {
	IsDark bool

	Title   lipgloss.Style
	Subtle  lipgloss.Style
	Text    lipgloss.Style
	Accent  lipgloss.Style
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Panel   lipgloss.Style
}

// New builds styles for a dark or light background.
func New(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	subtle := ld(lipgloss.Color("#8A8A99"), lipgloss.Color("#6C6C80"))
	return Styles{
		IsDark:  isDark,
		Title:   lipgloss.NewStyle().Bold(true).Foreground(ld(Violet, lipgloss.Color("#C4B5FD"))),
		Subtle:  lipgloss.NewStyle().Foreground(subtle),
		Text:    lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#1F1F2E"), lipgloss.Color("#E4E4F0"))),
		Accent:  lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#C026D3"), Pink)),
		Success: lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#16A34A"), lipgloss.Color("#4ADE80"))),
		Warning: lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#CA8A04"), lipgloss.Color("#FACC15"))),
		Error:   lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#DC2626"), lipgloss.Color("#F87171"))),
		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(1, 2),
	}
}

// Diff colors a unified diff line by line.
func (s Styles) Diff(d string) string {
	var b strings.Builder
	for line := range strings.Lines(d) {
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			line = s.Title.Render(line)
		case strings.HasPrefix(line, "+"):
			line = s.Success.Render(line)
		case strings.HasPrefix(line, "-"):
			line = s.Error.Render(line)
		case strings.HasPrefix(line, "@@"):
			line = s.Accent.Render(line)
		default:
			line = s.Subtle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
