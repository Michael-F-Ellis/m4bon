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
	case StyleComment:
		return "\033[38;2;80;150;80m\033[2m"
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

// FormatANSIRows converts MeasureRows into an ANSI-escaped string with
// three-column layout (chords, notes, lyrics), padding each column to
// its maximum width. Newlines separate measures.
func FormatANSIRows(rows []MeasureRow, maxChordW, maxNoteW, maxLyricW int, asciiLeaps bool) string {
	var b strings.Builder
	for ri, row := range rows {
		if ri > 0 {
			b.WriteByte('\n')
		}

		// Comment block before measure
		if len(row.CommentCells) > 0 {
			writeCellSeq(&b, row.CommentCells, asciiLeaps)
			b.WriteByte('\n')
		}

		// Chord column: left-justified, padded to maxChordW
		if maxChordW > 0 {
			writeCellSeq(&b, row.ChordCells, asciiLeaps)
			padW := maxChordW - visibleLen(row.ChordCells)
			b.WriteString(strings.Repeat(" ", padW))
			b.WriteString("    ") // 4-space gap
		}

		// Note column
		writeCellSeq(&b, row.NoteCells, asciiLeaps)
		if maxLyricW > 0 {
			padW := maxNoteW - visibleLen(row.NoteCells)
			b.WriteString(strings.Repeat(" ", padW))
			b.WriteString("    ") // 4-space gap

			// Lyric column: left-justified
			writeCellSeq(&b, row.LyricCells, asciiLeaps)
		}

		// Trailing comment block after the measure
		if len(row.TrailingCommentCells) > 0 {
			b.WriteByte('\n')
			writeCellSeq(&b, row.TrailingCommentCells, asciiLeaps)
		}
	}
	b.WriteByte('\n')
	return b.String()
}

// FormatPlainRows converts MeasureRows into plain text with three-column layout.
func FormatPlainRows(rows []MeasureRow, maxChordW, maxNoteW, maxLyricW int) string {
	var b strings.Builder
	for ri, row := range rows {
		if ri > 0 {
			b.WriteByte('\n')
		}
		// Comment block before measure
		if len(row.CommentCells) > 0 {
			plainWriteCells(&b, row.CommentCells)
			b.WriteByte('\n')
		}
		if maxChordW > 0 {
			plainWriteCells(&b, row.ChordCells)
			padW := maxChordW - visibleLen(row.ChordCells)
			b.WriteString(strings.Repeat(" ", padW))
			b.WriteString("    ")
		}
		plainWriteCells(&b, row.NoteCells)
		if maxLyricW > 0 {
			padW := maxNoteW - visibleLen(row.NoteCells)
			b.WriteString(strings.Repeat(" ", padW))
			b.WriteString("    ")
			plainWriteCells(&b, row.LyricCells)
		}
	}
	b.WriteByte('\n')
	return b.String()
}

// writeCellSeq writes a CellSeq with ANSI escapes to a builder.
func writeCellSeq(b *strings.Builder, cells CellSeq, asciiLeaps bool) {
	for _, c := range cells {
		baseContent := c.Content
		subscript := c.Subscript

		var unicodeLeap string
		if !asciiLeaps {
			switch c.Leap {
			case LeapUp:
				unicodeLeap = combiningCirc
			case LeapDown:
				unicodeLeap = combiningMacr
			}
		}

		var leapPrefix string
		if asciiLeaps {
			switch c.Leap {
			case LeapUp:
				leapPrefix = ansiOverline
			case LeapDown:
				leapPrefix = ansiUnderline
			}
		}

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

// plainWriteCells writes a CellSeq as plain text to a builder.
func plainWriteCells(b *strings.Builder, cells CellSeq) {
	for _, c := range cells {
		b.WriteString(c.Content)
		b.WriteString(c.Subscript)
	}
}

// stripTrailingNewline returns a copy of cells with the trailing newline
// cell removed, if present.
func stripTrailingNewline(cells CellSeq) CellSeq {
	if len(cells) > 0 && cells[len(cells)-1].Content == "\n" {
		return cells[:len(cells)-1]
	}
	return cells
}
