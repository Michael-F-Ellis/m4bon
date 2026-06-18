package render

import (
	"strings"
)

// ANSI escape code constants.
const (
	ansiReset = "\033[0m"
)

// ansiColor returns the ANSI 24-bit color escape for a StyleClass.
func ansiColor(s StyleClass) string {
	switch s {
	case StyleSharp:
		return "\033[38;2;209;34;34m"
	case StyleFlat:
		return "\033[38;2;152;140;254m"
	case StyleDoubleSharp:
		return "\033[38;2;255;165;0m"
	case StyleDoubleFlat:
		return "\033[38;2;4;182;4m"
	case StyleSustainRest:
		return "\033[38;2;160;160;160m"
	case StyleParen:
		return "\033[38;2;120;120;120m"
	default:
		return ""
	}
}

// FormatANSI converts a sequence of measure cell-sequences into an
// ANSI-escaped string. Each measure becomes one line.
func FormatANSI(measures []CellSeq) string {
	var b strings.Builder
	for _, cells := range measures {
		for _, c := range cells {
			content := c.Content + c.Subscript

			// Apply ANSI escapes
			if c.Style != StyleDefault {
				b.WriteString(ansiColor(c.Style))
			}
			if c.Italic {
				b.WriteString("\033[3m")
			}
			if c.Style != StyleDefault || c.Italic {
				b.WriteString(content)
				b.WriteString(ansiReset)
			} else {
				b.WriteString(content)
			}
		}
	}
	return b.String()
}

// FormatPlain converts a sequence of measure cell-sequences into plain text
// (no ANSI escapes). Useful for tests and for deriving text from the IR.
func FormatPlain(measures []CellSeq) string {
	var b strings.Builder
	for _, cells := range measures {
		for _, c := range cells {
			content := c.Content + c.Subscript
			b.WriteString(content)
		}
	}
	return b.String()
}
