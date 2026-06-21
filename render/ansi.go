package render

import (
	"strings"
)

// ANSI escape code constants.
const (
	ansiReset     = "\033[0m"
	combiningCirc = "\u0302" // combining circumflex accent — upward leap
	combiningMacr = "\u0331" // combining macron below — downward leap
	ansiOverline  = "\033[53m"
	ansiUnderline = "\033[4m"
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
// asciiLeaps uses ANSI overline/underline for leap indicators instead
// of Unicode combining diacritics.
func FormatANSI(measures []CellSeq, asciiLeaps bool) string {
	var b strings.Builder
	for _, cells := range measures {
		for _, c := range cells {
			baseContent := c.Content
			subscript := c.Subscript

			// Unicode combining diacritic is placed after the base letter
			// but before the subscript for correct rendering.
			var unicodeLeap string
			if !asciiLeaps {
				switch c.Leap {
				case LeapUp:
					unicodeLeap = combiningCirc
				case LeapDown:
					unicodeLeap = combiningMacr
				}
			}

			// ANSI leap escapes wrap the entire content
			var leapPrefix string
			if asciiLeaps {
				switch c.Leap {
				case LeapUp:
					leapPrefix = ansiOverline
				case LeapDown:
					leapPrefix = ansiUnderline
				}
			}

			// Apply ANSI escapes
			if c.Style != StyleDefault || c.Italic || leapPrefix != "" {
				b.WriteString(leapPrefix)
				if c.Style != StyleDefault {
					b.WriteString(ansiColor(c.Style))
				}
				if c.Italic {
					b.WriteString("\033[3m")
				}
				b.WriteString(baseContent)
				b.WriteString(unicodeLeap)
				b.WriteString(subscript)
				b.WriteString(ansiReset)
			} else {
				b.WriteString(baseContent)
				b.WriteString(unicodeLeap)
				b.WriteString(subscript)
			}
		}
	}
	return b.String()
}

// FormatPlain converts a sequence of measure cell-sequences into plain text
// (no ANSI escapes, no combining diacritics). Useful for tests.
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
