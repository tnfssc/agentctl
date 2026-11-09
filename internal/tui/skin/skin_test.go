package skin

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"rsc.io/qr"
)

func TestRowIsExactlyItsWidth(t *testing.T) {
	p := New(true)
	for _, icon := range []string{"◆", "[x]"} {
		for _, sel := range []bool{false, true} {
			for _, hover := range []bool{false, true} {
				row := p.Row(icon, "Codex", 11, "own login", 27, sel, hover)
				if w := lipgloss.Width(row); w != 27 {
					t.Errorf("icon %q selected %v hover %v: width %d", icon, sel, hover, w)
				}
				if plain := ansi.Strip(row); strings.Contains(plain, "…") {
					t.Errorf("icon %q selected %v hover %v: text that fits was cut: %q", icon, sel, hover, plain)
				}
			}
		}
	}
}

func TestQRIsSquareAndBlackOnWhite(t *testing.T) {
	rows, err := QR("https://box.tail035df.ts.net/pair#token=TOKEN12CHARS")
	if err != nil {
		t.Fatal(err)
	}
	w := lipgloss.Width(rows[0])
	if w != 2*len(rows)-1 && w != 2*len(rows) {
		t.Fatalf("%d rows of %d columns is not square in modules", len(rows), w)
	}
	for _, r := range rows {
		if lipgloss.Width(r) != w {
			t.Fatal("rows differ in width")
		}
	}
}

func TestQRDrawsEveryModule(t *testing.T) {
	text := "https://box.tail035df.ts.net/pair#token=TOKEN12CHARS"
	rows, err := QR(text)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := qr.Encode(text, qr.L)
	for ry, row := range rows {
		for rx, r := range []rune(ansi.Strip(row)) {
			x, y := rx-qrQuiet, 2*ry-qrQuiet
			top := r == '█' || r == '▀'
			bottom := r == '█' || r == '▄'
			if top != (x >= 0 && y >= 0 && x < code.Size && y < code.Size && code.Black(x, y)) ||
				bottom != (x >= 0 && y+1 >= 0 && x < code.Size && y+1 < code.Size && code.Black(x, y+1)) {
				t.Fatalf("cell %d,%d (%q) does not match modules %d,%d-%d", rx, ry, r, x, y, y+1)
			}
		}
	}
}

func TestTallIsTwoLinesOfItsWidth(t *testing.T) {
	p := New(true)
	for _, sel := range []bool{false, true} {
		for _, hover := range []bool{false, true} {
			for _, mark := range []string{"", "›"} {
				s := p.Tall("◆", "OpenCode with a long long name", "→ home · gemini-3.1-pro", mark, 30, sel, hover)
				lines := strings.Split(s, "\n")
				if len(lines) != 2 {
					t.Fatalf("want 2 lines, got %d", len(lines))
				}
				for _, l := range lines {
					if w := lipgloss.Width(l); w != 30 {
						t.Errorf("selected %v hover %v mark %q: line width %d: %q", sel, hover, mark, w, ansi.Strip(l))
					}
				}
			}
		}
	}
}
