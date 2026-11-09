// Package skin is agentctl's look, after the voidzero.dev site: a quiet canvas
// split by hairlines, small uppercase labels, crisp headings over muted text,
// and outlined buttons.
//
// Most text sits on the terminal's own background. Only a few shapes are
// filled (selected rows, the primary button, the gradient); Lip Gloss resets
// styles after every span, so text inside a fill must carry it (see On).
package skin

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Palette holds the colors for one terminal background.
type Palette struct {
	IsDark bool

	Text   color.Color // headings, active items
	Muted  color.Color // body text
	Dim    color.Color // labels, inactive items
	Line   color.Color // hairlines
	Strong color.Color // hairlines that need to stand out (focused boxes)
	Raised color.Color // fill for selected rows

	Accent color.Color // violet, the brand color
	Blue   color.Color
	Cyan   color.Color
	Green  color.Color
	Red    color.Color
	Amber  color.Color
}

// New returns colors for a dark or light terminal.
func New(isDark bool) Palette {
	p := Palette{
		IsDark: isDark,
		Accent: lipgloss.Color("#A78BFA"),
		Blue:   lipgloss.Color("#7AA2FF"),
		Cyan:   lipgloss.Color("#5EEAD4"),
		Green:  lipgloss.Color("#4ADE80"),
		Red:    lipgloss.Color("#F87171"),
		Amber:  lipgloss.Color("#FBBF24"),
	}
	if isDark {
		p.Text = lipgloss.Color("#EDEDF0")
		p.Muted = lipgloss.Color("#A1A1AA")
		p.Dim = lipgloss.Color("#6B6B76")
		p.Line = lipgloss.Color("#2E2F38")
		p.Strong = lipgloss.Color("#4A4B57")
		p.Raised = lipgloss.Color("#24252D")
	} else {
		p.Text = lipgloss.Color("#141418")
		p.Muted = lipgloss.Color("#52525B")
		p.Dim = lipgloss.Color("#8A8A93")
		p.Line = lipgloss.Color("#E4E4E7")
		p.Strong = lipgloss.Color("#C4C4CC")
		p.Raised = lipgloss.Color("#F1F1F4")
		p.Accent = lipgloss.Color("#6D28D9")
		p.Blue = lipgloss.Color("#2563EB")
		p.Cyan = lipgloss.Color("#0E7490")
		p.Green = lipgloss.Color("#15803D")
		p.Red = lipgloss.Color("#B91C1C")
		p.Amber = lipgloss.Color("#B45309")
	}
	return p
}

// Faded is the palette for the page behind a dialog: everything recedes to
// the dim color.
func (p Palette) Faded() Palette {
	p.Text, p.Muted = p.Dim, p.Dim
	p.Accent, p.Blue, p.Cyan, p.Green, p.Red, p.Amber = p.Dim, p.Dim, p.Dim, p.Dim, p.Dim, p.Dim
	return p
}

// Fg returns a style with only a foreground color.
func Fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// On returns fg-on-bg text, for spans inside a filled shape.
func On(fg, bg color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(fg).Background(bg) }

// Label is a small uppercase section label, like "OSS" or "VITE+".
func (p Palette) Label(s string) string { return Fg(p.Dim).Render(strings.ToUpper(s)) }

// Heading is a section heading.
func (p Palette) Heading(s string) string { return Fg(p.Text).Bold(true).Render(s) }

// Body is muted paragraph text wrapped to width.
func (p Palette) Body(s string, width int) string {
	return Fg(p.Muted).Render(lipgloss.Wrap(s, width, ""))
}

// HLine is a hairline of the given width.
func (p Palette) HLine(width int) string {
	return Fg(p.Line).Render(strings.Repeat("─", max(0, width)))
}

// VLine is a vertical hairline of the given height.
func (p Palette) VLine(height int) string {
	return Fg(p.Line).Render(strings.TrimSuffix(strings.Repeat("│\n", max(0, height)), "\n"))
}

// Box draws a hairline border around content.
func (p Palette) Box(content string, focused bool) string {
	border := p.Line
	if focused {
		border = p.Strong
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).Render(content)
}

