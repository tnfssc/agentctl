package dash

import (
	"image/color"

	"github.com/tnfssc/agentctl/internal/tui/skin"
)

// confettiFrames is how long the burst lasts at fps.
const confettiFrames = 2 * fps

// drawConfetti scatters pieces that fall from the top while the burst
// lasts. Positions come from the piece's index and the frame alone, so the
// burst looks the same on every redraw. The pieces sit above everything but
// carry no ids: clicks go through them.
func (m Model) drawConfetti(sc *scene) {
	if m.confetti == 0 || m.width == 0 {
		return
	}
	p := m.pal
	colors := []color.Color{p.Accent, p.Green, p.Amber, p.Blue, p.Red, p.Cyan}
	glyphs := []string{"*", "•", "◆", "▪", "✦", "+"}
	age := confettiFrames - m.confetti
	n := max(12, m.width*m.height/40)
	for i := range n {
		h := (i*2654435761 + 97) & 0x7fffffff // a fixed scatter per piece
		x := h % m.width
		start := -(h >> 8 % m.height)
		speed := 1 + h>>16%3
		y := start + age*speed
		if y < 0 || y >= m.height {
			continue
		}
		x = (x + age*(h>>20%3) - age) % m.width
		if x < 0 {
			x += m.width
		}
		sc.add(skin.Fg(colors[h>>4%len(colors)]).Bold(true).Render(glyphs[h>>12%len(glyphs)]), x, y, 90, "")
	}
}
