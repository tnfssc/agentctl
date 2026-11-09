package skin

import (
	"strings"

	"charm.land/lipgloss/v2"
	"rsc.io/qr"
)

// qrQuiet is the white margin, in modules, scanners need around a code.
const qrQuiet = 2

// QR renders text as a QR code, two modules per row with half blocks. It is
// always black on white, whatever the terminal's theme: many phone scanners
// can't read an inverted code. One style per row keeps it cheap over SSH.
func QR(text string) ([]string, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return nil, err
	}
	dark := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < code.Size && y < code.Size && code.Black(x, y)
	}
	style := On(lipgloss.Color("#000000"), lipgloss.Color("#ffffff"))
	var rows []string
	for y := -qrQuiet; y < code.Size+qrQuiet; y += 2 {
		var row strings.Builder
		for x := -qrQuiet; x < code.Size+qrQuiet; x++ {
			switch top, bottom := dark(x, y), dark(x, y+1); {
			case top && bottom:
				row.WriteString("█")
			case top:
				row.WriteString("▀")
			case bottom:
				row.WriteString("▄")
			default:
				row.WriteString(" ")
			}
		}
		rows = append(rows, style.Render(row.String()))
	}
	return rows, nil
}