// Button is a three-row button exactly width cells wide, label centered and
// truncated to fit, so swapping labels never moves anything. Primary buttons
// are a solid block; hovered buttons light up their outline or turn the block
// to the accent color.
func (p Palette) Button(label string, width int, primary, hover bool) string {
	inner := max(1, width-2)
	label = ansi.Truncate(label, inner, "…")
	left := (inner - lipgloss.Width(label)) / 2
	text := strings.Repeat(" ", left) + label + strings.Repeat(" ", inner-left-lipgloss.Width(label))
	if primary {
		bg, fg := p.Text, lipgloss.Color("#0B0B0F")
		if !p.IsDark {
			fg = lipgloss.Color("#FFFFFF")
		}
		if hover {
			bg = p.Accent
		}
		blank := On(fg, bg).Render(strings.Repeat(" ", width))
		return blank + "\n" + On(fg, bg).Render(" ") + On(fg, bg).Bold(true).Render(text) + On(fg, bg).Render(" ") + "\n" + blank
	}
	border, fg := p.Strong, p.Muted
	if hover {
		border, fg = p.Text, p.Text
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(border).Render(Fg(fg).Render(text))
}

// Pill is the bordered status badge, a colored square then dim and bright
// text ("■ prod server / running"), always width cells wide.
func (p Palette) Pill(dot color.Color, dim, bright string, width int) string {
	const brightW = 9
	dimW := max(1, width-2-2-3-brightW) // border, "■ ", " / "
	s := Fg(dot).Render("■") + " " + Fg(p.Muted).Render(Cell(dim, dimW)) + Fg(p.Dim).Render(" / ") + Fg(p.Text).Render(Cell(bright, brightW))
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(p.Line).Render(Cell(s, width-2))
}

// Cell renders styled text as exactly one row of exactly width cells: line
// breaks become spaces, then the text is truncated or padded.
func Cell(s string, width int) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return pad(ansi.Truncate(s, max(0, width), "…"), width)
}

// Clip wraps text to width and keeps at most lines lines, ending the last
// one with an ellipsis when text was cut.
func Clip(text string, width, lines int) []string {
	out := strings.Split(lipgloss.Wrap(text, max(1, width), ""), "\n")
	if len(out) > lines {
		out = out[:lines]
		out[lines-1] = ansi.Truncate(out[lines-1], max(1, width-1), "") + "…"
	}
	return out
}

// Wordmark is the product name in the site's "VOID(∅)" manner.
func (p Palette) Wordmark() string {
	return Fg(p.Text).Bold(true).Render("AGENTCTL") + Fg(p.Accent).Bold(true).Render("(∅)")
}

// Row renders one list row exactly width cells wide: icon, label in a
// labelW-wide column, then detail. Selected rows sit on a raised fill with an
// accent marker, hovered rows brighten, the rest are dim.
func (p Palette) Row(icon, label string, labelW int, detail string, width int, selected, hover bool) string {
	label = Cell(label, labelW)
	if detail != "" {
		// Marker or margin, icon and its space, label, two-space gap.
		used := 1 + lipgloss.Width(icon) + 1 + labelW + 2
		detail = "  " + Cell(detail, max(0, width-used))
	}
	switch {
	case selected:
		on := On(p.Text, p.Raised)
		s := On(p.Accent, p.Raised).Render("▎") + on.Render(icon+" ") + on.Bold(true).Render(label) + On(p.Muted, p.Raised).Render(detail)
		s = ansi.Truncate(s, width, "…")
		return s + On(p.Text, p.Raised).Render(strings.Repeat(" ", max(0, width-lipgloss.Width(s))))
	case hover:
		return Cell(" "+Fg(p.Text).Render(icon+" "+label)+Fg(p.Muted).Render(detail), width)
	default:
		return Cell(" "+Fg(p.Dim).Render(icon+" ")+Fg(p.Muted).Render(label)+Fg(p.Dim).Render(detail), width)
	}
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

// Spin is a quiet busy indicator.
func Spin(frame int) string {
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}[frame%10]
}
