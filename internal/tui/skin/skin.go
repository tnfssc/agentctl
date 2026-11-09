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

// LogoLines is agentctl's logo, the word in figlet's Calvin S font. Every
// line is LogoWidth cells wide.
var LogoLines = [3]string{
	"┌─┐┌─┐┌─┐┌┐┌┌┬┐┌─┐┌┬┐┬  ",
	"├─┤│ ┬├┤ │││ │ │   │ │  ",
	"┴ ┴└─┘└─┘┘└┘ ┴ └─┘ ┴ ┴─┘",
}

// LogoWidth is the logo's width in cells.
const LogoWidth = 24

// Tagline says what agentctl does, under the logo where there is room.
const Tagline = "route coding agents to their own login or a CLIProxyAPI server"

// Wordmark is the logo's one-line form for headers with no room for three
// rows, in the same gradient.
func (p Palette) Wordmark() string {
	const word = "agentctl"
	colors := lipgloss.Blend1D(len(word), p.Accent, p.Blue, p.Cyan)
	var b strings.Builder
	for i, r := range word {
		b.WriteString(Fg(colors[i]).Bold(true).Render(string(r)))
	}
	return b.String()
}

// Logo renders the logo with a gradient from the accent violet through
// blue to cyan, column by column.
func (p Palette) Logo() string {
	colors := lipgloss.Blend1D(LogoWidth, p.Accent, p.Blue, p.Cyan)
	var b strings.Builder
	for i, line := range LogoLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		for x, r := range []rune(line) {
			b.WriteString(Fg(colors[x]).Render(string(r)))
		}
	}
	return b.String()
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

// Tall renders a two-line list item, exactly width cells wide: icon, label
// and an optional trailing mark (e.g. "›") on the first line, detail under
// the label on the second. Two lines make a target a finger can hit on a
// phone. Selected items sit on a raised fill with an accent marker.
func (p Palette) Tall(icon, label, detail, mark string, width int, selected, hover bool) string {
	return p.TallDot(icon, nil, label, detail, mark, width, selected, hover)
}

// TallDot is Tall with the icon in its own color, e.g. a status dot. The
// icon must be plain text: Lip Gloss resets styles after every span, so a
// pre-colored icon would drop the fill and the label's style after it.
func (p Palette) TallDot(icon string, dot color.Color, label, detail, mark string, width int, selected, hover bool) string {
	iconW, markW := lipgloss.Width(icon), lipgloss.Width(mark)
	textW := max(0, width-3-iconW-markW-1)
	top := " " + Cell(label, textW)
	bottom := strings.Repeat(" ", iconW+1) + Cell(detail, textW)
	if mark != "" {
		top += " " + mark
		bottom += " " + strings.Repeat(" ", markW)
	}
	text, sub := p.Muted, p.Dim
	switch {
	case selected:
		text, sub = p.Text, p.Muted
	case hover:
		text, sub = p.Text, p.Muted
	}
	iconC := text
	if dot != nil {
		iconC = dot
	}
	if !selected {
		return Cell(" "+Fg(iconC).Render(icon)+Fg(text).Render(top), width) + "\n" + Cell(" "+Fg(sub).Render(bottom), width)
	}
	bar := On(p.Accent, p.Raised).Render("▎")
	fill := func(used int) string {
		return On(p.Text, p.Raised).Render(strings.Repeat(" ", max(0, width-1-used)))
	}
	first := bar + On(iconC, p.Raised).Bold(true).Render(icon) + On(text, p.Raised).Bold(true).Render(top) +
		fill(iconW+lipgloss.Width(top))
	second := bar + On(sub, p.Raised).Render(bottom) + fill(lipgloss.Width(bottom))
	return first + "\n" + second
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

// Spin is a quiet busy indicator.
func Spin(frame int) string {
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}[frame%10]
}
